"""Logical speed from the shared scene. It is not derived from the gyro."""

from piguard_edge.drivers.base import Clock, SceneSource, stamped
from piguard_edge.models.sample import SOURCE_MOCK, UNIT_KMH, Sample, SampleStatus


class ScenarioSpeedProvider:
    def __init__(self, scene_source: SceneSource) -> None:
        self._scene_source = scene_source

    def read(self, clock: Clock) -> Sample[float]:
        sample_at, monotonic_ns = stamped(clock)
        scene = self._scene_source.observe(monotonic_ns)
        if scene.speed_status is not SampleStatus.OK:
            return Sample(None, sample_at, monotonic_ns, SOURCE_MOCK, scene.speed_status, UNIT_KMH)
        if scene.speed_kmh is None:
            raise ValueError("ok speed scene is missing a value")
        return Sample(scene.speed_kmh, sample_at, monotonic_ns, SOURCE_MOCK, SampleStatus.OK, UNIT_KMH)
