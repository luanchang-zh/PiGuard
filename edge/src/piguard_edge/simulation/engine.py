"""One scenario clock shared by every mock source."""

from piguard_edge.simulation.scenarios import SCENARIO_NAMES, Scene, scene_at

PLAYBACK_SPEEDS = (0.5, 1.0, 2.0)


class ScenarioEngine:
    def __init__(self, name: str, playback_speed: float, origin_ns: int = 0) -> None:
        if name not in SCENARIO_NAMES:
            raise ValueError(f"unknown scenario: {name}")
        if playback_speed not in PLAYBACK_SPEEDS:
            raise ValueError(f"playback_speed must be one of {PLAYBACK_SPEEDS}")
        self._name = name
        self._playback_speed = playback_speed
        self._origin_ns = origin_ns
        self._anchor_next = False

    @property
    def name(self) -> str:
        return self._name

    @property
    def playback_speed(self) -> float:
        return self._playback_speed

    def stop(self) -> None:
        self._name = "normal_drive"
        self._anchor_next = True

    def observe(self, monotonic_ns: int) -> Scene:
        if self._anchor_next:
            self._origin_ns = monotonic_ns
            self._anchor_next = False
        elapsed_ns = monotonic_ns - self._origin_ns
        return scene_at(self._name, elapsed_ns, self._playback_speed)
