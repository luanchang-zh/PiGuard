import json
from datetime import datetime, timezone

from piguard_edge.models.frame import LaneResult, invalid_lane
from piguard_edge.models.sample import UNIT_M, Sample, SampleStatus, clear_fault_value

NOW = datetime(2026, 1, 1, tzinfo=timezone.utc)


def test_fault_value_stays_null_in_fixture() -> None:
    failed = Sample(0.0, NOW, 10, "mock", SampleStatus.TIMEOUT, UNIT_M)

    cleaned = clear_fault_value(failed)
    payload = json.loads(json.dumps(cleaned.to_fixture()))

    assert cleaned.value is None
    assert payload["value"] is None
    assert payload["status"] == "timeout"


def test_ok_zero_speed_is_preserved() -> None:
    stopped = Sample(0.0, NOW, 10, "mock", SampleStatus.OK, "km/h")

    assert clear_fault_value(stopped).value == 0.0


def test_invalid_lane_fixture_keeps_offset_empty() -> None:
    payload = json.loads(json.dumps(invalid_lane(NOW).to_fixture()))

    assert payload["valid"] is False
    assert payload["offset_ratio"] is None
    assert payload["direction"] == "unknown"
    assert payload["confidence"] is None


def test_negative_offset_is_left() -> None:
    lane = LaneResult(True, -0.4, "left", 1.0, NOW)
    payload = json.loads(json.dumps(lane.to_fixture()))

    assert payload["offset_ratio"] == -0.4
    assert payload["direction"] == "left"
