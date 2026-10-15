"""Online status, clean shutdown, and the offline will."""

from datetime import datetime

from piguard_edge.models.sample import SampleStatus, format_utc
from piguard_edge.runtime.state import LatestState


def sensor_statuses(state: LatestState) -> dict[str, str]:
    return {
        "distance": _sample_status(state.distance),
        "camera": "unavailable" if state.frame is None else state.frame.status.value,
        "gyro": _sample_status(state.gyro),
        "temperature": _sample_status(state.temperature),
    }


def build_online_status(
    state: LatestState,
    *,
    device_id: str,
    timestamp: datetime,
    software_version: str,
    config_version: int,
    mode: str,
) -> dict[str, object]:
    return {
        "schema_version": 1,
        "device_id": device_id,
        "online": True,
        "timestamp": format_utc(timestamp),
        "software_version": software_version,
        "config_version": config_version,
        "mode": mode,
        "sensors": sensor_statuses(state),
        "mqtt": {"connected": True},
    }


def build_shutdown_status(
    state: LatestState,
    *,
    device_id: str,
    timestamp: datetime,
    software_version: str,
    config_version: int,
    mode: str,
) -> dict[str, object]:
    return {
        "schema_version": 1,
        "device_id": device_id,
        "online": False,
        "timestamp": format_utc(timestamp),
        "software_version": software_version,
        "config_version": config_version,
        "mode": mode,
        "sensors": sensor_statuses(state),
        "mqtt": {"connected": False},
        "reason": "shutdown",
    }


def build_will(*, device_id: str) -> dict[str, object]:
    return {
        "schema_version": 1,
        "device_id": device_id,
        "online": False,
        "reason": "connection_lost",
    }


def _sample_status(sample: object) -> str:
    status = getattr(sample, "status", None)
    if isinstance(status, SampleStatus):
        return status.value
    return "unavailable"
