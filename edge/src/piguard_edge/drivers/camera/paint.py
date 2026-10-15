"""Draw a road whose lane position is only recoverable from the pixels."""

import cv2
import numpy as np

from piguard_edge.processing.lane import IMAGE_HEIGHT, IMAGE_WIDTH, ROI_TOP

_HALF = 140.0
_TOP_SCALE = 0.45


def paint_road(
    offset_ratio: float,
    *,
    left: bool = True,
    right: bool = True,
    width: int = IMAGE_WIDTH,
    height: int = IMAGE_HEIGHT,
) -> np.ndarray:
    image = np.full((height, width, 3), (32, 32, 32), dtype=np.uint8)
    bottom_y = height - 1
    top_y = ROI_TOP + 10
    center_bottom = width / 2 - offset_ratio * _HALF
    center_top = width / 2 - offset_ratio * _HALF * _TOP_SCALE
    top_half = _HALF * _TOP_SCALE
    if left:
        _line(image, center_bottom - _HALF, bottom_y, center_top - top_half, top_y)
    if right:
        _line(image, center_bottom + _HALF, bottom_y, center_top + top_half, top_y)
    return image


def _line(image: np.ndarray, x1: float, y1: int, x2: float, y2: int) -> None:
    cv2.line(image, (int(round(x1)), y1), (int(round(x2)), y2), (255, 255, 255), 6)
