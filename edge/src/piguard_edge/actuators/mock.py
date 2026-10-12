"""Simulated lamp and buzzer. No GPIO pins are assigned."""

_OUTPUT = {
    "danger": ("red", "fast"),
    "warning": ("yellow", "intermittent"),
    "normal": ("green", "off"),
    "unknown": ("off", "off"),
}


class MockActuator:
    def __init__(self) -> None:
        self.indicator = "off"
        self.buzzer = "off"

    def apply_risk(self, level: str) -> None:
        self.indicator, self.buzzer = _OUTPUT[level]
