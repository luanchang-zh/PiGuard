import json
import threading
import time
from contextlib import contextmanager
from dataclasses import replace
from http.server import BaseHTTPRequestHandler
from urllib.error import HTTPError
from urllib.request import ProxyHandler, Request, build_opener

import cv2
import pytest
import yaml

from piguard_edge.config.loader import ConfigError, load_config
from piguard_edge.drivers.camera.paint import paint_road
from piguard_edge.drivers.camera.remote import RemoteCamera
from piguard_edge.models.sample import SampleStatus
from piguard_edge.pc_camera import CameraServer, CapturedJpeg, Webcam, main
from piguard_edge.runtime.app import EdgeApp
from piguard_edge.runtime.clock import SystemClock
from piguard_edge.runtime.workers import Sampler
from piguard_edge.simulation.engine import ScenarioEngine
from piguard_edge.storage.commands import CommandStore
from piguard_edge.storage.events import EventStore
from piguard_edge.transport.mqtt import FakeMqtt, MqttMessage

TOKEN = "test-camera-token"


def jpeg():
    ok, image = cv2.imencode(".jpg", paint_road(0.3))
    assert ok
    return image.tobytes()


class CameraStub:
    def __init__(self):
        self.frame = CapturedJpeg(jpeg(), time.monotonic_ns())
        self.live = True

    def latest(self):
        if self.live and self.frame is not None:
            return CapturedJpeg(self.frame.data, time.monotonic_ns())
        return self.frame


@contextmanager
def camera_server(source=None, handler=None):
    source = CameraStub() if source is None else source
    with CameraServer(("127.0.0.1", 0), source, TOKEN) as server:
        if handler is not None:
            server.RequestHandlerClass = handler
        thread = threading.Thread(target=server.serve_forever, kwargs={"poll_interval": 0.01}, daemon=True)
        thread.start()
        try:
            yield f"http://127.0.0.1:{server.server_port}", source
        finally:
            server.shutdown()
            thread.join(timeout=2)


def wait_for(read, predicate, timeout=3):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        result = read()
        if predicate(result):
            return result
        time.sleep(0.01)
    pytest.fail("camera did not reach the expected state")


def remote_config(config, url, **kwargs):
    return replace(config.sensors.camera, driver="remote", remote_url=url + "/frame.jpg", **kwargs)


@pytest.fixture(autouse=True)
def token(monkeypatch):
    monkeypatch.setenv("PIGUARD_CAMERA_TOKEN", TOKEN)


def test_server_authentication_health_and_expired_frames():
    opener = build_opener(ProxyHandler({}))
    with camera_server() as (url, source):
        with pytest.raises(HTTPError) as exc:
            opener.open(url + "/frame.jpg")
        assert exc.value.code == 401
        headers = {"Authorization": f"Bearer {TOKEN}"}
        with opener.open(Request(url + "/frame.jpg", headers=headers)) as response:
            assert response.headers["Content-Type"] == "image/jpeg"
            assert response.headers["Cache-Control"] == "no-store"
            assert float(response.headers["X-Frame-Age-Ms"]) >= 0
            assert response.read() == source.frame.data
        with opener.open(Request(url + "/healthz", headers=headers)) as response:
            assert response.read() == b"ok\n"
        source.frame = CapturedJpeg(source.frame.data, time.monotonic_ns() - 2_000_000_000)
        source.live = False
        for endpoint in ("/frame.jpg", "/healthz"):
            with pytest.raises(HTTPError) as exc:
                opener.open(Request(url + endpoint, headers=headers))
            assert exc.value.code == 503


def test_remote_pixels_lane_disconnect_recovery_and_close(example_config):
    clock = SystemClock()
    with camera_server() as (url, source):
        camera = RemoteCamera(remote_config(example_config, url))
        try:
            frame = wait_for(lambda: camera.read(clock), lambda f: f.status is SampleStatus.OK)
            assert frame.source == "hardware"
            assert frame.image.shape == (480, 640, 3)
            assert frame.lane.offset_ratio == pytest.approx(0.3, abs=0.04)
            assert frame.sample_at <= clock.utc()
            source.frame = None
            failed = wait_for(lambda: camera.read(clock), lambda f: f.status is SampleStatus.UNAVAILABLE)
            assert failed.image is None and not failed.lane.valid
            source.frame = CapturedJpeg(jpeg(), time.monotonic_ns())
            wait_for(lambda: camera.read(clock), lambda f: f.status is SampleStatus.OK)
        finally:
            camera.release()
        assert not camera._thread.is_alive()
        assert camera.read(clock).image is None


def test_cached_frame_does_not_become_fresh_on_each_read(example_config):
    clock = SystemClock()
    with camera_server() as (url, source):
        source.live = False
        camera = RemoteCamera(remote_config(example_config, url, remote_max_age_ms=200))
        try:
            first = wait_for(lambda: camera.read(clock), lambda f: f.status is SampleStatus.OK)
            last = wait_for(lambda: camera.read(clock), lambda f: f.status is SampleStatus.TIMEOUT)
            assert first.image is not None
            assert last.image is None and not last.lane.valid
        finally:
            camera.release()


@pytest.mark.parametrize("kind", ["wrong-token", "corrupt-jpeg"])
def test_authentication_and_bad_pixels_are_explicit_errors(example_config, monkeypatch, kind):
    with camera_server() as (url, source):
        if kind == "wrong-token":
            monkeypatch.setenv("PIGUARD_CAMERA_TOKEN", "incorrect-token")
        else:
            source.frame = CapturedJpeg(b"\xff\xd8broken-jpeg", time.monotonic_ns())
        camera = RemoteCamera(remote_config(example_config, url))
        try:
            failed = wait_for(lambda: camera.read(SystemClock()), lambda f: f.status is SampleStatus.ERROR)
            assert failed.image is None and not failed.lane.valid
        finally:
            camera.release()


@pytest.mark.parametrize("case, expected", [
    ("missing-age", SampleStatus.ERROR), ("infinite-age", SampleStatus.ERROR),
    ("huge-age", SampleStatus.TIMEOUT), ("wrong-type", SampleStatus.ERROR),
    ("oversize", SampleStatus.ERROR), ("redirect", SampleStatus.ERROR),
    ("truncated", SampleStatus.ERROR),
])
def test_malformed_wire_responses_cannot_supply_a_frame(example_config, case, expected):
    paths = []
    class WireHandler(BaseHTTPRequestHandler):
        def do_GET(self):
            paths.append(self.path)
            body = jpeg() if case != "oversize" else b"\xff\xd8" + bytes(2 * 1024 * 1024)
            self.send_response(302 if case == "redirect" else 200)
            self.send_header("Content-Type", "text/html" if case == "wrong-type" else "image/jpeg")
            self.send_header("Content-Length", str(len(body) + (100 if case == "truncated" else 0)))
            if case != "missing-age":
                self.send_header("X-Frame-Age-Ms", {"infinite-age": "inf", "huge-age": "1e308"}.get(case, "0"))
            if case == "redirect":
                self.send_header("Location", "/redirect-target")
            self.end_headers()
            try:
                self.wfile.write(body)
            except OSError:
                pass
        def log_message(self, *args):
            pass
    with camera_server(handler=WireHandler) as (url, _):
        camera = RemoteCamera(remote_config(example_config, url))
        try:
            frame = wait_for(lambda: camera.read(SystemClock()), lambda f: f.status is expected)
            assert frame.image is None and not frame.lane.valid
            assert paths and all(path == "/frame.jpg" for path in paths)
        finally:
            camera.release()


def test_slow_camera_request_does_not_block_other_samples(example_config):
    entered, unblock = threading.Event(), threading.Event()
    class SlowCamera:
        def latest(self):
            entered.set()
            unblock.wait(2)
            return None
    with camera_server(SlowCamera()) as (url, _):
        config = replace(example_config, sensors=replace(example_config.sensors,
                         camera=remote_config(example_config, url, remote_timeout_s=0.05)))
        clock = SystemClock()
        sampler = Sampler.from_config(config, ScenarioEngine("normal_drive", 1), clock)
        try:
            assert entered.wait(1)
            # Drive due sampling while the HTTP handler is still waiting.
            sampler._next_ns = {name: 0 for name in sampler._next_ns}
            started = time.monotonic()
            sampler.step()
            assert time.monotonic() - started < 0.1
            assert sampler.state.gyro.status is SampleStatus.OK
            assert sampler.state.distance.status is SampleStatus.OK
            assert sampler.state.frame.image is None
            wait_for(lambda: sampler.sources.camera.read(clock), lambda f: f.status is SampleStatus.TIMEOUT)
        finally:
            unblock.set()
            sampler.close()


def test_snapshot_upload_uses_pc_pixels_and_duplicate_command_is_idempotent(example_config):
    from concurrent.futures import Future
    from datetime import timedelta
    from piguard_edge.models.sample import format_utc

    class UploadWorker:
        def __init__(self):
            self.client = self
            self.uploads = []
        def submit(self, fn, *args):
            future = Future()
            future.set_result(fn(*args))
            return future
        def upload(self, data, frame_id, at, kind, deadline):
            self.uploads.append((data, kind))
            return "pc-snapshot"
        def close(self):
            pass

    with camera_server() as (url, _):
        config = replace(example_config, sensors=replace(example_config.sensors,
                         camera=remote_config(example_config, url)),
                         features=replace(example_config.features, frame_upload=True))
        clock, transport, worker = SystemClock(), FakeMqtt(), UploadWorker()
        engine = ScenarioEngine("normal_drive", 1)
        sampler = Sampler.from_config(config, engine, clock)
        app = EdgeApp(config, clock, transport, sampler, store=EventStore(":memory:"),
                      command_store=CommandStore(":memory:"), engine=engine, http_worker=worker)
        try:
            wait_for(lambda: sampler.sources.camera.read(clock), lambda f: f.status is SampleStatus.OK)
            sampler._next_ns = {name: 0 for name in sampler._next_ns}
            app.start()
            app.step()
            payload = json.dumps({"schema_version": 1, "command_id": "pc-shot", "device_id": "car-001",
                                  "type": "camera.snapshot", "issued_at": format_utc(clock.utc()),
                                  "expires_at": format_utc(clock.utc() + timedelta(seconds=10)),
                                  "params": {"quality": 80}}).encode()
            command = MqttMessage("car/car-001/commands", payload, 1, False)
            transport.deliver(command)
            app.step()
            app.step()
            transport.deliver(command)
            app.step()
            shots = [data for data, kind in worker.uploads if kind == "snapshot"]
            assert len(shots) == 1 and shots[0].startswith(b"\xff\xd8")
            acks = [json.loads(msg.payload) for msg in transport.published if msg.topic.endswith("/command-acks")]
            assert acks[-1]["status"] == "success"
            assert acks[-1]["result"] == {"snapshot_id": "pc-snapshot"}
        finally:
            app.stop()


def test_webcam_releases_and_reopens_after_read_failure(monkeypatch):
    import piguard_edge.pc_camera as module
    opened, recovered = [], threading.Event()
    class FakeCapture:
        def __init__(self):
            self.released = False
            opened.append(self)
        def isOpened(self):
            return True
        def set(self, *args):
            pass
        def read(self):
            if len(opened) == 1:
                return False, None
            recovered.set()
            time.sleep(0.01)
            return True, paint_road(0)
        def release(self):
            self.released = True
    monkeypatch.setattr(module.cv2, "VideoCapture", lambda *args: FakeCapture())
    webcam = Webcam("/dev/video0", 640, 480, 10, 85)
    webcam.start()
    try:
        assert recovered.wait(2)
        frame = wait_for(webcam.latest, lambda f: f is not None)
        assert frame.data.startswith(b"\xff\xd8")
    finally:
        webcam.close()
    assert len(opened) == 2 and all(item.released for item in opened)


@pytest.mark.parametrize("override", [
    {"remote_url": ""}, {"remote_url": "file:///frame.jpg"},
    {"remote_url": "http://user:pass@localhost/frame.jpg"},
    {"remote_url": "http://localhost:99999/frame.jpg"},
    {"remote_url": "http://localhost/frame.jpg?token=secret"},
    {"remote_timeout_s": 0}, {"remote_timeout_s": float("nan")},
    {"remote_timeout_s": 6}, {"remote_max_age_ms": 0},
])
def test_invalid_remote_configuration_is_rejected(example_config, tmp_path, override):
    data = yaml.safe_load(example_config.source_path.read_text(encoding="utf-8"))
    data["sensors"]["camera"].update({"driver": "remote", "remote_url": "http://192.168.7.1:8765/frame.jpg", **override})
    path = tmp_path / "config.yaml"
    path.write_text(yaml.safe_dump(data), encoding="utf-8")
    with pytest.raises(ConfigError):
        load_config(path)


def test_remote_requires_a_token_before_starting(example_config, monkeypatch):
    monkeypatch.delenv("PIGUARD_CAMERA_TOKEN")
    with pytest.raises(ValueError, match="PIGUARD_CAMERA_TOKEN"):
        RemoteCamera(remote_config(example_config, "http://192.168.7.1:8765"))
    with pytest.raises(SystemExit):
        main([])
