"""Mock distance. Road metres from the scene become raw metres here, once."""

from piguard_edge.drivers.base import Clock, SceneSource, stamped
from piguard_edge.models.sample import SOURCE_MOCK, UNIT_M, Sample, SampleStatus


class MockDistanceSensor:
    def __init__(self, scene_source: SceneSource, distance_scale: float) -> None:
        if distance_scale <= 0:
            raise ValueError("distance_scale must be positive")
        self._scene_source = scene_source
        self._distance_scale = distance_scale

    def read(self, clock: Clock) -> Sample[float]:
        sample_at, monotonic_ns = stamped(clock)
        scene = self._scene_source.observe(monotonic_ns)
        if scene.distance_status is not SampleStatus.OK:
            return Sample(None, sample_at, monotonic_ns, SOURCE_MOCK, scene.distance_status, UNIT_M)
        if scene.road_distance_m is None:
            raise ValueError("ok distance scene is missing a road distance")
        raw_m = scene.road_distance_m / self._distance_scale
        return Sample(raw_m, sample_at, monotonic_ns, SOURCE_MOCK, SampleStatus.OK, UNIT_M)
