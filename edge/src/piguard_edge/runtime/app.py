"""Sample, judge, and publish the latest telemetry, status, and events."""

import json
import sqlite3
import threading
import time
import uuid
from collections.abc import Callable
from contextlib import ExitStack
from dataclasses import replace
from datetime import timedelta

from piguard_edge import __version__
from piguard_edge.actuators.manager import ActuatorManager
from piguard_edge.commands.handler import CommandHandler
from piguard_edge.config.model import EdgeConfig
from piguard_edge.drivers.base import Clock
from piguard_edge.models.sample import SampleStatus, format_utc
from piguard_edge.protocol.config import merge_rules, parse_config
from piguard_edge.protocol.event import EdgeEvent
from piguard_edge.protocol.status import build_online_status, build_shutdown_status, build_will, sensor_statuses
from piguard_edge.protocol.telemetry import build_telemetry
from piguard_edge.protocol.topics import command_acks_topic, commands_topic, events_topic, status_topic, telemetry_topic
from piguard_edge.rules.engine import RuleEngine
from piguard_edge.runtime.workers import Sampler
from piguard_edge.simulation.engine import ScenarioEngine
from piguard_edge.storage.commands import CommandStore
from piguard_edge.storage.configs import ConfigStore
from piguard_edge.storage.events import EventStore
from piguard_edge.transport.http import HttpWorker, PlatformHttp
from piguard_edge.transport.mqtt import MqttMessage

_NS_PER_SECOND = 1_000_000_000


class EdgeApp:
    def __init__(
        self,
        config: EdgeConfig,
        clock: Clock,
        transport: object,
        sampler: Sampler,
        message_ids: Callable[[], str] | None = None,
        rules: RuleEngine | None = None,
        store: EventStore | None = None,
        actuators: ActuatorManager | None = None,
        engine: ScenarioEngine | None = None,
        command_store: CommandStore | None = None,
        http_worker: HttpWorker | None = None,
    ) -> None:
        self._config = config
        self._clock = clock
        self._transport = transport
        self._sampler = sampler
        self._message_ids = message_ids or (lambda: str(uuid.uuid4()))
        self._rules = rules if rules is not None else RuleEngine.from_config(config)
        self._store = store if store is not None else EventStore(config.storage.sqlite_path)
        self._actuators = actuators if actuators is not None else ActuatorManager.from_config(config)
        self._engine = engine
        self._http = http_worker
        self._maintenance = None
        self._snapshot = None
        self._confirm_after = 0
        self._next_confirm_ns = 0
        self._next_preview_ns = 0
        self._configs = ConfigStore(config.storage.sqlite_path) if config.features.remote_config else None
        self._commands = CommandHandler(
            config.device.id,
            config.device.mode,
            clock,
            engine,
            self._actuators,
            command_store if command_store is not None else CommandStore(config.storage.sqlite_path),
            self._begin_snapshot if config.features.frame_upload and self._http is not None else None,
        )
        self._seq = 0
        self._connected = False
        self._announced = False
        self._sensors: dict[str, str] | None = None
        self._risk_level = "unknown"
        self._active_events: list[str] = []
        self._unstored: list[EdgeEvent] = []
        self.storage_error: str | None = None
        self.network_error: str | None = None
        self._pending: dict[tuple[str, bytes], object] = {}
        self._connect_started = False
        self._next_connect_ns = 0
        now = clock.monotonic_ns()
        self._next_telemetry_ns = now + _hz_to_ns(config.runtime.telemetry_hz)
        self._next_rule_ns = now + _hz_to_ns(config.runtime.rule_hz)
        self._next_heartbeat_ns = now + config.mqtt.status_heartbeat_s * _NS_PER_SECOND

    @classmethod
    def from_config(
        cls,
        config: EdgeConfig,
        clock: Clock,
        transport: object,
        message_ids: Callable[[], str] | None = None,
    ) -> "EdgeApp":
        engine = ScenarioEngine(config.simulation.scenario, config.simulation.playback_speed)
        with ExitStack() as cleanup:
            def own(resource):
                cleanup.callback(resource.close)
                return resource
            sampler = own(Sampler.from_config(config, engine, clock))
            store = own(EventStore(config.storage.sqlite_path))
            actuators = own(ActuatorManager.from_config(config))
            commands = own(CommandStore(config.storage.sqlite_path))
            http = own(HttpWorker(PlatformHttp(config))) if config.features.frame_upload or config.http.username else None
            app = cls(config, clock, transport, sampler, message_ids, RuleEngine.from_config(config),
                      store, actuators, engine, commands, http)
            cleanup.pop_all()
            return app

    def start(self) -> None:
        if self._configs is not None:
            saved = self._configs.latest()
            if saved is not None and saved[0] >= self._config.device.config_version:
                self._apply_config(saved[0], merge_rules(self._config.rules, json.loads(saved[1])))
        try:
            self._commands.recover()
        except sqlite3.Error as exc:
            self.storage_error = str(exc)
        self._purge_confirmed()
        device_id = self._config.device.id
        will = build_will(device_id=device_id)
        self._transport.set_will(self._status_message(will))
        self._try_connect()
        self._drain()

    def _try_connect(self) -> None:
        now = self._clock.monotonic_ns()
        if self._connect_started or now < self._next_connect_ns:
            return
        self._next_connect_ns = now + _NS_PER_SECOND
        try:
            self._transport.connect()
            self._connect_started = True
        except OSError:
            self.network_error = "MQTT connection unavailable"

    def step(self) -> None:
        self._sampler.step()
        self._try_connect()
        self._retry_unstored()
        self._evaluate_rules()
        messages = self._transport.take_messages()
        for message in messages:
            if message.topic.endswith("/config") and self._configs is not None:
                try:
                    self._accept_config(message)
                except sqlite3.Error as exc:
                    self.storage_error = str(exc)
        self._commands.handle(messages)
        if self._commands.storage_error:
            self.storage_error = self._commands.storage_error
        self._commands.tick(self._clock.monotonic_ns())
        self._http_step()
        self._drain()
        if not self._connected:
            return
        now = self._clock.monotonic_ns()
        if now >= self._next_telemetry_ns:
            self._publish_telemetry()
            period = _hz_to_ns(self._config.runtime.telemetry_hz)
            self._next_telemetry_ns += period
            if self._next_telemetry_ns <= now:
                self._next_telemetry_ns = now + period
        self._flush_events(republish=False)
        self._flush_acks()
        self._flush_config_acks()
        self._publish_status_if_due(now)

    def stop(self) -> None:
        try:
            self._commands.cancel_for_shutdown()
            if self._snapshot is not None:
                self._commands.finish_snapshot(self._snapshot[0], None, "CANCELLED")
                self._snapshot = None
            if self._connected:
                receipt = self._publish(self._status_message(build_shutdown_status(
                    self._sampler.state.snapshot(),
                    device_id=self._config.device.id,
                    timestamp=self._clock.utc(),
                    software_version=__version__,
                    config_version=self._config.device.config_version,
                    mode=self._config.device.mode,
                )))
                self._flush_acks()
                deadline = time.monotonic() + 1
                try:
                    while receipt is not None and not receipt.is_published() and time.monotonic() < deadline:
                        time.sleep(0.01)
                except (ValueError, RuntimeError):
                    self.network_error = "MQTT shutdown confirmation unavailable"
        finally:
            with ExitStack() as cleanup:
                cleanup.callback(self._commands.close)
                cleanup.callback(self._store.close)
                cleanup.callback(self._sampler.close)
                if self._configs is not None:
                    cleanup.callback(self._configs.close)
                    self._configs = None
                if self._http is not None:
                    cleanup.callback(self._http.close)
                def disconnect():
                    try:
                        self._transport.disconnect()
                    except OSError:
                        self.network_error = "MQTT disconnect unavailable"
                cleanup.callback(disconnect)
                cleanup.callback(self._actuators.close)

    def run(self, sleep: Callable[[float], None] | None = None, stop: threading.Event | None = None) -> None:
        pause = sleep or time.sleep
        done = stop or threading.Event()
        try:
            self.start()
            while not done.is_set():
                self.step()
                pause(0.01)
        finally:
            self.stop()

    def _purge_confirmed(self) -> None:
        cutoff = self._clock.utc() - timedelta(days=self._config.storage.confirmed_event_retention_days)
        try:
            self._store.purge_confirmed_before(format_utc(cutoff))
        except (sqlite3.Error, OSError) as exc:
            self.storage_error = str(exc)

    def _apply_config(self, version: int, rules) -> None:
        self._config = replace(self._config, rules=rules, device=replace(self._config.device, config_version=version))
        self._rules.reconfigure(self._config)

    def _accept_config(self, message: MqttMessage) -> None:
        body = parse_config(message.topic, message.payload, self._config.device.id)
        if body is None:
            return
        version = body["config_version"]
        fingerprint = json.dumps(body["rules"], sort_keys=True, separators=(",", ":"))
        existing = self._configs.get(version)
        if existing is not None:
            if existing[1] == fingerprint:
                self._configs.republish(version)
            return
        if version < self._config.device.config_version:
            return
        ack = {"schema_version": 1, "device_id": self._config.device.id, "config_version": version}
        try:
            rules = merge_rules(self._config.rules, body["rules"])
        except ValueError:
            ack.update(status="failed", error={"code": "INVALID_CONFIG", "message": "invalid rule thresholds"})
            self._configs.save(version, fingerprint, _encode(ack))
            return
        ack.update(status="success", applied_at=format_utc(self._clock.utc()))
        self._configs.save(version, fingerprint, _encode(ack))
        self._apply_config(version, rules)
        self._publish_online()

    def _flush_config_acks(self) -> None:
        if self._configs is None:
            return
        for version, payload in self._configs.unpublished():
            if self._publish_confirmed(MqttMessage(f"car/{self._config.device.id}/config-acks", payload, 1, False)):
                self._configs.mark_published(version)

    def _jpeg(self, quality: int) -> tuple[bytes, str]:
        import cv2

        frame = self._sampler.state.frame
        if frame is None or frame.status is not SampleStatus.OK or frame.image is None:
            raise ValueError("camera unavailable")
        if self._clock.monotonic_ns() - frame.monotonic_ns > self._config.runtime.ttl_ms.lane * 1_000_000:
            raise ValueError("camera frame is stale")
        try:
            ok, encoded = cv2.imencode(".jpg", frame.image, [cv2.IMWRITE_JPEG_QUALITY, quality])
        except cv2.error as exc:
            raise ValueError("JPEG encoding failed") from exc
        if not ok:
            raise ValueError("JPEG encoding failed")
        return encoded.tobytes(), format_utc(frame.sample_at)

    def _begin_snapshot(self, command) -> None:
        if self._snapshot is not None:
            self._commands.finish_snapshot(command.command_id, None, "DEVICE_BUSY")
            return
        try:
            jpeg, at = self._jpeg(command.params["quality"])
        except (ValueError, RuntimeError):
            self._commands.finish_snapshot(command.command_id, None, "CAMERA_UNAVAILABLE")
            return
        duration = max(0, (command.expires_at - self._clock.utc()).total_seconds())
        deadline = time.monotonic() + duration
        frame_id = "shot-" + uuid.uuid5(uuid.NAMESPACE_URL, command.device_id + "/" + command.command_id).hex
        future = self._http.submit(self._http.client.upload, jpeg, frame_id, at, "snapshot", deadline)
        self._snapshot = (command.command_id, future, command.expires_at)

    def _http_step(self) -> None:
        if self._http is None:
            return
        if self._snapshot is not None and self._snapshot[1].done():
            command_id, future, expires_at = self._snapshot
            try:
                result = future.result()
                if self._clock.utc() >= expires_at:
                    raise TimeoutError("snapshot command expired")
                self._commands.finish_snapshot(command_id, result)
            except Exception:
                self._commands.finish_snapshot(command_id, None)
                self.network_error = "snapshot upload unavailable"
            self._snapshot = None
        if self._maintenance is not None:
            kind, future = self._maintenance
            if not future.done():
                return
            try:
                result = future.result()
                if kind == "confirm":
                    for event_id in result:
                        self._store.mark_platform_confirmed(event_id)
                    self._purge_confirmed()
            except sqlite3.Error as exc:
                self.storage_error = str(exc)
            except Exception:
                self.network_error = "platform HTTP unavailable"
            self._maintenance = None
        now = self._clock.monotonic_ns()
        if now >= self._next_confirm_ns and self._config.storage.event_confirmation == "http_query":
            self._next_confirm_ns = now + 5 * _NS_PER_SECOND
            rows = self._store.unconfirmed(self._confirm_after)
            if not rows:
                self._confirm_after = 0
                rows = self._store.unconfirmed()
            if rows:
                self._confirm_after = rows[-1].row_id
                future = self._http.submit(self._http.client.confirm, [json.loads(row.payload) for row in rows])
                self._maintenance = ("confirm", future)
                return
        if self._config.features.frame_upload and now >= self._next_preview_ns:
            self._next_preview_ns = now + 2 * _NS_PER_SECOND
            try:
                jpeg, at = self._jpeg(75)
            except (ValueError, RuntimeError):
                return
            future = self._http.submit(self._http.client.upload, jpeg, "preview-" + uuid.uuid4().hex, at, "preview", time.monotonic() + self._config.http.timeout_s)
            self._maintenance = ("preview", future)

    def _evaluate_rules(self) -> None:
        now = self._clock.monotonic_ns()
        if now < self._next_rule_ns:
            return
        update = self._rules.evaluate(self._sampler.state.snapshot(), now, self._clock.utc())
        self._risk_level = update.level
        self._active_events = list(update.active_events)
        self._actuators.apply(update.level)
        for event in update.events:
            self._remember(event)
        period = _hz_to_ns(self._config.runtime.rule_hz)
        self._next_rule_ns += period
        if self._next_rule_ns <= now:
            self._next_rule_ns = now + period

    def _remember(self, event: EdgeEvent) -> None:
        try:
            self._store.append(event)
        except (sqlite3.Error, OSError) as exc:
            self.storage_error = str(exc)
            self._unstored.append(event)
            return
        self.storage_error = None

    def _retry_unstored(self) -> None:
        pending = self._unstored
        self._unstored = []
        for event in pending:
            self._remember(event)

    def _drain(self) -> None:
        for event in self._transport.poll():
            if event == "up":
                self._connected = True
                self.network_error = None
                self._announced = False
            elif event == "down":
                self._connected = False
                self._pending.clear()
        if self._connected and not self._announced:
            self._announce()

    def _announce(self) -> None:
        self._transport.subscribe(commands_topic(self._config.device.id), self._config.mqtt.command_qos)
        if self._configs is not None:
            self._transport.subscribe(f"car/{self._config.device.id}/config", 1)
            self._configs.republish()
        self._publish_online()
        self._flush_events(republish=True)
        self._flush_acks()
        self._flush_config_acks()
        self._announced = True

    def _publish_status_if_due(self, now: int) -> None:
        changed = sensor_statuses(self._sampler.state.snapshot()) != self._sensors
        if changed or now >= self._next_heartbeat_ns:
            self._publish_online()
            self._next_heartbeat_ns = now + self._config.mqtt.status_heartbeat_s * _NS_PER_SECOND

    def _publish_online(self) -> None:
        state = self._sampler.state.snapshot()
        payload = build_online_status(
            state,
            device_id=self._config.device.id,
            timestamp=self._clock.utc(),
            software_version=__version__,
            config_version=self._config.device.config_version,
            mode=self._config.device.mode,
        )
        if self._publish(self._status_message(payload)) is not None:
            self._sensors = sensor_statuses(state)

    def _publish_telemetry(self) -> None:
        state = self._sampler.state.snapshot()
        payload = build_telemetry(
            state,
            device_id=self._config.device.id,
            timestamp=self._clock.utc(),
            seq=self._seq,
            message_id=self._message_ids(),
            distance_scale=self._config.mapping.distance_scale,
            source=self._config.device.mode,
            risk_level=self._risk_level,
            active_events=self._active_events,
        )
        self._seq += 1
        qos = self._config.mqtt.telemetry_qos
        self._publish(MqttMessage(telemetry_topic(self._config.device.id), _encode(payload), qos, False))

    def _flush_events(self, *, republish: bool) -> None:
        if not self._connected:
            return
        if republish:
            self._store.request_republish()
        rows = self._store.unpublished()
        qos = self._config.mqtt.event_qos
        topic = events_topic(self._config.device.id)
        for row in rows:
            if self._publish_confirmed(MqttMessage(topic, row.payload, qos, False)):
                self._store.mark_broker_confirmed(row.event_id)

    def _flush_acks(self) -> None:
        if not self._connected:
            return
        try:
            rows = self._commands.unpublished()
        except sqlite3.Error as exc:
            self.storage_error = str(exc)
            return
        topic = command_acks_topic(self._config.device.id)
        qos = self._config.mqtt.ack_qos
        for row in rows:
            if not self._publish_confirmed(MqttMessage(topic, row.payload, qos, False)):
                continue
            try:
                self._commands.mark_published(row.command_id)
            except sqlite3.Error as exc:
                self.storage_error = str(exc)
                return

    def _status_message(self, payload: dict[str, object]) -> MqttMessage:
        return MqttMessage(
            status_topic(self._config.device.id),
            _encode(payload),
            self._config.mqtt.status_qos,
            True,
        )

    def _publish(self, message: MqttMessage) -> object | None:
        try:
            return self._transport.publish(message)
        except (OSError, RuntimeError):
            self.network_error = "MQTT publish unavailable"
            return None

    def _publish_confirmed(self, message: MqttMessage) -> bool:
        key = (message.topic, message.payload)
        receipt = self._pending.get(key)
        if receipt is None:
            receipt = self._publish(message)
            if receipt is None:
                return False
            self._pending[key] = receipt
        try:
            if not receipt.is_published():
                return False
        except (ValueError, RuntimeError):
            self._pending.pop(key, None)
            return False
        self._pending.pop(key, None)
        return True


def _encode(payload: dict[str, object]) -> bytes:
    return json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode("utf-8")


def _hz_to_ns(hz: int) -> int:
    return _NS_PER_SECOND // hz
