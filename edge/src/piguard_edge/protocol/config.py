"""Validate the platform's complete v1 rule set before applying it."""

import json
import math
from dataclasses import replace
from datetime import datetime

from piguard_edge.config.model import RulesConfig

FIELDS = {
    "obstacle": {"warning_distance_m", "danger_distance_m"},
    "lane_departure": {"offset_threshold", "duration_ms"},
    "sharp_turn": {"yaw_threshold_deg_s", "min_speed_kmh", "duration_ms"},
    "temperature": {"trigger_c", "recover_c", "duration_ms"},
}


def merge_rules(base: RulesConfig, rules: object) -> RulesConfig:
    if not isinstance(rules, dict) or set(rules) != set(FIELDS):
        raise ValueError("rules must contain all four groups")
    for group, fields in FIELDS.items():
        values = rules[group]
        if not isinstance(values, dict) or set(values) != fields:
            raise ValueError(f"invalid rules.{group} fields")
        for key, value in values.items():
            try:
                finite = not isinstance(value, bool) and isinstance(value, (int, float)) and math.isfinite(value)
            except OverflowError:
                finite = False
            if not finite:
                raise ValueError(f"invalid rules.{group}.{key}")
            if key == "duration_ms" and (not isinstance(value, int) or not 0 < value <= 2**63 - 1):
                raise ValueError("duration_ms must be a positive integer")
    o, l, s, t = (rules[name] for name in FIELDS)
    if not 0 < o["danger_distance_m"] < o["warning_distance_m"]:
        raise ValueError("invalid obstacle distances")
    if not 0 < l["offset_threshold"] <= 1:
        raise ValueError("invalid lane threshold")
    if s["yaw_threshold_deg_s"] <= 0 or s["min_speed_kmh"] < 0:
        raise ValueError("invalid sharp turn thresholds")
    if t["recover_c"] >= t["trigger_c"]:
        raise ValueError("invalid temperature thresholds")
    return replace(base, **{name: replace(getattr(base, name), **rules[name]) for name in FIELDS})


def parse_config(topic: str, payload: bytes, device_id: str) -> dict | None:
    try:
        body = json.loads(payload)
        if not isinstance(body, dict) or set(body) != {"schema_version", "device_id", "config_version", "issued_at", "rules"}:
            return None
        version = body["config_version"]
        if type(body["schema_version"]) is not int or body["schema_version"] != 1 or body["device_id"] != device_id or topic != f"car/{device_id}/config":
            return None
        if type(version) is not int or not 0 < version <= 2**63 - 1:
            return None
        at = datetime.fromisoformat(body["issued_at"].replace("Z", "+00:00"))
        if at.utcoffset() is None:
            return None
        return body
    except (ValueError, TypeError, KeyError, AttributeError, OverflowError):
        return None
