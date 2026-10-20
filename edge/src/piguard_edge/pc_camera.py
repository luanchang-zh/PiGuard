"""Share the Ubuntu computer's webcam over a USB Ethernet link."""

import argparse
import hmac
import logging
import signal
import threading
import time
from dataclasses import dataclass
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import cv2

from piguard_edge.drivers.camera.bridge import AGE_HEADER, MAX_JPEG_BYTES, camera_token

_LOG = logging.getLogger(__name__)


@dataclass(frozen=True, slots=True)
class CapturedJpeg:
    data: bytes
    monotonic_ns: int


class Webcam:
    def __init__(self, device: str, width: int, height: int, fps: int, quality: int) -> None:
        self._device = device
        self._width, self._height = width, height
        self._fps, self._quality = fps, quality
        self._lock = threading.Lock()
        self._latest: CapturedJpeg | None = None
        self._stop = threading.Event()
        self._thread = threading.Thread(target=self._run, name="pc-webcam", daemon=True)

    def start(self) -> None:
        self._thread.start()

    def latest(self) -> CapturedJpeg | None:
        with self._lock:
            return self._latest

    def close(self) -> None:
        self._stop.set()
        self._thread.join(timeout=2)

    def _run(self) -> None:
        while not self._stop.is_set():
            capture = cv2.VideoCapture(self._device, cv2.CAP_V4L2)
            try:
                if not capture.isOpened():
                    _LOG.warning("Cannot open %s; check device permissions or camera use by another app", self._device)
                else:
                    capture.set(cv2.CAP_PROP_FRAME_WIDTH, self._width)
                    capture.set(cv2.CAP_PROP_FRAME_HEIGHT, self._height)
                    capture.set(cv2.CAP_PROP_FPS, self._fps)
                    capture.set(cv2.CAP_PROP_BUFFERSIZE, 1)
                    self._capture(capture)
            except cv2.error:
                _LOG.warning("Webcam capture failed; retrying")
            finally:
                capture.release()
                with self._lock:
                    self._latest = None
            self._stop.wait(1)

    def _capture(self, capture) -> None:
        next_encode = 0
        while not self._stop.is_set():
            ok, image = capture.read()
            captured_ns = time.monotonic_ns()
            if not ok or image is None:
                _LOG.warning("Webcam disconnected or could not read a frame; retrying")
                return
            # Keep draining the camera even if it ignores the requested FPS.
            if captured_ns < next_encode:
                continue
            next_encode = captured_ns + 1_000_000_000 // self._fps
            image = cv2.resize(image, (self._width, self._height))
            ok, encoded = cv2.imencode(".jpg", image, [cv2.IMWRITE_JPEG_QUALITY, self._quality])
            if not ok or encoded.nbytes > MAX_JPEG_BYTES:
                with self._lock:
                    self._latest = None
                continue
            with self._lock:
                self._latest = CapturedJpeg(encoded.tobytes(), captured_ns)


class CameraServer(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, address: tuple[str, int], webcam: Webcam, token: str, max_age_ms: int = 1000):
        self.webcam = webcam
        self.token = token
        self.max_age_ms = max_age_ms
        super().__init__(address, CameraHandler)


class CameraHandler(BaseHTTPRequestHandler):
    def setup(self) -> None:
        super().setup()
        self.connection.settimeout(2)

    def do_GET(self) -> None:
        authorization = self.headers.get("Authorization", "")
        if not hmac.compare_digest(authorization.encode("utf-8"), f"Bearer {self.server.token}".encode("ascii")):
            self._reply(401, b"camera token required\n")
            return
        if self.path not in {"/frame.jpg", "/healthz"}:
            self._reply(404, b"not found\n")
            return
        frame = self.server.webcam.latest()
        age_ms = None if frame is None else max(0, time.monotonic_ns() - frame.monotonic_ns) / 1_000_000
        if age_ms is None or age_ms > self.server.max_age_ms:
            self._reply(503, b"camera unavailable or frame expired\n")
            return
        if self.path == "/healthz":
            self._reply(200, b"ok\n")
        else:
            self._reply(200, frame.data, "image/jpeg", age_ms)

    def _reply(self, status: int, body: bytes, content_type: str = "text/plain", age_ms: float | None = None) -> None:
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        if age_ms is not None:
            self.send_header(AGE_HEADER, f"{age_ms:.3f}")
        self.end_headers()
        try:
            self.wfile.write(body)
        except (OSError, TimeoutError):
            pass  # A USB disconnect can close an in-flight response.

    def log_message(self, format, *args) -> None:
        return  # Frame polling is frequent; never log credentials or query text.


def main(argv: list[str] | None = None) -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="127.0.0.1", help="bind to the Ubuntu USB interface IP")
    parser.add_argument("--port", type=int, default=8765)
    parser.add_argument("--device", default="/dev/video0")
    parser.add_argument("--width", type=int, default=640)
    parser.add_argument("--height", type=int, default=480)
    parser.add_argument("--fps", type=int, default=10)
    parser.add_argument("--quality", type=int, default=85)
    parser.add_argument("--max-age-ms", type=int, default=1000)
    parser.add_argument("--token-env", default="PIGUARD_CAMERA_TOKEN")
    args = parser.parse_args(argv)
    if not (1 <= args.port <= 65535 and 1 <= args.width <= 1920 and 1 <= args.height <= 1080
            and 1 <= args.fps <= 60 and 1 <= args.quality <= 95 and args.max_age_ms > 0):
        parser.error("invalid port, image dimensions, fps, quality or max age")
    try:
        token = camera_token(args.token_env)
    except ValueError as exc:
        parser.error(str(exc))
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    webcam = Webcam(args.device, args.width, args.height, args.fps, args.quality)
    with CameraServer((args.host, args.port), webcam, token, args.max_age_ms) as server:
        stop = threading.Event()
        for sig in (signal.SIGINT, signal.SIGTERM):
            signal.signal(sig, lambda *_: stop.set())
        server.timeout = 0.2
        webcam.start()
        _LOG.info("Camera server listening on %s:%s using %s", args.host, args.port, args.device)
        try:
            while not stop.is_set():
                server.handle_request()
        finally:
            webcam.close()


if __name__ == "__main__":
    main()
