"""Camera frame and the lane result carried with it."""

from dataclasses import dataclass
from datetime import datetime

from piguard_edge.models.sample import SampleStatus, format_utc


def lane_direction(offset: float, valid: bool) -> str:
    if not valid:
        return "unknown"
    if offset > 0:
        return "right"
    if offset < 0:
        return "left"
    return "center"


@dataclass(frozen=True, slots=True)
class LaneResult:
    valid: bool
    offset_ratio: float | None
    direction: str
    confidence: float | None
    sample_at: datetime

    def __post_init__(self) -> None:
        if self.sample_at.utcoffset() is None:
            raise ValueError("sample_at must be timezone-aware UTC")
        if self.valid:
            if self.offset_ratio is None or self.confidence is None:
                raise ValueError("a valid lane needs an offset and confidence")
            if self.direction != lane_direction(self.offset_ratio, True):
                raise ValueError("lane direction does not match the offset sign")
            return
        if self.offset_ratio is not None or self.confidence is not None or self.direction != "unknown":
            raise ValueError("an invalid lane keeps offset, confidence, and direction empty")

    def to_fixture(self) -> dict[str, object]:
        return {
            "valid": self.valid,
            "offset_ratio": self.offset_ratio,
            "direction": self.direction,
            "confidence": self.confidence,
            "sample_at": format_utc(self.sample_at),
        }


def invalid_lane(sample_at: datetime) -> LaneResult:
    return LaneResult(False, None, "unknown", None, sample_at)


@dataclass(frozen=True, slots=True)
class Frame:
    sample_at: datetime
    monotonic_ns: int
    source: str
    status: SampleStatus
    image: object | None
    lane: LaneResult

    def __post_init__(self) -> None:
        if self.sample_at.utcoffset() is None:
            raise ValueError("sample_at must be timezone-aware UTC")
        if not isinstance(self.status, SampleStatus):
            raise ValueError("status must be a SampleStatus")
