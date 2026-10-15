"""Distance and headway. The mapped metre is used once; speed 0 is not divided."""

from piguard_edge.config.model import ObstacleRuleConfig
from piguard_edge.models.sample import DistanceReading, Sample
from piguard_edge.rules.alarm import Alarm, Transition
from piguard_edge.rules.reading import as_float, classify

_RANK = {"normal": 0, "warning": 1, "danger": 2}


class ObstacleRule:
    event_type = "obstacle_warning"

    def __init__(self, config: ObstacleRuleConfig, distance_ttl_ms: int, speed_ttl_ms: int) -> None:
        self._config = config
        self._distance_ttl_ms = distance_ttl_ms
        self._speed_ttl_ms = speed_ttl_ms
        self.alarm = Alarm()
        self.data: dict[str, object] = {}

    def update(
        self,
        distance: Sample[DistanceReading] | None,
        speed: Sample[float] | None,
        now_ns: int,
    ) -> Transition | None:
        kind, _reason = classify(distance, now_ns, self._distance_ttl_ms)
        if kind != "ok" or distance is None or not isinstance(distance.value, DistanceReading):
            self.alarm.note_invalid()
            return None
        mapped = distance.value.mapped_value
        speed_kind, _speed_reason = classify(speed, now_ns, self._speed_ttl_ms)
        speed_kmh = as_float(speed.value) if speed_kind == "ok" and speed is not None else None
        headway = _headway(mapped, speed_kmh)
        target = _higher(_distance_level(mapped, self._config), _headway_level(headway, self._config))
        self.data = {"distance_m": mapped, "speed_kmh": speed_kmh, "headway_s": headway}
        return self.alarm.consider(now_ns, target, enter_ns=0, recover_ns=0)


def _headway(distance_m: float, speed_kmh: float | None) -> float | None:
    if speed_kmh is None or speed_kmh <= 0:
        return None
    return distance_m / (speed_kmh / 3.6)


def _distance_level(distance_m: float, config: ObstacleRuleConfig) -> str:
    if distance_m <= config.danger_distance_m:
        return "danger"
    if distance_m <= config.warning_distance_m:
        return "warning"
    return "normal"


def _headway_level(headway_s: float | None, config: ObstacleRuleConfig) -> str:
    if headway_s is None or not config.headway_enabled:
        return "normal"
    if headway_s < config.danger_headway_s:
        return "danger"
    if headway_s < config.warning_headway_s:
        return "warning"
    return "normal"


def _higher(left: str, right: str) -> str:
    return left if _RANK[left] >= _RANK[right] else right
