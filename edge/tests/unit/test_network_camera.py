"""PC-camera HTTP contract tested against generated JPEGs, without a webcam."""

import importlib.util
import io
import threading
import time
import urllib.error
import urllib.request
from datetime import datetime, timedelta, timezone
from http.server import ThreadingHTTPServer
from pathlib import Path
from types import SimpleNamespace

import cv2
import pytest

from piguard_edge.drivers.camera.network import NetworkCamera
from piguard_edge.drivers.camera.paint import paint_road
from piguard_edge.models.sample import SampleStatus
from piguard_edge.protocol.telemetry import build_telemetry
from piguard_edge.runtime.clock import FakeClock, SystemClock
from piguard_edge.runtime.state import LatestState

SCRIPT = Path(__file__).resolve().parents[2] / 'scripts/serve-pc-camera.py'
spec = importlib.util.spec_from_file_location('piguard_pc_camera', SCRIPT)
pc_camera = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pc_camera)


def jpeg():
    success, data = cv2.imencode('.jpg', paint_road(.2))
    assert success
    return data.tobytes()


@pytest.fixture
def camera_server():
    capture = SimpleNamespace(lock=threading.Lock(), error=None, count=1,
                              latest=(jpeg(), 'frame-1', datetime.now(timezone.utc).isoformat(), time.monotonic()))
    base = pc_camera.make_handler(capture, 'test-token')
    class Handler(base):
        def do_GET(self):
            if self.path == '/redirect':
                self.send_response(302)
                self.send_header('Location', '/frame.jpg')
                self.end_headers()
                return
            super().do_GET()
    server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    thread = threading.Thread(target=server.serve_forever, kwargs={'poll_interval': .01}, daemon=True)
    thread.start()
    try:
        yield f'http://127.0.0.1:{server.server_port}', capture
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


def test_pc_server_and_client_produce_real_frame_lane_and_metadata(camera_server):
    url, capture = camera_server
    camera = NetworkCamera(url + '/frame.jpg', 'test-token', start=False)
    try:
        camera._receive()
        frame = camera.read(SystemClock())
        assert frame.status is SampleStatus.OK and frame.source == 'hardware'
        assert frame.image.shape == (480, 640, 3)
        assert frame.lane.valid
        data = build_telemetry(LatestState(frame=frame), device_id='car-001', timestamp=SystemClock().utc(),
                               seq=0, message_id='network-test', distance_scale=100, source='hardware')
        assert data['lane']['source'] == 'hardware' and data['lane']['status'] == 'ok'
        original = camera._latest
        camera._receive()
        assert camera._latest is original  # Repeated frame ID never refreshes sample time.
        clock = FakeClock(original[3] + 1_000_000_001)
        expired = camera.read(clock)
        assert expired.status is SampleStatus.UNAVAILABLE and expired.image is None
        assert not expired.lane.valid
    finally:
        camera.release()


def test_pc_server_authentication_and_stale_frames(camera_server):
    url, capture = camera_server
    direct = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    for path in ('/health', '/frame.jpg'):
        with pytest.raises(urllib.error.HTTPError) as error:
            direct.open(url + path)
        assert error.value.code == 401
    capture.latest = (*capture.latest[:3], time.monotonic() - 3)
    for path in ('/health', '/frame.jpg'):
        request = urllib.request.Request(url + path, headers={'Authorization': 'Bearer test-token'})
        with pytest.raises(urllib.error.HTTPError) as error:
            direct.open(request)
        assert error.value.code == 503


def test_camera_rejects_redirects(camera_server):
    url, _ = camera_server
    camera = NetworkCamera(url + '/redirect', 'test-token', start=False)
    try:
        with pytest.raises(urllib.error.HTTPError) as error: camera._receive()
        assert error.value.code == 302
    finally:
        camera.release()


def test_platform_http_bypasses_environment_proxy(camera_server, example_config, monkeypatch):
    from piguard_edge.transport.http import PlatformHttp
    url, _ = camera_server
    monkeypatch.setenv('http_proxy', 'http://127.0.0.1:1')
    monkeypatch.setenv('no_proxy', '')
    client = PlatformHttp(example_config)
    request = urllib.request.Request(url + '/health', headers={'Authorization': 'Bearer test-token'})
    with client._opener.open(request, timeout=1) as response:
        assert response.status == 200


class Response(io.BytesIO):
    def __init__(self, data, headers):
        super().__init__(data)
        self.headers = headers


@pytest.mark.parametrize('data,headers', [
    (b'not jpeg', {'X-Frame-ID': '1', 'X-Captured-At': '2026-01-01T00:00:00Z'}),
    (b'x' * (2 * 1024 * 1024 + 1), {}),
    (jpeg(), {}),
    (jpeg(), {'X-Frame-ID': '1', 'X-Captured-At': '2026-01-01T00:00:00'}),
    (jpeg(), {'X-Frame-ID': '1', 'X-Captured-At': (datetime.now(timezone.utc) + timedelta(hours=1)).isoformat()}),
])
def test_invalid_remote_frames_are_rejected(data, headers):
    camera = NetworkCamera('http://127.0.0.1/frame.jpg', start=False)
    camera._opener = SimpleNamespace(open=lambda *args, **kwargs: Response(data, headers))
    try:
        with pytest.raises(ValueError): camera._receive()
        assert camera.read(SystemClock()).status is SampleStatus.UNAVAILABLE
    finally:
        camera.release()


def test_camera_disconnect_clears_cached_frame_without_blocking_read():
    entered, resume = threading.Event(), threading.Event()
    camera = NetworkCamera('http://127.0.0.1/frame.jpg', start=False)
    def offline(*args, **kwargs):
        entered.set()
        resume.wait(2)
        raise TimeoutError('camera disconnected')
    camera._opener = SimpleNamespace(open=offline)
    camera._latest = ('old', paint_road(0), datetime.now(timezone.utc), time.monotonic_ns())
    camera._status = SampleStatus.OK
    camera._thread = threading.Thread(target=camera._run)
    camera._thread.start()
    try:
        assert entered.wait(1)
        assert camera.read(SystemClock()).image is not None
        resume.set()
        camera.release()
        assert camera._status is SampleStatus.TIMEOUT
        assert camera.read(SystemClock()).image is None
    finally:
        resume.set()
        camera.release()


def test_pc_capture_parses_fragmented_jpegs_without_launching_ffmpeg(monkeypatch):
    capture = pc_camera.Capture.__new__(pc_camera.Capture)
    capture.command, capture.latest, capture.process, capture.error = [], None, None, None
    capture.lock, capture.stop, capture.count, capture.run_id = threading.Lock(), threading.Event(), 0, 'fake'
    process = SimpleNamespace(stdout=SimpleNamespace(fileno=lambda: 7, close=lambda: None),
                              poll=lambda: 0, wait=lambda *args: 0)
    monkeypatch.setattr(pc_camera.subprocess, 'Popen', lambda *args, **kwargs: process)
    chunks = iter([b'noise\xff', b'\xd8one\xff', b'\xd9\xff\xd8two\xff\xd9'])
    def read(*args):
        try: return next(chunks)
        except StopIteration:
            capture.stop.set()
            return b''
    monkeypatch.setattr(pc_camera.os, 'read', read)
    capture._run()
    assert capture.count == 2
    assert capture.latest[:2] == (b'\xff\xd8two\xff\xd9', 'fake-2')
