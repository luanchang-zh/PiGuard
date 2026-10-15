import io
import json
import time
import urllib.error
from dataclasses import replace

import pytest

from piguard_edge.transport.http import PlatformHttp


class Response(io.BytesIO):
    def __enter__(self): return self
    def __exit__(self, *args): self.close()


class Opener:
    def __init__(self, codes):
        self.codes = iter(codes)
        self.requests = []
    def open(self, request, timeout):
        self.requests.append(request)
        code = next(self.codes)
        if code != 200:
            raise urllib.error.HTTPError(request.full_url, code, 'test', {}, None)
        return Response(json.dumps({'code':0,'data':{'snapshot_id':'snap-uploaded'}}).encode())


def test_retry_reuses_entire_body_and_device_credentials(example_config):
    config = replace(example_config, http=replace(example_config.http, username='car-001', password='device-secret'))
    client = PlatformHttp(config)
    opener = Opener([500,200])
    client._opener = opener
    result = client.upload(b'jpeg-bytes','frame-once','2026-10-15T00:00:00Z','snapshot',time.monotonic()+2)
    assert result == 'snap-uploaded'
    assert len(opener.requests) == 2
    assert opener.requests[0].data == opener.requests[1].data
    assert opener.requests[0].get_header('Authorization').startswith('Basic ')
    assert b'device-secret' not in opener.requests[0].data


def test_authentication_rejection_is_not_retried(example_config):
    client = PlatformHttp(example_config)
    opener = Opener([401,200])
    client._opener = opener
    with pytest.raises(urllib.error.HTTPError):
        client.upload(b'jpeg','frame','2026-10-15T00:00:00Z','snapshot',time.monotonic()+2)
    assert len(opener.requests) == 1


def test_expired_upload_never_sends(example_config):
    client = PlatformHttp(example_config)
    opener = Opener([200])
    client._opener = opener
    with pytest.raises(TimeoutError):
        client.upload(b'jpeg','frame','2026-10-15T00:00:00Z','snapshot',time.monotonic()-1)
    assert opener.requests == []


def test_confirmation_filters_unrequested_ids(example_config, monkeypatch):
    client = PlatformHttp(example_config)
    monkeypatch.setattr(client,'_request',lambda *args:{'confirmed_event_ids':['ours','unrequested']})
    assert client.confirm([{'event_id':'ours'}]) == {'ours'}
