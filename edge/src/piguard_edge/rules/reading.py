"""Classify a sample before a rule uses its number."""

from piguard_edge.models.frame import Frame
from piguard_edge.models.sample import Sample, SampleStatus

_NS_PER_MS = 1_000_000


def as_float(value: object) -> float | None:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return None
    return float(value)


def classify(sample: Sample[object] | None, now_ns: int, ttl_ms: int) -> tuple[str, str]:
    if sample is None:
        return "missing", ""
    if sample.status is not SampleStatus.OK or sample.value is None:
        return "fault", sample.status.value
    if now_ns - sample.monotonic_ns > ttl_ms * _NS_PER_MS:
        return "fault", "stale"
    return "ok", ""


def classify_frame(frame: Frame | None, now_ns: int, ttl_ms: int) -> tuple[str, str]:
    if frame is None:
        return "missing", ""
    if frame.status is not SampleStatus.OK:
        return "fault", frame.status.value
    if now_ns - frame.monotonic_ns > ttl_ms * _NS_PER_MS:
        return "fault", "stale"
    return "ok", ""
