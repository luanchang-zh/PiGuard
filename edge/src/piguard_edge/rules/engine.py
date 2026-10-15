"""Run the four warning rules and report one risk level."""

import uuid
from collections.abc import Callable
from dataclasses import dataclass
from datetime import datetime

from piguard_edge.config.model import EdgeConfig, TtlConfig
from piguard_edge.protocol.event import EdgeEvent, make_event
from piguard_edge.rules.alarm import Transition
from piguard_edge.rules.lane_departure import LaneDepartureRule
from piguard_edge.rules.obstacle import ObstacleRule
from piguard_edge.rules.reading import classify, classify_frame
from piguard_edge.rules.sensors import SensorWatch
from piguard_edge.rules.sharp_turn import SharpTurnRule
from piguard_edge.rules.temperature import TemperatureRule
from piguard_edge.runtime.state import LatestState

_RANK = {"warning": 1, "danger": 2}
_SENSOR_NAMES = ("distance", "camera", "gyro", "temperature")


@dataclass(frozen=True, slots=True)
class RiskUpdate:
    level: str
    active_events: tuple[str, ...]
    events: tuple[EdgeEvent, ...]


class RuleEngine:
    def __init__(
        self,
        device_id: str,
        obstacle: ObstacleRule,
        lane: LaneDepartureRule,
        sharp_turn: SharpTurnRule,
        temperature: TemperatureRule,
        ttl: TtlConfig,
        event_ids: Callable[[], str],
    ) -> None:
        self._device_id = device_id
        self._obstacle = obstacle
        self._lane = lane
        self._sharp_turn = sharp_turn
        self._temperature = temperature
        self._ttl = ttl
        self._event_ids = event_ids
        self._watches = {name: SensorWatch(name) for name in _SENSOR_NAMES}

    @classmethod
    def from_config(cls, config: EdgeConfig, event_ids: Callable[[], str] | None = None) -> "RuleEngine":
        ttl = config.runtime.ttl_ms
        rules = config.rules
        return cls(
            config.device.id,
            ObstacleRule(rules.obstacle, ttl.distance, ttl.speed),
            LaneDepartureRule(rules.lane_departure),
            SharpTurnRule(rules.sharp_turn, ttl.gyro, ttl.speed),
            TemperatureRule(rules.temperature, ttl.temperature),
            ttl,
            event_ids or (lambda: str(uuid.uuid4())),
        )

    def evaluate(self, state: LatestState, now_ns: int, timestamp: datetime) -> RiskUpdate:
        events: list[EdgeEvent] = []
        self._emit(events, self._obstacle.event_type, self._obstacle.update(state.distance, state.speed, now_ns), self._obstacle.data, timestamp)
        self._watch(events, "distance", state, now_ns, timestamp)
        self._emit_lane(events, state, now_ns, timestamp)
        self._emit(events, self._sharp_turn.event_type, self._sharp_turn.update(state.gyro, state.speed, now_ns), self._sharp_turn.data, timestamp)
        self._watch_sample(events, "gyro", classify(state.gyro, now_ns, self._ttl.gyro), now_ns, timestamp)
        self._emit(events, self._temperature.event_type, self._temperature.update(state.temperature, now_ns), self._temperature.data, timestamp)
        self._watch_sample(events, "temperature", classify(state.temperature, now_ns, self._ttl.temperature), now_ns, timestamp)
        return RiskUpdate(self._level(), self._active(), tuple(events))

    def reconfigure(self, config: EdgeConfig) -> None:
        # Preserve active alarms so a new threshold can emit their recovery.
        for rule, settings in (
            (self._obstacle, config.rules.obstacle),
            (self._lane, config.rules.lane_departure),
            (self._sharp_turn, config.rules.sharp_turn),
            (self._temperature, config.rules.temperature),
        ):
            rule._config = settings
            rule.alarm.note_invalid()

    def _emit_lane(self, events: list[EdgeEvent], state: LatestState, now_ns: int, timestamp: datetime) -> None:
        kind, reason = classify_frame(state.frame, now_ns, self._ttl.lane)
        self._watch_sample(events, "camera", (kind, reason), now_ns, timestamp)
        frame = state.frame
        if kind != "ok" or frame is None or not frame.lane.valid:
            self._lane.alarm.note_invalid()
            return
        self._emit(events, self._lane.event_type, self._lane.update(frame, now_ns), self._lane.data, timestamp)

    def _watch(self, events: list[EdgeEvent], name: str, state: LatestState, now_ns: int, timestamp: datetime) -> None:
        sample = state.distance if name == "distance" else None
        self._watch_sample(events, name, classify(sample, now_ns, self._ttl.distance), now_ns, timestamp)

    def _watch_sample(
        self,
        events: list[EdgeEvent],
        name: str,
        reading: tuple[str, str],
        now_ns: int,
        timestamp: datetime,
    ) -> None:
        kind, reason = reading
        watch = self._watches[name]
        transition = watch.update(kind, reason, now_ns)
        if transition is None:
            return
        self._emit(events, "sensor_failure", transition, watch.event_data(transition), timestamp)

    def _emit(
        self,
        events: list[EdgeEvent],
        event_type: str,
        transition: Transition | None,
        data: dict[str, object],
        timestamp: datetime,
    ) -> None:
        if transition is None:
            return
        events.append(make_event(
            device_id=self._device_id,
            event_id=self._event_ids(),
            event_type=event_type,
            action=transition.action,
            level=transition.level,
            timestamp=timestamp,
            data=dict(data),
        ))

    def _active(self) -> tuple[str, ...]:
        names = []
        if self._obstacle.alarm.active:
            names.append(self._obstacle.event_type)
        if self._lane.alarm.active:
            names.append(self._lane.event_type)
        if self._sharp_turn.alarm.active:
            names.append(self._sharp_turn.event_type)
        if self._temperature.alarm.active:
            names.append(self._temperature.event_type)
        if any(self._watches[name].alarm.active for name in _SENSOR_NAMES):
            names.append("sensor_failure")
        return tuple(names)

    def _level(self) -> str:
        rank = 0
        for alarm in (
            self._obstacle.alarm,
            self._lane.alarm,
            self._sharp_turn.alarm,
            self._temperature.alarm,
        ):
            if alarm.active and alarm.level is not None:
                rank = max(rank, _RANK[alarm.level])
        if any(self._watches[name].alarm.active for name in _SENSOR_NAMES):
            rank = max(rank, _RANK["warning"])
        if rank == _RANK["danger"]:
            return "danger"
        if rank == _RANK["warning"]:
            return "warning"
        alarms = (self._obstacle.alarm, self._lane.alarm, self._sharp_turn.alarm, self._temperature.alarm)
        if all(alarm.usable and not alarm.active for alarm in alarms):
            return "normal"
        return "unknown"
