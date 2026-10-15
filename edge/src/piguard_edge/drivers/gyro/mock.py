"""Mock gyro. The scene's yaw rate is placed on z; axis selection happens later."""

from piguard_edge.drivers.base import Clock, SceneSource, stamped
from piguard_edge.models.sample import SOURCE_MOCK, UNIT_DPS, GyroAxes, Sample, SampleStatus


class MockGyroscopeSensor:
    def __init__(self, scene_source: SceneSource) -> None:
        self._scene_source = scene_source

    def read(self, clock: Clock) -> Sample[GyroAxes]:
        sample_at, monotonic_ns = stamped(clock)
        scene = self._scene_source.observe(monotonic_ns)
        if scene.gyro_status is not SampleStatus.OK:
            return Sample(None, sample_at, monotonic_ns, SOURCE_MOCK, scene.gyro_status, UNIT_DPS)
        if scene.gyro_x is None or scene.gyro_y is None or scene.gyro_z is None:
            raise ValueError("ok gyro scene is missing an axis")
        axes = GyroAxes(scene.gyro_x, scene.gyro_y, scene.gyro_z)
        return Sample(axes, sample_at, monotonic_ns, SOURCE_MOCK, SampleStatus.OK, UNIT_DPS)
