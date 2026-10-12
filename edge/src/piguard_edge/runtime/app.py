"""Sample locally and publish the latest telemetry and status."""

import json
import threading
import time
import uuid
from collections.abc import Callable

from piguard_edge import __version__
from piguard_edge.config.model import EdgeConfig
from piguard_edge.drivers.base import Clock
from piguard_edge.protocol.status import build_online_status, build_shutdown_status, build_will, sensor_statuses
from piguard_edge.protocol.telemetry import build_telemetry
from piguard_edge.protocol.topics import commands_topic, status_topic, telemetry_topic
from piguard_edge.runtime.workers import Sampler
from piguard_edge.simulation.engine import ScenarioEngine
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
    ) -> None:
        self._config = config
        self._clock = clock
        self._transport = transport
        self._sampler = sampler
        self._message_ids = message_ids or (lambda: str(uuid.uuid4()))
        self._seq = 0
        self._connected = False
        self._announced = False
        self._sensors: dict[str, str] | None = None
        now = clock.monotonic_ns()
        self._next_telemetry_ns = now + _hz_to_ns(config.runtime.telemetry_hz)
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
        return cls(config, clock, transport, sampler, message_ids)

    def start(self) -> None:
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
        self._publish_status_if_due(now)

    def stop(self) -> None:
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
        )
        self._seq += 1
        qos = self._config.mqtt.telemetry_qos
        self._publish(MqttMessage(telemetry_topic(self._config.device.id), _encode(payload), qos, False))

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
