from dataclasses import replace

from piguard_edge.drivers.distance.mock import MockDistanceSensor
from piguard_edge.drivers.speed.scenario import ScenarioSpeedProvider
from piguard_edge.processing.distance import map_distance
from piguard_edge.processing.gyro import apply_yaw
from piguard_edge.runtime.clock import FakeClock
from piguard_edge.simulation.engine import ScenarioEngine
from piguard_edge.simulation.scenarios import normal_scene


def test_road_distance_is_divided_once_and_mapped_once() -> None:
    engine = ScenarioEngine("normal_drive", 1.0)
    sensor = MockDistanceSensor(engine, distance_scale=100)
    raw = sensor.read(FakeClock())

    assert raw.value == 0.5
    assert map_distance(raw.value, 100) == 50


def test_zero_speed_is_not_passed_through_distance_mapping() -> None:
    scene = replace(normal_scene(), speed_kmh=0.0)
    provider = ScenarioSpeedProvider(_Fixed(scene))
    sample = provider.read(FakeClock())

    assert sample.value == 0.0
    assert map_distance(0.5, 100) == 50


def test_yaw_uses_configured_axis_and_sign() -> None:
    assert apply_yaw(0.0, 0.0, 2.0, "z", 1) == 2.0
    assert apply_yaw(0.0, 0.0, 2.0, "z", -1) == -2.0
    assert apply_yaw(4.0, 0.0, 2.0, "x", 1) == 4.0


class _Fixed:
    def __init__(self, scene) -> None:
        self._scene = scene

    def observe(self, monotonic_ns: int):
        return self._scene
