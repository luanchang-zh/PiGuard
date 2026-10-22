"""Choose the output that wins: an automatic alarm, otherwise a manual test."""

from piguard_edge.actuators.mock import ActuatorState, ManualTest, MockActuator
from piguard_edge.config.model import EdgeConfig


class ActuatorManager:
    def __init__(self, actuator: ActuatorState, buzzer=None) -> None:
        self.actuator = actuator
        self._buzzer = buzzer
        self._error: str | None = None

    @classmethod
    def from_config(cls, config: EdgeConfig) -> "ActuatorManager":
        if config.actuators.driver == "mock":
            return cls(MockActuator())
        if config.actuators.driver == "gpio" and config.actuators.buzzer_bcm is not None:
            from piguard_edge.actuators.gpio import GpioBuzzer
            settings = config.actuators
            return cls(ActuatorState(), GpioBuzzer(settings.gpio_chip, settings.buzzer_bcm,
                                                  settings.buzzer_frequency_hz, settings.buzzer_active_high))
        raise ValueError("actuators driver is not configured")

    @property
    def error(self) -> str | None:
        return self._error or (self._buzzer.error if self._buzzer is not None else None)

    def supports(self, kind: str, mode: str) -> bool:
        if self.error:
            return False
        return kind == "buzzer" if self._buzzer is not None else mode == "mock"

    def _sync(self) -> None:
        if self._buzzer is not None:
            try:
                manual = self.manual
                self._buzzer.set_pattern(self.actuator.buzzer, manual.until_ns if manual else None)
            except Exception as exc:
                self._error = str(exc)

    @property
    def risk_level(self) -> str:
        return self.actuator.risk_level

    @property
    def manual(self) -> ManualTest | None:
        return self.actuator.manual

    def apply(self, risk_level: str) -> None:
        self.actuator.apply_risk(risk_level)
        self._sync()

    def begin_test(self, command_id: str, kind: str, until_ns: int, color: str | None = None) -> bool:
        started = self.actuator.begin_test(command_id, kind, until_ns, color)
        self._sync()
        return started

    def clear_test(self) -> None:
        self.actuator.clear_test()
        self._sync()

    def take_preempted(self) -> list[str]:
        return self.actuator.take_preempted()

    def close(self) -> None:
        if self._buzzer is not None:
            self._buzzer.close()
