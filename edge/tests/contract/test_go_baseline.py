"""Check encoded payloads with the Go parser from the backend branch.

The edge branch does not contain go-backend. The test reads that parser from
git and does not copy it into this tree.
"""

import json
import shutil
import subprocess
from pathlib import Path

import pytest

from piguard_edge.models.sample import UNIT_M, Sample, SampleStatus
from piguard_edge.protocol.event import make_event
from piguard_edge.protocol.status import build_online_status, build_will
from piguard_edge.protocol.telemetry import build_telemetry
from piguard_edge.protocol.topics import events_topic, status_topic, telemetry_topic
from piguard_edge.runtime.clock import FakeClock
from piguard_edge.runtime.state import LatestState

REPO = Path(__file__).resolve().parents[3]
HARNESS = Path(__file__).resolve().parent / "go_harness" / "parser_test.go"
PARSERS = (
    "go后端开发:go-backend/internal/protocol/telemetry.go",
    "go后端开发:go-backend/internal/protocol/event.go",
    "go后端开发:go-backend/internal/protocol/command.go",
)
FRAME_IDENTITY = '''package protocol

import "regexp"

var frameIdentity = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
'''


def test_go_parser_accepts_encoder_output_and_rejects_bad_payloads(example_config) -> None:
    go = shutil.which("go")
    if go is None:
        pytest.skip("go is not installed")
    sources = {
        Path(ref).name: subprocess.run(
            ["git", "show", ref],
            cwd=REPO,
            check=True,
            capture_output=True,
        ).stdout
        for ref in PARSERS
    }
    clock = FakeClock()
    normal = build_telemetry(
        _ready_state(clock),
        device_id="car-001",
        timestamp=clock.utc(),
        seq=0,
        message_id="msg-normal",
        distance_scale=example_config.mapping.distance_scale,
        source="mock",
    )
    fault = build_telemetry(
        _fault_state(clock),
        device_id="car-001",
        timestamp=clock.utc(),
        seq=1,
        message_id="msg-fault",
        distance_scale=example_config.mapping.distance_scale,
        source="mock",
    )
    online = build_online_status(
        _ready_state(clock),
        device_id="car-001",
        timestamp=clock.utc(),
        software_version="0.0.0",
        config_version=0,
        mode="mock",
    )
    will = build_will(device_id="car-001")
    missing_seq = dict(normal)
    missing_seq.pop("seq")
    stale = json.loads(json.dumps(online))
    stale["sensors"]["distance"] = "stale"
    incomplete_online = dict(online)
    incomplete_online.pop("timestamp")
    fixtures = {
        "accept_telemetry": [
            {"topic": telemetry_topic("car-001"), "body": normal},
            {"topic": telemetry_topic("car-001"), "body": fault},
        ],
        "reject_telemetry": [
            {"topic": telemetry_topic("car-001"), "body": missing_seq},
            {"topic": "car/car-002/telemetry", "body": normal},
            {"topic": telemetry_topic("car-001"), "body": {**normal, "schema_version": 2}},
        ],
        "accept_status": [
            {"topic": status_topic("car-001"), "body": online},
            {"topic": status_topic("car-001"), "body": will},
        ],
        "reject_status": [
            {"topic": status_topic("car-001"), "body": stale},
            {"topic": status_topic("car-001"), "body": incomplete_online},
        ],
        "accept_event": [
            {"topic": events_topic("car-001"), "body": _event(clock, "evt-obstacle", "obstacle_warning", "started", "danger", {"distance_m": 6.8, "speed_kmh": 32.0, "headway_s": 0.76})},
            {"topic": events_topic("car-001"), "body": _event(clock, "evt-temp", "high_temperature", "recovered", "warning", {"temperature_c": 32.0})},
            {"topic": events_topic("car-001"), "body": _event(clock, "evt-sensor", "sensor_failure", "started", "warning", {"sensor": "distance", "reason": "stale"})},
        ],
        "reject_event": [
            {"topic": events_topic("car-001"), "body": {**_event(clock, "evt-bad-data", "lane_departure", "started", "warning", {"offset_ratio": 0.4}), "data": []}},
            {"topic": events_topic("car-001"), "body": {**_event(clock, "evt-bad-level", "lane_departure", "recovered", "warning", {"offset_ratio": 0.0}), "level": "normal"}},
            {"topic": events_topic("car-001"), "body": {**_event(clock, "evt-extra", "sharp_turn", "started", "warning", {"yaw_rate_deg_s": 35.0}), "incident_id": "nope"}},
            {"topic": "car/car-002/events", "body": _event(clock, "evt-topic", "sensor_failure", "started", "warning", {"sensor": "gyro", "reason": "error"})},
        ],
    }
    from tempfile import TemporaryDirectory

    with TemporaryDirectory() as directory:
        root = Path(directory)
        package = root / "protocol"
        package.mkdir()
        for name, source in sources.items():
            (package / name).write_bytes(source)
        (package / "frame_identity.go").write_text(FRAME_IDENTITY, encoding="utf-8")
        (package / "parser_test.go").write_text(HARNESS.read_text(encoding="utf-8"), encoding="utf-8")
        payloads = root / "payloads.json"
        payloads.write_text(json.dumps(fixtures), encoding="utf-8")
        (root / "go.mod").write_text("module piguard.edge.contract\n\ngo 1.22\n", encoding="utf-8")
        completed = subprocess.run(
            [go, "test", "./protocol"],
            cwd=root,
            capture_output=True,
            text=True,
            env={**dict(**_env()), "PAYLOADS": str(payloads)},
        )
    assert completed.returncode == 0, completed.stdout + completed.stderr


def _event(clock: FakeClock, event_id: str, event_type: str, action: str, level: str, data: dict[str, object]) -> dict[str, object]:
    event = make_event(
        device_id="car-001",
        event_id=event_id,
        event_type=event_type,
        action=action,
        level=level,
        timestamp=clock.utc(),
        data=data,
    )
    return json.loads(event.payload.decode("utf-8"))


def _ready_state(clock: FakeClock) -> LatestState:
    from piguard_edge.models.frame import Frame, LaneResult
    from piguard_edge.models.sample import UNIT_C, UNIT_DPS, UNIT_KMH, DistanceReading, GyroReading

    now = clock.utc()
    state = LatestState()
    state.speed = Sample(30.0, now, 0, "mock", SampleStatus.OK, UNIT_KMH)
    state.distance = Sample(DistanceReading(0.5, 50.0, 100.0), now, 0, "mock", SampleStatus.OK, UNIT_M)
    state.temperature = Sample(25.0, now, 0, "mock", SampleStatus.OK, UNIT_C)
    state.gyro = Sample(GyroReading(0.0, 0.0, 2.0, 2.0), now, 0, "mock", SampleStatus.OK, UNIT_DPS)
    state.frame = Frame(now, 0, "mock", SampleStatus.OK, None, LaneResult(True, 0.0, "center", 1.0, now))
    return state


def _fault_state(clock: FakeClock) -> LatestState:
    now = clock.utc()
    state = LatestState()
    state.distance = Sample(None, now, 0, "mock", SampleStatus.TIMEOUT, UNIT_M)
    return state


def _env() -> dict[str, str]:
    import os

    return dict(os.environ)
