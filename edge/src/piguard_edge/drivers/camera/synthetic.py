"""Scenario lane without pixels. OpenCV frames arrive in a later phase."""

from piguard_edge.drivers.base import Clock, SceneSource, stamped
from piguard_edge.models.frame import Frame, LaneResult, invalid_lane, lane_direction
from piguard_edge.models.sample import SOURCE_MOCK, SampleStatus
from piguard_edge.simulation.scenarios import SIMULATED_LANE_CONFIDENCE


class SyntheticCamera:
    def __init__(self, scene_source: SceneSource) -> None:
        self._scene_source = scene_source

    def read(self, clock: Clock) -> Frame:
        sample_at, monotonic_ns = stamped(clock)
        scene = self._scene_source.observe(monotonic_ns)
        if scene.camera_status is not SampleStatus.OK or not scene.lane_valid or scene.lane_offset is None:
            return Frame(
                sample_at,
                monotonic_ns,
                SOURCE_MOCK,
                scene.camera_status,
                None,
                invalid_lane(sample_at),
            )
        lane = LaneResult(
            True,
            scene.lane_offset,
            lane_direction(scene.lane_offset, True),
            SIMULATED_LANE_CONFIDENCE,
            sample_at,
        )
        return Frame(sample_at, monotonic_ns, SOURCE_MOCK, SampleStatus.OK, None, lane)
