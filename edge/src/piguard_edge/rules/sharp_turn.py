"""Yaw rate and speed together. A missing sample restarts the timer."""

from piguard_edge.config.model import SharpTurnRuleConfig
from piguard_edge.models.sample import GyroReading, Sample
from piguard_edge.rules.alarm import Alarm, Transition
from piguard_edge.rules.reading import as_float, classify

_NS_PER_MS = 1_000_000


class SharpTurnRule:
    event_type = "sharp_turn"

    def __init__(self, config: SharpTurnRuleConfig, gyro_ttl_ms: int, speed_ttl_ms: int) -> None:
        self._config = config
        self._gyro_ttl_ms = gyro_ttl_ms
        self._speed_ttl_ms = speed_ttl_ms
        self.alarm = Alarm()
        self.data: dict[str, object] = {}

    def update(self, gyro: Sample[GyroReading] | None, speed: Sample[float] | None, now_ns: int) -> Transition | None:
        gyro_kind, _gyro_reason = classify(gyro, now_ns, self._gyro_ttl_ms)
        speed_kind, _speed_reason = classify(speed, now_ns, self._speed_ttl_ms)
        if gyro_kind != "ok" or speed_kind != "ok" or gyro is None or speed is None:
            self.alarm.note_invalid()
            return None
        if not isinstance(gyro.value, GyroReading):
            self.alarm.note_invalid()
            return None
        yaw = as_float(gyro.value.yaw_rate)
        speed_kmh = as_float(speed.value)
        if yaw is None or speed_kmh is None:
            self.alarm.note_invalid()
            return None
        self.data = {"yaw_rate_deg_s": yaw, "speed_kmh": speed_kmh}
        target = "warning" if abs(yaw) > self._config.yaw_threshold_deg_s and speed_kmh > self._config.min_speed_kmh else "normal"
        return self.alarm.consider(
            now_ns,
            target,
            enter_ns=self._config.duration_ms * _NS_PER_MS,
            recover_ns=0,
        )
