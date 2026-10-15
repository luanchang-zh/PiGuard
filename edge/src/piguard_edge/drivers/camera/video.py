"""Read a local recording and use the same lane function as the synthetic camera."""

from pathlib import Path

import cv2

from piguard_edge.drivers.base import Clock, stamped
from piguard_edge.drivers.camera.synthetic import _lane_from_image
from piguard_edge.models.frame import Frame, invalid_lane
from piguard_edge.models.sample import SOURCE_MOCK, SampleStatus
from piguard_edge.processing.lane import IMAGE_HEIGHT, IMAGE_WIDTH


class VideoCamera:
    def __init__(self, path: str | Path) -> None:
        self._path = Path(path)
        if not self._path.is_file():
            raise FileNotFoundError(self._path)
        capture = cv2.VideoCapture(str(self._path))
        if not capture.isOpened():
            capture.release()
            raise RuntimeError(f"cannot open video: {self._path}")
        self._capture: cv2.VideoCapture | None = capture

    @property
    def released(self) -> bool:
        return self._capture is None

    def read(self, clock: Clock) -> Frame:
        sample_at, monotonic_ns = stamped(clock)
        if self._capture is None:
            return _closed(sample_at, monotonic_ns)
        ok, frame = self._capture.read()
        if not ok:
            self.release()
            return _closed(sample_at, monotonic_ns)
        image = _fit(frame)
        return Frame(sample_at, monotonic_ns, SOURCE_MOCK, SampleStatus.OK, image, _lane_from_image(image, sample_at))

    def release(self) -> None:
        if self._capture is None:
            return
        self._capture.release()
        self._capture = None


def _fit(frame: object) -> object:
    if getattr(frame, "shape", None) == (IMAGE_HEIGHT, IMAGE_WIDTH, 3):
        return frame
    return cv2.resize(frame, (IMAGE_WIDTH, IMAGE_HEIGHT), interpolation=cv2.INTER_LINEAR)


def _closed(sample_at: object, monotonic_ns: int) -> Frame:
    return Frame(sample_at, monotonic_ns, SOURCE_MOCK, SampleStatus.UNAVAILABLE, None, invalid_lane(sample_at))
