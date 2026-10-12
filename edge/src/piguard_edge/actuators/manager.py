"""Apply the current risk to the mock output."""

from piguard_edge.actuators.mock import MockActuator
from piguard_edge.config.model import EdgeConfig


class ActuatorManager:
    def __init__(self, actuator: MockActuator) -> None:
        self.actuator = actuator

    @classmethod
    def from_config(cls, config: EdgeConfig) -> "ActuatorManager":
        if config.actuators.driver != "mock":
            raise ValueError("actuators only run driver=mock")
        return cls(MockActuator())

    def apply(self, risk_level: str) -> None:
        self.actuator.apply_risk(risk_level)
