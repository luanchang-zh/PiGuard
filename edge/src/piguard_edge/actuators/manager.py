"""Choose the output that wins: an automatic alarm, otherwise a manual test."""

from piguard_edge.actuators.mock import ManualTest, MockActuator
from piguard_edge.config.model import EdgeConfig


class ActuatorManager:
    def __init__(self, actuator: MockActuator) -> None:
        self.actuator = actuator

    @classmethod
    def from_config(cls, config: EdgeConfig) -> "ActuatorManager":
        if config.actuators.driver != "mock":
            raise ValueError("actuators only run driver=mock")
        return cls(MockActuator())

    @property
    def risk_level(self) -> str:
        return self.actuator.risk_level

    @property
    def manual(self) -> ManualTest | None:
        return self.actuator.manual

    def apply(self, risk_level: str) -> None:
        self.actuator.apply_risk(risk_level)

    def begin_test(self, command_id: str, kind: str, until_ns: int, color: str | None = None) -> bool:
        return self.actuator.begin_test(command_id, kind, until_ns, color)

    def clear_test(self) -> None:
        self.actuator.clear_test()

    def take_preempted(self) -> list[str]:
        return self.actuator.take_preempted()
