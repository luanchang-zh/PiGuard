"""Encode the latest samples as one telemetry document.

Publishing again does not rewrite sample_at. A failed reading keeps its
object and uses null for the numbers.
"""

from datetime import datetime

from piguard_edge.models.frame import Frame
from piguard_edge.models.sample import (
    UNIT_C,
    UNIT_DPS,
    UNIT_KMH,
    UNIT_M,
    DistanceReading,
    GyroReading,
    Sample,
    SampleStatus,
    format_utc,
)
from piguard_edge.runtime.state import LatestState


def build_telemetry(
    state: LatestState,
    *,
    device_id: str,
    timestamp: datetime,
    seq: int,
    message_id: str,
    distance_scale: float,
    source: str,
    risk_level: str = "unknown",
    active_events: list[str] | None = None,
) -> dict[str, object]:
    if seq < 0:
        raise ValueError("seq must be non-negative")
    return {
        "schema_version": 1,
        "message_id": message_id,
        "device_id": device_id,
        "timestamp": format_utc(timestamp),
        "seq": seq,
        "speed": _scalar(state.speed, UNIT_KMH, source),
        "distance": _distance(state.distance, distance_scale, source),
        "temperature": _scalar(state.temperature, UNIT_C, source),
        "gyro": _gyro(state.gyro, source),
        "lane": _lane(state.frame),
        "risk": {"level": risk_level, "active_events": list(active_events or [])},
    }


def _scalar(sample: Sample[float] | None, unit: str, source: str) -> dict[str, object]:
    if sample is None:
        return {"value": None, "unit": unit, "source": source, "status": "unavailable", "sample_at": None}
    value = sample.value if sample.status is SampleStatus.OK else None
    return {
        "value": value,
        "unit": sample.unit,
        "source": sample.source,
        "status": sample.status.value,
        "sample_at": format_utc(sample.sample_at),
    }


def _distance(sample: Sample[DistanceReading] | None, scale: float, source: str) -> dict[str, object]:
    if sample is None:
        return _distance_body(None, None, scale, source, "unavailable", None)
    if sample.status is not SampleStatus.OK or sample.value is None:
        return _distance_body(None, None, scale, sample.source, sample.status.value, format_utc(sample.sample_at))
    reading = sample.value
    return _distance_body(
        reading.raw_value,
        reading.mapped_value,
        reading.scale,
        sample.source,
        sample.status.value,
        format_utc(sample.sample_at),
    )


def _distance_body(
    raw: float | None,
    mapped: float | None,
    scale: float,
    source: str,
    status: str,
    sample_at: str | None,
) -> dict[str, object]:
    return {
        "raw_value": raw,
        "mapped_value": mapped,
        "unit": UNIT_M,
        "scale": scale,
        "source": source,
        "status": status,
        "sample_at": sample_at,
    }


def _gyro(sample: Sample[GyroReading] | None, source: str) -> dict[str, object]:
    if sample is None or sample.status is not SampleStatus.OK or sample.value is None:
        status = "unavailable" if sample is None else sample.status.value
        origin = source if sample is None else sample.source
        sample_at = None if sample is None else format_utc(sample.sample_at)
        return {
            "x": None,
            "y": None,
            "z": None,
            "yaw_rate": None,
            "unit": UNIT_DPS,
            "source": origin,
            "status": status,
            "sample_at": sample_at,
        }
    reading = sample.value
    return {
        "x": reading.x,
        "y": reading.y,
        "z": reading.z,
        "yaw_rate": reading.yaw_rate,
        "unit": sample.unit,
        "source": sample.source,
        "status": sample.status.value,
        "sample_at": format_utc(sample.sample_at),
    }


def _lane(frame: Frame | None) -> dict[str, object]:
    if frame is None or frame.status is not SampleStatus.OK or not frame.lane.valid:
        sample_at = None if frame is None else format_utc(frame.lane.sample_at)
        return {
            "valid": False,
            "offset_ratio": None,
            "direction": "unknown",
            "confidence": None,
            "sample_at": sample_at,
        }
    lane = frame.lane
    return {
        "valid": True,
        "offset_ratio": lane.offset_ratio,
        "direction": lane.direction,
        "confidence": lane.confidence,
        "sample_at": format_utc(lane.sample_at),
    }
