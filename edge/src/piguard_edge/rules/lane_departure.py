"""Lane offset must stay past the threshold for the configured duration."""

from piguard_edge.config.model import LaneRuleConfig
from piguard_edge.models.frame import Frame
from piguard_edge.rules.alarm import Alarm, Transition

_NS_PER_MS = 1_000_000


class LaneDepartureRule:
    event_type = "lane_departure"

    def __init__(self, config: LaneRuleConfig) -> None:
        self._config = config
        self.alarm = Alarm()
        self.data: dict[str, object] = {}

    def update(self, frame: Frame, now_ns: int) -> Transition | None:
        lane = frame.lane
        offset = lane.offset_ratio
        if offset is None:
            self.alarm.note_invalid()
            return None
        self.data = {"offset_ratio": offset, "direction": lane.direction}
        target = "warning" if abs(offset) > self._config.offset_threshold else "normal"
        return self.alarm.consider(
            now_ns,
            target,
            enter_ns=self._config.duration_ms * _NS_PER_MS,
            recover_ns=0,
        )
