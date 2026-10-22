"""Accept a command only after its id is stored. A repeated id does not run again."""

import sqlite3
from datetime import datetime

from piguard_edge.actuators.manager import ActuatorManager
from piguard_edge.drivers.base import Clock
from piguard_edge.protocol.command import (
    CommandProblem,
    DeviceCommand,
    encode_failure,
    encode_success,
    parse_command,
)
from piguard_edge.simulation.engine import ScenarioEngine
from piguard_edge.storage.commands import CommandStore, StoredAck
from piguard_edge.transport.mqtt import MqttMessage

_NS_PER_MS = 1_000_000
_ALARM = {"warning", "danger"}


class CommandHandler:
    def __init__(
        self,
        device_id: str,
        mode: str,
        clock: Clock,
        engine: ScenarioEngine | None,
        actuators: ActuatorManager,
        store: CommandStore,
        snapshot_handler=None,
    ) -> None:
        self._device_id = device_id
        self._mode = mode
        self._clock = clock
        self._engine = engine
        self._actuators = actuators
        self._store = store
        self._snapshot_handler = snapshot_handler
        self._deferred: list[MqttMessage] = []
        self.storage_error: str | None = None

    def recover(self) -> None:
        self._actuators.clear_test()
        for record in self._store.running():
            self._store.finish(record.command_id, "failed", self._failure(
                record.command_id,
                "RESULT_UNCERTAIN",
                "previous test did not finish; it was not run again",
            ))

    def handle(self, messages: list[MqttMessage]) -> None:
        pending = self._deferred
        self._deferred = []
        for message in [*pending, *messages]:
            self._accept(message)

    def tick(self, now_ns: int) -> None:
        for command_id in self._actuators.take_preempted():
            self._finish(command_id, "failed", "DEVICE_BUSY", "automatic warning preempted the test")
        manual = self._actuators.manual
        if manual is None:
            return
        if self._actuators.error:
            self._actuators.clear_test()
            self._finish(manual.command_id, "failed", "ACTUATOR_UNAVAILABLE", "GPIO output failed")
            return
        if now_ns < manual.until_ns:
            return
        if manual.kind == "indicator":
            result = {"color": manual.color, "message": "indicator test completed"}
        else:
            result = {"message": "buzzer test completed"}
        self._actuators.clear_test()
        if self._actuators.error:
            self._finish(manual.command_id, "failed", "ACTUATOR_UNAVAILABLE", "GPIO output failed")
        else:
            self._finish(manual.command_id, "success", result=result)

    def cancel_for_shutdown(self) -> None:
        manual = self._actuators.manual
        if manual is not None:
            self._finish(manual.command_id, "failed", "CANCELLED", "test output cancelled during shutdown")
        self._actuators.clear_test()

    def close(self) -> None:
        self._store.close()

    def unpublished(self) -> list[StoredAck]:
        return self._store.unpublished()

    def mark_published(self, command_id: str) -> None:
        self._store.mark_published(command_id)

    def _accept(self, message: MqttMessage) -> None:
        if not message.topic.endswith("/commands"):
            return
        try:
            self._handle_one(message)
        except sqlite3.Error as exc:
            self.storage_error = str(exc)
            self._deferred.append(message)

    def _handle_one(self, message: MqttMessage) -> None:
        parsed = parse_command(message.topic, message.payload, device_id=self._device_id)
        if parsed is None:
            return
        if isinstance(parsed, CommandProblem):
            self._reject_new(parsed.command_id or "", parsed.fingerprint or "", parsed.code, parsed.message)
            return
        existing = self._store.get(parsed.command_id)
        if existing is not None:
            if existing.fingerprint == parsed.fingerprint and existing.ack is not None:
                self._store.request_republish(parsed.command_id)
            return
        refusal = self._refusal(parsed)
        if refusal is not None:
            code, text = refusal
            self._store.insert_finished(parsed.command_id, parsed.device_id, parsed.fingerprint, "failed", self._failure(
                parsed.command_id, code, text
            ))
            self.storage_error = None
            return
        if parsed.command_type in ("buzzer.test", "indicator.test"):
            self._start_test(parsed)
            return
        if parsed.command_type == "camera.snapshot":
            self._store.insert_running(parsed.command_id, parsed.device_id, parsed.fingerprint)
            self._snapshot_handler(parsed)
            return
        self._run_scenario(parsed)

    def finish_snapshot(self, command_id: str, snapshot_id: str | None, code: str = "SNAPSHOT_UPLOAD_FAILED") -> None:
        if snapshot_id is not None:
            self._finish(command_id, "success", result={"snapshot_id": snapshot_id})
        else:
            self._finish(command_id, "failed", code, "snapshot could not be completed")

    def _refusal(self, command: DeviceCommand) -> tuple[str, str] | None:
        now = self._clock.utc()
        if now >= command.expires_at:
            return "COMMAND_EXPIRED", "command expired before execution"
        if command.command_type == "camera.snapshot" and self._snapshot_handler is None:
            return "UNSUPPORTED_COMMAND", "frame upload is disabled"
        if command.command_type in ("scenario.start", "scenario.stop") and self._mode != "mock":
            return "DEVICE_NOT_SUPPORTED", "scenario commands are not available in hardware mode"
        if command.command_type in ("scenario.start", "scenario.stop") and self._engine is None:
            return "DEVICE_NOT_SUPPORTED", "scenario engine is not available"
        if command.command_type in ("buzzer.test", "indicator.test"):
            kind = "buzzer" if command.command_type == "buzzer.test" else "indicator"
            if not self._actuators.supports(kind, self._mode):
                return "ACTUATOR_UNAVAILABLE", "hardware actuator is not configured"
            if self._actuators.risk_level in _ALARM or self._actuators.manual is not None:
                return "DEVICE_BUSY", "automatic warning is active" if self._actuators.risk_level in _ALARM else "another test is still running"
            duration = command.params["duration_ms"]
            if not isinstance(duration, int):
                return "INVALID_PARAMS", "duration_ms must be an integer from 100 to 5000"
            if duration > _remaining_ms(now, command.expires_at):
                return "COMMAND_EXPIRED", "command cannot finish before it expires"
        return None

    def _start_test(self, command: DeviceCommand) -> None:
        duration = command.params["duration_ms"]
        if not isinstance(duration, int):
            return
        until_ns = self._clock.monotonic_ns() + duration * _NS_PER_MS
        self._store.insert_running(command.command_id, command.device_id, command.fingerprint)
        color = command.params.get("color")
        color_text = color if isinstance(color, str) else None
        kind = "indicator" if command.command_type == "indicator.test" else "buzzer"
        started = self._actuators.begin_test(command.command_id, kind, until_ns, color_text)
        if not started:
            self._store.finish(command.command_id, "failed", self._failure(
                command.command_id, "DEVICE_BUSY", "another test is still running"
            ))
        self.storage_error = None

    def _run_scenario(self, command: DeviceCommand) -> None:
        if self._engine is None:
            return
        self._store.insert_running(command.command_id, command.device_id, command.fingerprint)
        if command.command_type == "scenario.stop":
            self._engine.stop()
            result = {"scenario": self._engine.name, "speed": self._engine.playback_speed}
        else:
            scenario = command.params["scenario"]
            speed = command.params["speed"]
            if not isinstance(scenario, str) or not isinstance(speed, float):
                self._store.finish(command.command_id, "failed", self._failure(
                    command.command_id, "INVALID_PARAMS", "scenario.start params must be scenario and speed"
                ))
                self.storage_error = None
                return
            self._engine.start(scenario, speed)
            result = {"scenario": scenario, "speed": speed}
        self._store.finish(
            command.command_id,
            "success",
            encode_success(command.command_id, command.device_id, self._clock.utc(), result),
        )
        self.storage_error = None

    def _reject_new(self, command_id: str, fingerprint: str, code: str, message: str) -> None:
        if not command_id or not fingerprint:
            return
        existing = self._store.get(command_id)
        if existing is not None:
            if existing.fingerprint == fingerprint and existing.ack is not None:
                self._store.request_republish(command_id)
            return
        self._store.insert_finished(command_id, self._device_id, fingerprint, "failed", self._failure(command_id, code, message))
        self.storage_error = None

    def _finish(self, command_id: str, status: str, code: str = "", message: str = "", result: dict[str, object] | None = None) -> None:
        record = self._store.get(command_id)
        if record is None or record.status != "running":
            return
        if status == "success":
            ack = encode_success(command_id, self._device_id, self._clock.utc(), result or {})
        else:
            ack = self._failure(command_id, code, message)
        self._store.finish(command_id, status, ack)

    def _failure(self, command_id: str, code: str, message: str) -> bytes:
        return encode_failure(command_id, self._device_id, self._clock.utc(), code, message)


def _remaining_ms(now: datetime, expires_at: datetime) -> int:
    delta = expires_at - now
    return delta.days * 86_400_000 + delta.seconds * 1000 + delta.microseconds // 1000
