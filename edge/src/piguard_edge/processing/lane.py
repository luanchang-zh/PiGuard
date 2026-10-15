"""Lane offset from one BGR frame. The same function serves synthetic, video, and later CSI frames."""

from dataclasses import dataclass

import cv2
import numpy as np

from piguard_edge.models.frame import lane_direction

IMAGE_WIDTH = 640
IMAGE_HEIGHT = 480
ROI_TOP = 240
_MEASURE_Y = 220
_MIN_SLOPE = 0.3
_MIN_LANE_WIDTH = 40


@dataclass(frozen=True, slots=True)
class LaneInspection:
    roi: np.ndarray
    gray: np.ndarray
    blurred: np.ndarray
    edges: np.ndarray
    lines: np.ndarray | None
    valid: bool
    offset_ratio: float | None
    direction: str
    confidence: float | None


def inspect_lane(image: np.ndarray) -> LaneInspection:
    if image.ndim != 3 or image.shape[2] != 3:
        raise ValueError("lane input must be a BGR image")
    frame = image
    if frame.shape[1] != IMAGE_WIDTH or frame.shape[0] != IMAGE_HEIGHT:
        frame = cv2.resize(frame, (IMAGE_WIDTH, IMAGE_HEIGHT), interpolation=cv2.INTER_LINEAR)
    roi = frame[ROI_TOP:IMAGE_HEIGHT, :].copy()
    gray = cv2.cvtColor(roi, cv2.COLOR_BGR2GRAY)
    blurred = cv2.GaussianBlur(gray, (5, 5), 0)
    edges = cv2.Canny(blurred, 50, 150)
    found = cv2.HoughLinesP(edges, 1, np.pi / 180, 40, minLineLength=40, maxLineGap=30)
    left, right = _sides(found)
    valid, offset, confidence = _offset(left, right)
    direction = lane_direction(offset, valid) if valid and offset is not None else "unknown"
    return LaneInspection(roi, gray, blurred, edges, found, valid, offset, direction, confidence)


def _sides(lines: np.ndarray | None) -> tuple[list[float], list[float]]:
    left: list[float] = []
    right: list[float] = []
    if lines is None:
        return left, right
    for item in lines[:, 0, :]:
        x1, y1, x2, y2 = (int(value) for value in item)
        dx = x2 - x1
        dy = y2 - y1
        if dy == 0:
            continue
        if dx == 0:
            x_at = float(x1)
            side = left if x_at < IMAGE_WIDTH / 2 else right
            side.append(x_at)
            continue
        slope = dy / dx
        if abs(slope) < _MIN_SLOPE:
            continue
        x_at = x1 + (_MEASURE_Y - y1) * dx / dy
        if x_at < 0 or x_at >= IMAGE_WIDTH:
            continue
        side = left if slope < 0 else right
        side.append(float(x_at))
    return left, right


def _offset(left: list[float], right: list[float]) -> tuple[bool, float | None, float | None]:
    if not left or not right:
        return False, None, None
    left_x = sum(left) / len(left)
    right_x = sum(right) / len(right)
    lane_width = right_x - left_x
    if lane_width < _MIN_LANE_WIDTH:
        return False, None, None
    lane_center = (left_x + right_x) / 2
    offset = (IMAGE_WIDTH / 2 - lane_center) / (lane_width / 2)
    if abs(offset) < 0.01:
        offset = 0.0
    confidence = min(1.0, (len(left) + len(right)) / 4)
    return True, float(offset), float(confidence)
