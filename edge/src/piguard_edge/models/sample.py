"""Sample values shared by drivers, processing, and later telemetry."""

from dataclasses import dataclass
from datetime import datetime, timezone
from enum import StrEnum
from typing import Generic, TypeVar

T = TypeVar("T")

UNIT_M = "m"
UNIT_KMH = "km/h"
UNIT_C = "C"
UNIT_DPS = "deg/s"
SOURCE_MOCK = "mock"


class SampleStatus(StrEnum):
    OK = "ok"
    TIMEOUT = "timeout"
    UNAVAILABLE = "unavailable"
    ERROR = "error"


def format_utc(value: datetime) -> str:
    utc = value.astimezone(timezone.utc)
    return utc.strftime("%Y-%m-%dT%H:%M:%S.") + f"{utc.microsecond // 1000:03d}Z"


@dataclass(frozen=True, slots=True)
class DistanceReading:
    raw_value: float
    mapped_value: float
    scale: float


@dataclass(frozen=True, slots=True)
class GyroAxes:
    x: float
    y: float
    z: float


@dataclass(frozen=True, slots=True)
class GyroReading:
    x: float
    y: float
    z: float
    yaw_rate: float


@dataclass(frozen=True, slots=True)
class Sample(Generic[T]):
    value: T | None
    sample_at: datetime
    monotonic_ns: int
    source: str
    status: SampleStatus
    unit: str

    def __post_init__(self) -> None:
        if self.sample_at.utcoffset() is None:
            raise ValueError("sample_at must be timezone-aware UTC")
        if not isinstance(self.status, SampleStatus):
            raise ValueError("status must be a SampleStatus")

    def to_fixture(self) -> dict[str, object]:
        value = self.value
        if isinstance(value, DistanceReading):
            encoded: object = {
                "raw_value": value.raw_value,
                "mapped_value": value.mapped_value,
                "scale": value.scale,
            }
        elif isinstance(value, GyroReading):
            encoded = {
                "x": value.x,
                "y": value.y,
                "z": value.z,
                "yaw_rate": value.yaw_rate,
            }
        elif isinstance(value, GyroAxes):
            encoded = {"x": value.x, "y": value.y, "z": value.z}
        else:
            encoded = value
        return {
            "value": encoded,
            "sample_at": format_utc(self.sample_at),
            "monotonic_ns": self.monotonic_ns,
            "source": self.source,
            "status": self.status.value,
            "unit": self.unit,
        }


def clear_fault_value(sample: Sample[T]) -> Sample[T]:
    """A failed read stores an empty value. A numeric zero is only valid when status is ok."""
    if sample.status is SampleStatus.OK or sample.value is None:
        return sample
    return Sample(
        None,
        sample.sample_at,
        sample.monotonic_ns,
        sample.source,
        sample.status,
        sample.unit,
    )
