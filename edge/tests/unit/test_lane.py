import numpy as np
import pytest

from piguard_edge.drivers.camera.paint import paint_road
from piguard_edge.drivers.camera.synthetic import SyntheticCamera
from piguard_edge.drivers.camera.video import VideoCamera
from piguard_edge.processing.lane import inspect_lane
from piguard_edge.runtime.clock import FakeClock
from piguard_edge.simulation.engine import ScenarioEngine


def test_centered_and_shifted_roads_keep_their_sign() -> None:
    center = inspect_lane(paint_road(0))
    right = inspect_lane(paint_road(0.4))
    left = inspect_lane(paint_road(-0.4))

    assert center.valid is True
    assert center.offset_ratio == pytest.approx(0)
    assert center.direction == "center"
    assert right.offset_ratio == pytest.approx(0.4, abs=0.02)
    assert right.direction == "right"
    assert left.offset_ratio == pytest.approx(-0.4, abs=0.02)
    assert left.direction == "left"
    assert 0 < center.confidence <= 1


def test_pipeline_uses_roi_gray_blur_canny_and_hough() -> None:
    found = inspect_lane(paint_road(0.2))

    assert found.roi.shape == (240, 640, 3)
    assert found.gray.shape == (240, 640)
    assert found.blurred.shape == found.gray.shape
    assert not np.array_equal(found.blurred, found.gray)
    assert found.edges.shape == found.gray.shape
    assert found.edges.max() == 255
    assert np.count_nonzero(found.edges) < found.gray.size
    assert found.lines is not None
    assert len(found.lines) >= 2


def test_one_missing_line_or_a_blank_frame_is_invalid() -> None:
    for image in (paint_road(0.2, right=False), paint_road(0.2, left=False), np.zeros((480, 640, 3), np.uint8)):
        found = inspect_lane(image)
        assert found.valid is False
        assert found.offset_ratio is None
        assert found.direction == "unknown"
        assert found.confidence is None


def test_synthetic_camera_uses_the_detector_instead_of_the_scenario_number() -> None:
    engine = ScenarioEngine("normal_drive", 1.0)
    camera = SyntheticCamera(engine)

    def shifted(image):
        found = inspect_lane(image)
        return found.__class__(
            found.roi,
            found.gray,
            found.blurred,
            found.edges,
            found.lines,
            True,
            -0.2,
            "left",
            0.5,
        )

    camera_module = __import__("piguard_edge.drivers.camera.synthetic", fromlist=["lane"])
    original = camera_module.lane.inspect_lane
    camera_module.lane.inspect_lane = shifted
    try:
        frame = camera.read(FakeClock())
    finally:
        camera_module.lane.inspect_lane = original

    assert frame.lane.offset_ratio == -0.2
    assert frame.lane.direction == "left"
    assert frame.image is not None


def test_video_ends_and_releases_the_capture(tmp_path) -> None:
    import cv2

    path = tmp_path / "road.avi"
    writer = cv2.VideoWriter(str(path), cv2.VideoWriter_fourcc(*"MJPG"), 10, (640, 480))
    assert writer.isOpened()
    writer.write(paint_road(0))
    writer.write(paint_road(0.4))
    writer.release()

    camera = VideoCamera(path)
    first = camera.read(FakeClock())
    second = camera.read(FakeClock())
    ended = camera.read(FakeClock())
    camera.release()

    assert first.lane.direction == "center"
    assert second.lane.offset_ratio == pytest.approx(0.4, abs=0.02)
    assert second.lane.direction == "right"
    assert ended.status.value == "unavailable"
    assert ended.image is None
    assert ended.lane.valid is False
    assert camera.released is True


def test_capture_rate_and_processing_rate_are_independent(example_config, monkeypatch):
    from piguard_edge.runtime.workers import Sampler
    import piguard_edge.drivers.camera.synthetic as camera_module
    calls = []
    original = camera_module.lane.inspect_lane
    def count(image):
        calls.append(1)
        return original(image)
    monkeypatch.setattr(camera_module.lane, 'inspect_lane', count)
    clock = FakeClock()
    sampler = Sampler.from_config(example_config, ScenarioEngine('normal_drive', 1), clock)
    timestamps = []
    for _ in range(10):
        clock.advance_ns(100_000_000)
        sampler.step()
        timestamps.append(sampler.state.frame.lane.sample_at)
    assert len(calls) == 5
    assert timestamps[0] == timestamps[1]
    assert timestamps[1] < timestamps[2]
    sampler.close()
