"""Sample, judge, and publish the latest telemetry, status, and events."""

import json
import sqlite3
import threading
import time
import uuid
from collections.abc import Callable
from datetime import timedelta

from piguard_edge import __version__
from piguard_edge.actuators.manager import ActuatorManager
from piguard_edge.config.model import EdgeConfig
from piguard_edge.drivers.base import Clock
from piguard_edge.models.sample import format_utc
from piguard_edge.protocol.event import EdgeEvent
from piguard_edge.protocol.status import build_online_status, build_shutdown_status, build_will, sensor_statuses
from piguard_edge.protocol.telemetry import build_telemetry
from piguard_edge.protocol.topics import commands_topic, events_topic, status_topic, telemetry_topic
from piguard_edge.rules.engine import RuleEngine
from piguard_edge.runtime.workers import Sampler
from piguard_edge.simulation.engine import ScenarioEngine
from piguard_edge.storage.events import EventStore
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
    ) -> None:
        self._config = config
        self._clock = clock
        self._transport = transport
        self._sampler = sampler
        self._message_ids = message_ids or (lambda: str(uuid.uuid4()))
        self._rules = rules if rules is not None else RuleEngine.from_config(config)
        self._store = store if store is not None else EventStore(config.storage.sqlite_path)
        self._actuators = actuators if actuators is not None else ActuatorManager.from_config(config)
        self._seq = 0
        self._connected = False
        self._announced = False
        self._sensors: dict[str, str] | None = None
        self._risk_level = "unknown"
        self._active_events: list[str] = []
        self._unstored: list[EdgeEvent] = []
        self.storage_error: str | None = None
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
        sampler = Sampler.from_config(config, engine, clock)
        return cls(
            config,
            clock,
            transport,
            sampler,
            message_ids,
            RuleEngine.from_config(config),
            EventStore(config.storage.sqlite_path),
            ActuatorManager.from_config(config),
        )

    def start(self) -> None:
        self._purge_confirmed()
        device_id = self._config.device.id
        will = build_will(device_id=device_id)
        self._transport.set_will(self._status_message(will))
        try:
            self._transport.connect()
        except OSError:
            return
        self._drain()

    def step(self) -> None:
        self._sampler.step()
        self._retry_unstored()
        self._evaluate_rules()
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
        self._publish_status_if_due(now)

    def stop(self) -> None:
        try:
            if self._connected:
                self._publish(self._status_message(build_shutdown_status(
                    self._sampler.state.snapshot(),
                    device_id=self._config.device.id,
                    timestamp=self._clock.utc(),
                    software_version=__version__,
                    config_version=self._config.device.config_version,
                    mode=self._config.device.mode,
                )))
            try:
                self._transport.disconnect()
            except OSError:
                return
        finally:
            self._store.close()

    def run(self, sleep: Callable[[float], None] | None = None, stop: threading.Event | None = None) -> None:
        pause = sleep or time.sleep
        done = stop or threading.Event()
        self.start()
        try:
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
                self._announced = False
            elif event == "down":
                self._connected = False
        if self._connected and not self._announced:
            self._announce()

    def _announce(self) -> None:
        self._transport.subscribe(commands_topic(self._config.device.id), self._config.mqtt.command_qos)
        self._publish_online()
        self._flush_events(republish=True)
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
        self._publish(self._status_message(payload))
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
        rows = self._store.unconfirmed() if republish else self._store.unpublished()
        qos = self._config.mqtt.event_qos
        topic = events_topic(self._config.device.id)
        for row in rows:
            self._publish(MqttMessage(topic, row.payload, qos, False))
            self._store.mark_broker_confirmed(row.event_id)

    def _status_message(self, payload: dict[str, object]) -> MqttMessage:
        return MqttMessage(
            status_topic(self._config.device.id),
            _encode(payload),
            self._config.mqtt.status_qos,
            True,
        )

    def _publish(self, message: MqttMessage) -> None:
        self._transport.publish(message)


def _encode(payload: dict[str, object]) -> bytes:
    return json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode("utf-8")


def _hz_to_ns(hz: int) -> int:
    return _NS_PER_SECOND // hz
