"""Simulated lamp and buzzer. Automatic alarms replace a manual test."""

from dataclasses import dataclass

_AUTOMATIC = {
    "danger": ("red", "fast"),
    "warning": ("yellow", "intermittent"),
}


@dataclass(frozen=True, slots=True)
class ManualTest:
    command_id: str
    kind: str
    color: str | None
    until_ns: int


class ActuatorState:
    def __init__(self) -> None:
        self.risk_level = "unknown"
        self.indicator = "off"
        self.buzzer = "off"
        self._manual: ManualTest | None = None
        self._preempted: list[str] = []

    @property
    def manual(self) -> ManualTest | None:
        return self._manual

    def apply_risk(self, level: str) -> None:
        self.risk_level = level
        if level in _AUTOMATIC and self._manual is not None:
            self._preempted.append(self._manual.command_id)
            self._manual = None
        self._render()

    def begin_test(self, command_id: str, kind: str, until_ns: int, color: str | None = None) -> bool:
        if self.risk_level in _AUTOMATIC or self._manual is not None:
            return False
        self._manual = ManualTest(command_id, kind, color, until_ns)
        self._render()
        return True

    def clear_test(self) -> None:
        self._manual = None
        self._render()

    def take_preempted(self) -> list[str]:
        found = self._preempted
        self._preempted = []
        return found

    def _render(self) -> None:
        if self.risk_level in _AUTOMATIC:
            self.indicator, self.buzzer = _AUTOMATIC[self.risk_level]
            return
        if self._manual is not None and self._manual.kind == "buzzer":
            self.indicator = "green" if self.risk_level == "normal" else "off"
            self.buzzer = "on"
            return
        if self._manual is not None and self._manual.kind == "indicator":
            self.indicator = self._manual.color or "off"
            self.buzzer = "off"
            return
        if self.risk_level == "normal":
            self.indicator, self.buzzer = "green", "off"
            return
        self.indicator, self.buzzer = "off", "off"


class MockActuator(ActuatorState):
    """The shared arbitration state without a physical output."""
