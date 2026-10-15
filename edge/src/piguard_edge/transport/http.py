"""Device-scoped HTTP work. Retries reuse the same upload identity and bytes."""

import base64
import http.client
import json
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from concurrent.futures import ThreadPoolExecutor

from piguard_edge.config.model import EdgeConfig


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class PlatformHttp:
    def __init__(self, config: EdgeConfig) -> None:
        self._base = config.http.base_url.rstrip("/")
        self._device = urllib.parse.quote(config.device.id, safe="")
        self._timeout = config.http.timeout_s
        self._headers = {}
        if config.http.username:
            token = base64.b64encode(f"{config.http.username}:{config.http.password}".encode()).decode()
            self._headers["Authorization"] = "Basic " + token
        self._opener = urllib.request.build_opener(_NoRedirect())

    def _request(self, path: str, data: bytes, content_type: str, timeout: float):
        request = urllib.request.Request(self._base + path, data=data, headers={**self._headers, "Content-Type": content_type}, method="POST")
        with self._opener.open(request, timeout=timeout) as response:
            # Limit untrusted responses; no credentials or response text enter diagnostics.
            raw = response.read(256 * 1024 + 1)
            if len(raw) > 256 * 1024:
                raise ValueError("HTTP response too large")
            body = json.loads(raw)
            if body.get("code") != 0:
                raise ValueError("platform rejected request")
            return body["data"]

    def confirm(self, events: list[dict]) -> set[str]:
        body = self._request(f"/devices/{self._device}/events/confirmations", json.dumps({"events": events}).encode(), "application/json", self._timeout)
        ids = body.get("confirmed_event_ids")
        if not isinstance(ids, list) or not all(isinstance(x, str) for x in ids):
            raise ValueError("invalid confirmation response")
        requested = {x["event_id"] for x in events}
        return set(ids) & requested

    def upload(self, jpeg: bytes, frame_id: str, captured_at: str, kind: str, deadline: float) -> str:
        if len(jpeg) > 2 * 1024 * 1024:
            raise ValueError("JPEG too large")
        boundary = "piguard-" + uuid.uuid4().hex
        chunks = []
        for name, value in {"frame_id": frame_id, "captured_at": captured_at, "type": kind}.items():
            chunks.append(f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"\r\n\r\n{value}\r\n'.encode())
        chunks.extend([f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="frame.jpg"\r\nContent-Type: image/jpeg\r\n\r\n'.encode(), jpeg, f'\r\n--{boundary}--\r\n'.encode()])
        data = b"".join(chunks)
        for attempt in range(3):
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise TimeoutError("upload deadline exceeded")
            try:
                result = self._request(f"/devices/{self._device}/frames", data, f"multipart/form-data; boundary={boundary}", min(self._timeout, remaining))
                snapshot = result.get("snapshot_id")
                if not isinstance(snapshot, str) or not snapshot or len(snapshot) > 64:
                    raise ValueError("invalid snapshot response")
                return snapshot
            except urllib.error.HTTPError as exc:
                if exc.code not in (408, 429) and exc.code < 500:
                    raise
            except (OSError, urllib.error.URLError, http.client.HTTPException):
                pass
            if attempt < 2:
                time.sleep(min(0.1, max(0, deadline - time.monotonic())))
        raise OSError("upload failed")


class HttpWorker:
    def __init__(self, client: PlatformHttp) -> None:
        self.client = client
        self._pool = ThreadPoolExecutor(max_workers=2, thread_name_prefix="piguard-http")

    def submit(self, function, *args):
        return self._pool.submit(function, *args)

    def close(self) -> None:
        self._pool.shutdown(wait=True, cancel_futures=True)
