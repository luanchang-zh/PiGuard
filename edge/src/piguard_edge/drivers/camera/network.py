"""Live HTTP JPEG frames, polled in the background with original timestamps."""

import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone

import cv2
import numpy as np

from piguard_edge.drivers.base import stamped
from piguard_edge.drivers.camera.synthetic import LaneProcessor
from piguard_edge.drivers.camera.video import _fit
from piguard_edge.models.frame import Frame, invalid_lane
from piguard_edge.models.sample import SampleStatus


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class NetworkCamera:
    source = "hardware"

    def __init__(self, url, token="", *, capture_fps=10, process_fps=5, timeout_s=.7, start=True):
        parsed = urllib.parse.urlparse(url)
        if parsed.scheme not in ("http", "https") or not parsed.hostname:
            raise ValueError("network camera requires an HTTP(S) URL")
        if not 0 < timeout_s <= 5 or capture_fps <= 0:
            raise ValueError("network camera requires positive FPS and timeout <= 5 seconds")
        self._url, self._token = url, token
        self._period, self._timeout = 1 / capture_fps, timeout_s
        self._detector = LaneProcessor(process_fps)
        self._opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), _NoRedirect())
        self._stop = threading.Event()
        self._lock = threading.Lock()
        self._latest = None
        self._status = SampleStatus.UNAVAILABLE
        self.last_error = None
        self._thread = None
        if start:
            self._thread = threading.Thread(target=self._run, name="piguard-network-camera", daemon=True)
            self._thread.start()

    def _receive(self):
        headers = {"Authorization": "Bearer " + self._token} if self._token else {}
        request = urllib.request.Request(self._url, headers=headers)
        with self._opener.open(request, timeout=self._timeout) as response:
            data = response.read(2 * 1024 * 1024 + 1)
            if len(data) > 2 * 1024 * 1024:
                raise ValueError("camera frame is too large")
            frame_id = response.headers.get("X-Frame-ID")
            captured = response.headers.get("X-Captured-At")
            if not frame_id or not captured:
                raise ValueError("camera frame identity and capture timestamp are required")
        image = cv2.imdecode(np.frombuffer(data, dtype=np.uint8), cv2.IMREAD_COLOR)
        if image is None:
            raise ValueError("camera returned an invalid JPEG")
        now = datetime.now(timezone.utc)
        at = datetime.fromisoformat(captured.replace("Z", "+00:00"))
        if at.utcoffset() is None:
            raise ValueError("camera timestamp must include a timezone")
        age = (now - at).total_seconds()
        if age < -2:
            raise ValueError("camera clock is ahead of the Pi")
        ns = time.monotonic_ns() - int(max(0, age) * 1_000_000_000)
        with self._lock:
            # Repeated server frames keep their original sample time.
            if self._latest is None or not frame_id or frame_id != self._latest[0]:
                self._latest = (frame_id, _fit(image), at, ns)
            self._status = SampleStatus.OK
            self.last_error = None

    def _run(self):
        while not self._stop.is_set():
            began = time.monotonic()
            try:
                self._receive()
            except Exception as exc:
                with self._lock:
                    self._status = SampleStatus.TIMEOUT if isinstance(exc, TimeoutError) else SampleStatus.UNAVAILABLE
                    self.last_error = str(exc)
            self._stop.wait(max(0, self._period - (time.monotonic() - began)))

    def read(self, clock):
        at, ns = stamped(clock)
        with self._lock:
            latest, status = self._latest, self._status
        if self._stop.is_set() or status is not SampleStatus.OK or latest is None or ns - latest[3] > 1_000_000_000:
            self._detector.reset()
            return Frame(at, ns, "hardware", SampleStatus.UNAVAILABLE, None, invalid_lane(at))
        _, image, at, sampled_ns = latest
        return Frame(at, sampled_ns, "hardware", SampleStatus.OK, image, self._detector.read(image, at, sampled_ns))

    def release(self):
        self._stop.set()
        if self._thread is not None:
            self._thread.join()
