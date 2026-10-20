"""Fetch PC JPEGs in the background so USB failures cannot stall sampling."""

import logging
import math
import threading
import time
from datetime import timedelta
from http.client import HTTPException
from urllib.error import HTTPError, URLError
from urllib.request import HTTPRedirectHandler, ProxyHandler, Request, build_opener

import cv2
import numpy as np

from piguard_edge.config.model import CameraConfig
from piguard_edge.drivers.base import Clock, stamped
from piguard_edge.drivers.camera.bridge import AGE_HEADER, MAX_JPEG_BYTES, camera_token
from piguard_edge.drivers.camera.synthetic import LaneProcessor
from piguard_edge.models.frame import Frame, invalid_lane
from piguard_edge.models.sample import SampleStatus

_LOG = logging.getLogger(__name__)


class _NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class RemoteCamera:
    def __init__(self, config: CameraConfig) -> None:
        self._config = config
        self._token = camera_token(config.remote_token_env)
        # USB camera traffic must not go through a desktop HTTP proxy.
        self._opener = build_opener(ProxyHandler({}), _NoRedirect())
        self._detector = LaneProcessor(config.process_fps)
        self._lock = threading.Lock()
        self._latest: tuple[object, int] | None = None
        self._status = SampleStatus.UNAVAILABLE
        self._stop = threading.Event()
        self._thread = threading.Thread(target=self._run, name="pc-camera-client", daemon=True)
        self._thread.start()

    def read(self, clock: Clock) -> Frame:
        sample_at, monotonic_ns = stamped(clock)
        with self._lock:
            latest, status = self._latest, self._status
        if latest is not None and not self._stop.is_set():
            image, captured_ns = latest
            age_ns = max(0, time.monotonic_ns() - captured_ns)
            if age_ns <= self._config.remote_max_age_ms * 1_000_000:
                at = sample_at - timedelta(microseconds=age_ns // 1000)
                return Frame(at, monotonic_ns - age_ns, "hardware", SampleStatus.OK, image,
                             self._detector.read(image, at, monotonic_ns))
            status = SampleStatus.TIMEOUT
        self._detector.reset()
        return Frame(sample_at, monotonic_ns, "hardware", status, None, invalid_lane(sample_at))

    def release(self) -> None:
        self._stop.set()
        self._thread.join(timeout=self._config.remote_timeout_s + 0.5)
        with self._lock:
            self._latest = None
            self._status = SampleStatus.UNAVAILABLE

    def _fetch(self) -> tuple[object, int]:
        started_ns = time.monotonic_ns()
        request = Request(self._config.remote_url, headers={
            "Authorization": f"Bearer {self._token}", "Accept": "image/jpeg",
        })
        with self._opener.open(request, timeout=self._config.remote_timeout_s) as response:
            if response.headers.get_content_type() != "image/jpeg":
                raise ValueError("expected image/jpeg")
            age_ms = float(response.headers.get(AGE_HEADER, "nan"))
            if not math.isfinite(age_ms) or age_ms < 0:
                raise ValueError("missing or invalid frame age")
            if age_ms > self._config.remote_max_age_ms:
                raise TimeoutError("remote frame is stale")
            content_length = int(response.headers.get("Content-Length", ""))
            if not 0 < content_length <= MAX_JPEG_BYTES:
                raise ValueError("invalid JPEG content length")
            jpeg = response.read(MAX_JPEG_BYTES + 1)
        if len(jpeg) != content_length or not jpeg.startswith(b"\xff\xd8"):
            raise ValueError("invalid JPEG size or signature")
        # Include the entire round trip as a conservative age estimate; PC and
        # Pi wall clocks need not agree for stale-frame rejection to work.
        captured_ns = started_ns - int(age_ms * 1_000_000)
        if time.monotonic_ns() - captured_ns > self._config.remote_max_age_ms * 1_000_000:
            raise TimeoutError("remote frame is stale")
        image = cv2.imdecode(np.frombuffer(jpeg, np.uint8), cv2.IMREAD_COLOR)
        if image is None:
            raise ValueError("invalid JPEG pixels")
        image = cv2.resize(image, (self._config.width, self._config.height))
        return image, captured_ns

    def _run(self) -> None:
        while not self._stop.is_set():
            started = time.monotonic()
            latest = None
            reason = ""
            try:
                latest = self._fetch()
                status = SampleStatus.OK
            except HTTPError as exc:
                status = SampleStatus.UNAVAILABLE if exc.code == 503 else SampleStatus.ERROR
                reason = f"HTTP {exc.code}"
            except TimeoutError:
                status, reason = SampleStatus.TIMEOUT, "request timed out or frame expired"
            except (OSError, URLError, HTTPException):
                status, reason = SampleStatus.UNAVAILABLE, "connection unavailable"
            except (ValueError, cv2.error):
                status, reason = SampleStatus.ERROR, "invalid camera response"
            with self._lock:
                previous = self._status
                if not self._stop.is_set():
                    self._latest, self._status = latest, status
            if status != previous and status is not SampleStatus.OK:
                _LOG.warning("PC camera: %s", reason)
            delay = 1 / self._config.capture_fps if latest is not None else 1.0
            self._stop.wait(max(0, delay - (time.monotonic() - started)))
