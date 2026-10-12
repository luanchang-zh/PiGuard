import pytest

from piguard_edge.drivers.factory import build_sources
from piguard_edge.models.sample import SampleStatus
from piguard_edge.runtime.clock import FakeClock
from piguard_edge.simulation.engine import ScenarioEngine
from piguard_edge.simulation.scenarios import SCENARIO_NAMES

_DRIVERS = {
    "distance_driver": "mock",
    "camera_driver": "synthetic",
    "gyro_driver": "mock",
    "temperature_driver": "mock",
    "speed_driver": "scenario",
}


def test_scenario_names_match_the_command_contract() -> None:
    assert SCENARIO_NAMES == (
        "normal_drive",
        "obstacle_approach",
        "lane_departure",
        "sharp_turn",
        "high_temperature",
        "sensor_failure",
    )


def test_same_time_is_the_same_keyframe_after_rewind() -> None:
    engine = ScenarioEngine("obstacle_approach", 1.0)

    early = engine.observe(500_000_000)
    late = engine.observe(1_500_000_000)
    again = engine.observe(500_000_000)

    assert early == again
    assert early.road_distance_m == 50
    assert late.road_distance_m == 40


def test_playback_speed_only_scales_keyframe_time() -> None:
    assert ScenarioEngine("obstacle_approach", 2.0).observe(500_000_000).road_distance_m == 40
    assert ScenarioEngine("obstacle_approach", 0.5).observe(1_000_000_000).road_distance_m == 50
    assert ScenarioEngine("obstacle_approach", 0.5).observe(2_000_000_000).road_distance_m == 40


def test_sequence_holds_the_last_keyframe() -> None:
    assert ScenarioEngine("obstacle_approach", 1.0).observe(100_000_000_000).road_distance_m == 5


def test_sources_share_one_scene_and_keep_image_empty() -> None:
    engine = ScenarioEngine("obstacle_approach", 1.0)
    sources = build_sources(engine, distance_scale=100, **_DRIVERS)
    early = FakeClock(500_000_000)
    late = FakeClock(2_500_000_000)

    late_distance = sources.distance.read(late)
    assert late_distance.value == pytest.approx(0.3)
    assert sources.speed.read(late).value == 30
    assert sources.gyro.read(late).value is not None
    assert sources.gyro.read(late).value.z == 2
    assert sources.temperature.read(late).value == 25
    assert sources.camera.read(late).lane.offset_ratio == 0
    assert sources.camera.read(late).image is None
    assert sources.distance.read(early).value == 0.5


def test_stop_restores_normal_drive_on_the_next_read() -> None:
    engine = ScenarioEngine("obstacle_approach", 1.0)
    sources = build_sources(engine, distance_scale=100, **_DRIVERS)
    late = FakeClock(3_000_000_000)
    assert sources.distance.read(late).value == pytest.approx(0.2)

    engine.stop()

    assert sources.distance.read(late).value == 0.5
    assert sources.speed.read(late).value == 30
    assert engine.name == "normal_drive"


def test_lane_departure_positive_offset_is_right() -> None:
    engine = ScenarioEngine("lane_departure", 1.0)
    sources = build_sources(engine, distance_scale=100, **_DRIVERS)
    frame = sources.camera.read(FakeClock(4_000_000_000))

    assert frame.lane.valid is True
    assert frame.lane.offset_ratio == pytest.approx(0.4)
    assert frame.lane.direction == "right"


def test_sensor_failure_empties_only_the_named_source() -> None:
    engine = ScenarioEngine("sensor_failure", 1.0)
    sources = build_sources(engine, distance_scale=100, **_DRIVERS)

    distance = sources.distance.read(FakeClock(500_000_000))
    assert distance.status is SampleStatus.TIMEOUT
    assert distance.value is None
    assert sources.speed.read(FakeClock(500_000_000)).value == 30

    camera = sources.camera.read(FakeClock(1_500_000_000))
    assert camera.status is SampleStatus.UNAVAILABLE
    assert camera.image is None
    assert camera.lane.valid is False
    assert camera.lane.offset_ratio is None
    assert sources.temperature.read(FakeClock(1_500_000_000)).value == 25

    gyro = sources.gyro.read(FakeClock(2_500_000_000))
    assert gyro.status is SampleStatus.ERROR
    assert gyro.value is None
    assert sources.distance.read(FakeClock(2_500_000_000)).value == 0.5


def test_rejected_playback_speed() -> None:
    with pytest.raises(ValueError):
        ScenarioEngine("normal_drive", 3.0)
