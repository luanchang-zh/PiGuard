#!/usr/bin/env python3
"""Publish live computer-camera JPEGs for the Pi; needs Python and FFmpeg only."""

import argparse
import hmac
import json
import os
import signal
import subprocess
import threading
import time
import uuid
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

MAX_JPEG = 2 * 1024 * 1024


class Capture:
    def __init__(self, device, width, height, fps):
        self.command = ["ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "v4l2",
                        "-framerate", str(fps), "-video_size", f"{width}x{height}", "-i", device,
                        "-an", "-threads", "1", "-q:v", "5", "-f", "image2pipe", "-vcodec", "mjpeg", "pipe:1"]
        self.lock = threading.Lock()
        self.stop = threading.Event()
        self.latest = None
        self.process = None
        self.error = None
        self.count = 0
        self.run_id = uuid.uuid4().hex[:12]
        self.thread = threading.Thread(target=self._run, name="pc-camera", daemon=True)
        self.thread.start()

    def _run(self):
        while not self.stop.is_set():
            process = None
            try:
                process = subprocess.Popen(self.command, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
                with self.lock: self.process = process
                buffer = bytearray()
                while not self.stop.is_set():
                    data = os.read(process.stdout.fileno(), 65536)
                    if not data: raise OSError("FFmpeg camera capture ended")
                    buffer.extend(data)
                    while True:
                        begin = buffer.find(b'\xff\xd8')
                        if begin < 0:
                            buffer[:] = buffer[-1:]; break
                        end = buffer.find(b'\xff\xd9', begin + 2)
                        if end < 0:
                            if begin: del buffer[:begin]
                            break
                        jpeg = bytes(buffer[begin:end + 2]); del buffer[:end + 2]
                        if len(jpeg) > MAX_JPEG: raise ValueError("camera JPEG exceeds 2 MiB")
                        with self.lock:
                            self.count += 1
                            self.latest = (jpeg, f"{self.run_id}-{self.count}", datetime.now(timezone.utc).isoformat(), time.monotonic())
                            self.error = None
                    if len(buffer) > MAX_JPEG: raise ValueError("camera JPEG exceeds 2 MiB")
            except Exception as exc:
                with self.lock: self.error = str(exc)
            finally:
                if process is not None:
                    self._stop_process(process)
                    process.stdout.close()
                with self.lock: self.process = None
            self.stop.wait(1)

    def close(self):
        self.stop.set()
        with self.lock: process = self.process
        if process is not None: self._stop_process(process)
        self.thread.join()

    @staticmethod
    def _stop_process(process):
        if process.poll() is None:
            try: process.terminate()
            except ProcessLookupError: pass
        try: process.wait(2)
        except subprocess.TimeoutExpired:
            try: process.kill()
            except ProcessLookupError: pass
            process.wait()


def make_handler(capture, token):
    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_GET(self):
            if not hmac.compare_digest(self.headers.get("Authorization", ""), "Bearer " + token):
                self.send_error(401); return
            with capture.lock:
                latest, error, count = capture.latest, capture.error, capture.count
            ready = latest is not None and time.monotonic() - latest[3] <= 2
            if self.path == "/health":
                body = json.dumps({"ready": ready, "frames": count, "error": error}).encode()
                self.send_response(200 if ready else 503)
                self.send_header("Content-Type", "application/json")
            elif self.path == "/frame.jpg":
                if not ready:
                    self.send_error(503, "Camera frame unavailable"); return
                body, frame_id, captured_at, _ = latest
                self.send_response(200)
                self.send_header("Content-Type", "image/jpeg")
                self.send_header("X-Frame-ID", frame_id)
                self.send_header("X-Captured-At", captured_at)
            else:
                self.send_error(404); return
            self.send_header("Cache-Control", "no-store")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            try: self.wfile.write(body)
            except (BrokenPipeError, ConnectionResetError): pass
    return Handler


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--device", default="/dev/video0")
    parser.add_argument("--bind", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8090)
    parser.add_argument("--width", type=int, default=640)
    parser.add_argument("--height", type=int, default=480)
    parser.add_argument("--fps", type=int, default=10)
    parser.add_argument("--token", default="piguard-camera-demo")
    args = parser.parse_args()
    if not args.token or min(args.width, args.height, args.fps) <= 0:
        parser.error("token and positive width/height/fps are required")
    capture = Capture(args.device, args.width, args.height, args.fps)
    try:
        server = ThreadingHTTPServer((args.bind, args.port), make_handler(capture, args.token))
        server.daemon_threads = True
        server.timeout = .5
        stop = threading.Event()
        signal.signal(signal.SIGINT, lambda *_: stop.set())
        signal.signal(signal.SIGTERM, lambda *_: stop.set())
        print(f"Camera endpoint: http://{args.bind}:{args.port}/frame.jpg", flush=True)
        try:
            while not stop.is_set(): server.handle_request()
        finally:
            server.server_close()
    finally:
        capture.close()


if __name__ == "__main__":
    main()
