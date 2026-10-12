"""Paint a road from the scenario, then read the lane from those pixels."""

from piguard_edge.drivers.base import Clock, SceneSource, stamped
from piguard_edge.drivers.camera.paint import paint_road
from piguard_edge.models.frame import Frame, LaneResult, invalid_lane
from piguard_edge.models.sample import SOURCE_MOCK, SampleStatus
from piguard_edge.processing import lane
from piguard_edge.processing.lane import IMAGE_HEIGHT, IMAGE_WIDTH


class SyntheticCamera:
    def __init__(self, scene_source: SceneSource, *, width: int = IMAGE_WIDTH, height: int = IMAGE_HEIGHT) -> None:
        self._scene_source = scene_source
        self._width = width
        self._height = height

    def read(self, clock: Clock) -> Frame:
        sample_at, monotonic_ns = stamped(clock)
        scene = self._scene_source.observe(monotonic_ns)
        if scene.camera_status is not SampleStatus.OK or not scene.lane_valid or scene.lane_offset is None:
            return Frame(sample_at, monotonic_ns, SOURCE_MOCK, scene.camera_status, None, invalid_lane(sample_at))
        image = paint_road(scene.lane_offset, width=self._width, height=self._height)
        return Frame(
            sample_at,
            monotonic_ns,
            SOURCE_MOCK,
            SampleStatus.OK,
            image,
            _lane_from_image(image, sample_at),
        )


def _lane_from_image(image: object, sample_at: object) -> LaneResult:
    found = lane.inspect_lane(image)
    if not found.valid or found.offset_ratio is None or found.confidence is None:
        return invalid_lane(sample_at)
    return LaneResult(True, found.offset_ratio, found.direction, found.confidence, sample_at)
