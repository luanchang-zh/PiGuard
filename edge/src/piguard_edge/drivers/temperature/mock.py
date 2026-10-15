"""Mock cabin temperature."""

from piguard_edge.drivers.base import Clock, SceneSource, stamped
from piguard_edge.models.sample import SOURCE_MOCK, UNIT_C, Sample, SampleStatus


class MockTemperatureSensor:
    def __init__(self, scene_source: SceneSource) -> None:
        self._scene_source = scene_source

    def read(self, clock: Clock) -> Sample[float]:
        sample_at, monotonic_ns = stamped(clock)
        scene = self._scene_source.observe(monotonic_ns)
        if scene.temperature_status is not SampleStatus.OK:
            return Sample(None, sample_at, monotonic_ns, SOURCE_MOCK, scene.temperature_status, UNIT_C)
        if scene.temperature_c is None:
            raise ValueError("ok temperature scene is missing a value")
        return Sample(scene.temperature_c, sample_at, monotonic_ns, SOURCE_MOCK, SampleStatus.OK, UNIT_C)
