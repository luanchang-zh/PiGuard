"""Encode one state change as a v1 event. Replay keeps this payload."""

import json
from dataclasses import dataclass
from datetime import datetime

from piguard_edge.models.sample import format_utc

_ACTIONS = {"started", "level_changed", "recovered"}
_LEVELS = {"warning", "danger"}
_TYPES = {"obstacle_warning", "lane_departure", "sharp_turn", "high_temperature", "sensor_failure"}


@dataclass(frozen=True, slots=True)
class EdgeEvent:
    event_id: str
    device_id: str
    event_type: str
    action: str
    level: str
    timestamp: datetime
    data: dict[str, object]
    payload: bytes


def make_event(
    *,
    device_id: str,
    event_id: str,
    event_type: str,
    action: str,
    level: str,
    timestamp: datetime,
    data: dict[str, object],
) -> EdgeEvent:
    if event_type not in _TYPES or action not in _ACTIONS or level not in _LEVELS:
        raise ValueError("event type, action, or level is not in the v1 contract")
    if not event_id or any(character.isspace() for character in event_id):
        raise ValueError("event_id must be a token")
    body = {
        "schema_version": 1,
        "event_id": event_id,
        "device_id": device_id,
        "type": event_type,
        "action": action,
        "level": level,
        "timestamp": format_utc(timestamp),
        "data": data,
    }
    payload = json.dumps(body, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    return EdgeEvent(event_id, device_id, event_type, action, level, timestamp, dict(data), payload)
