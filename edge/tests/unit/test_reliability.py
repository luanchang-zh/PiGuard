import json
from concurrent.futures import Future
from dataclasses import replace

import pytest

from piguard_edge.protocol.config import merge_rules
from piguard_edge.protocol.event import make_event
from piguard_edge.models.sample import format_utc
from piguard_edge.runtime.app import EdgeApp
from piguard_edge.runtime.clock import FakeClock
from piguard_edge.transport.mqtt import FakeMqtt, MqttMessage, Published, PahoMqtt
from test_uplink import _app, _StubClient, app_will
from test_commands import _message

RULES = {
    'obstacle': {'warning_distance_m': 25, 'danger_distance_m': 10},
    'lane_departure': {'offset_threshold': .3, 'duration_ms': 500},
    'sharp_turn': {'yaw_threshold_deg_s': 25, 'min_speed_kmh': 15, 'duration_ms': 200},
    'temperature': {'trigger_c': 34, 'recover_c': 31, 'duration_ms': 1000},
}


def test_initial_broker_outage_retries_without_stopping_sampling(example_config):
    app, clock, transport = _app(example_config)
    transport.fail_connect = True
    app.start()
    clock.advance_ns(100_000_000)
    app.step()
    assert app._sampler.state.speed.value == 30
    transport.fail_connect = False
    clock.advance_ns(1_000_000_000)
    app.step()
    assert app._connected
    assert any(x.topic.endswith('/telemetry') for x in transport.published)
    app.stop()


def test_qos_receipt_controls_event_and_ack_persistence(example_config):
    app, clock, transport = _app(example_config)
    class Receipt:
        acknowledged = False
        def is_published(self): return self.acknowledged
    receipt = Receipt()
    publish = transport.publish
    def delayed(message):
        publish(message)
        return receipt if message.topic.endswith(('/events', '/command-acks')) else Published()
    transport.publish = delayed
    app.start()
    event = make_event(device_id='car-001', event_id='event-delayed', event_type='sensor_failure', action='started', level='warning', timestamp=clock.utc(), data={'sensor': 'gyro', 'reason': 'error'})
    app._store.append(event)
    transport.deliver(_message(clock, 'command-delayed', 'scenario.stop', {}))
    app.step()
    app.step()
    assert [x.event_id for x in app._store.unpublished()] == ['event-delayed']
    assert [x.command_id for x in app._commands.unpublished()] == ['command-delayed']
    assert sum(x.topic.endswith('/events') for x in transport.published) == 1
    receipt.acknowledged = True
    app.step()
    assert app._store.unpublished() == []
    assert app._commands.unpublished() == []
    app.stop()


def test_publish_rejection_is_retryable_and_does_not_stop_rules(example_config):
    app, clock, transport = _app(example_config)
    app.start()
    transport.publish = lambda message: (_ for _ in ()).throw(OSError('offline'))
    clock.advance_ns(1_000_000_000)
    app.step()
    assert app._sampler.state.speed.value == 30
    assert app.network_error
    app.stop()


def test_paho_rejects_unqueued_publish(example_config):
    client = _StubClient()
    client.publish = lambda *args: type('Info', (), {'rc': 4})()
    with pytest.raises(OSError):
        PahoMqtt(example_config, client).publish(app_will())


def configured(example_config, tmp_path):
    return replace(example_config, storage=replace(example_config.storage, sqlite_path=tmp_path/'edge.db'), features=replace(example_config.features, remote_config=True))


def config_message(clock, version=1, rules=None):
    return MqttMessage('car/car-001/config', json.dumps({'schema_version':1,'device_id':'car-001','config_version':version,'issued_at':format_utc(clock.utc()),'rules':RULES if rules is None else rules}).encode(), 1, True)


def test_configuration_persists_replays_and_preserves_first_outcome(example_config, tmp_path):
    config = configured(example_config, tmp_path)
    clock, transport = FakeClock(), FakeMqtt()
    app = EdgeApp.from_config(config, clock, transport)
    app.start()
    message = config_message(clock)
    transport.deliver(message)
    app.step()
    assert app._config.rules.obstacle.warning_distance_m == 25
    ack = [x.payload for x in transport.published if x.topic.endswith('/config-acks')][-1]
    clock.advance_ns(1_000_000_000)
    transport.deliver(message)
    app.step()
    assert [x.payload for x in transport.published if x.topic.endswith('/config-acks')][-1] == ack
    conflicting = {**RULES, 'obstacle': {'warning_distance_m': 40, 'danger_distance_m': 10}}
    transport.deliver(config_message(clock, rules=conflicting))
    app.step()
    assert app._config.rules.obstacle.warning_distance_m == 25
    invalid = {**RULES, 'obstacle': {'warning_distance_m': 5, 'danger_distance_m': 10}}
    transport.deliver(config_message(clock, 2, invalid))
    app.step()
    failed = json.loads([x.payload for x in transport.published if x.topic.endswith('/config-acks')][-1])
    assert failed['status'] == 'failed'
    assert app._config.device.config_version == 1
    app.stop()
    restored = EdgeApp.from_config(config, clock, FakeMqtt())
    restored.start()
    assert restored._config.device.config_version == 1
    assert restored._config.rules.obstacle.warning_distance_m == 25
    restored.stop()


@pytest.mark.parametrize('bad', [None, {}, {**RULES,'temperature': {'trigger_c':30,'recover_c':35,'duration_ms':1000}}, {**RULES,'lane_departure': {'offset_threshold':float('nan'),'duration_ms':1000}}, {**RULES,'sharp_turn': {'yaw_threshold_deg_s':30,'min_speed_kmh':20,'duration_ms':True}}])
def test_config_rejects_invalid_complete_rules(example_config, bad):
    with pytest.raises(ValueError): merge_rules(example_config.rules, bad)


class HttpStub:
    def __init__(self):
        self.client = self
        self.uploads = []
    def submit(self, fn, *args):
        future = Future()
        try: future.set_result(fn(*args))
        except Exception as exc: future.set_exception(exc)
        return future
    def upload(self, jpeg, frame_id, at, kind, deadline):
        assert jpeg[:2] == b'\xff\xd8'
        self.uploads.append((kind, frame_id, jpeg))
        return 'snap-test'
    def confirm(self, events): return {x['event_id'] for x in events}
    def close(self): pass


def test_snapshot_uses_current_pixels_and_duplicate_does_not_upload_again(example_config, tmp_path):
    config = replace(configured(example_config, tmp_path), features=replace(example_config.features, frame_upload=True))
    clock, transport = FakeClock(), FakeMqtt()
    app = EdgeApp.from_config(replace(config, features=example_config.features), clock, transport)
    # Inject the worker without performing network IO.
    worker = HttpStub()
    app._http = worker
    app._config = config
    app._commands._snapshot_handler = app._begin_snapshot
    app.start()
    clock.advance_ns(500_000_000)
    app.step()
    message = _message(clock, 'snapshot-once', 'camera.snapshot', {'quality':80})
    transport.deliver(message)
    app.step()
    ack = json.loads([x.payload for x in transport.published if x.topic.endswith('/command-acks')][-1])
    assert ack['status'] == 'success' and ack['result'] == {'snapshot_id':'snap-test'}
    transport.deliver(message)
    app.step()
    assert sum(kind == 'snapshot' for kind, *_ in worker.uploads) == 1
    app.stop()


def test_confirmation_only_removes_replay_need_after_http_result(example_config):
    app, clock, transport = _app(example_config)
    app._http = HttpStub()
    event = make_event(device_id='car-001', event_id='confirm-me', event_type='sensor_failure', action='started', level='warning', timestamp=clock.utc(), data={'sensor':'gyro','reason':'error'})
    app._store.append(event)
    app.start()
    app.step()
    assert app._store.counts() == (1, 1)
    app.step()
    assert app._store.counts() == (1, 0)
    app.stop()


def test_sigterm_requests_graceful_stop(monkeypatch):
    import signal
    from piguard_edge import __main__ as cli
    callbacks = {}
    app = type('App', (), {})()
    def run(stop):
        callbacks[signal.SIGTERM](signal.SIGTERM, None)
        assert stop.is_set()
    app.run = run
    monkeypatch.setattr(cli.signal, 'signal', lambda sig, fn: callbacks.update({sig:fn}))
    monkeypatch.setattr(cli, 'load_config', lambda path: object())
    monkeypatch.setattr(cli, 'PahoMqtt', lambda config: object())
    monkeypatch.setattr(cli.EdgeApp, 'from_config', lambda *args: app)
    cli.main(['--config','unused.yaml'])
    assert signal.SIGINT in callbacks


def test_config_rejects_numbers_outside_backend_range(example_config):
    for values in (
        {**RULES, 'obstacle': {'warning_distance_m': 10**400, 'danger_distance_m': 10}},
        {**RULES, 'lane_departure': {'offset_threshold': .3, 'duration_ms': 2**63}},
    ):
        with pytest.raises(ValueError):
            merge_rules(example_config.rules, values)


def test_pending_http_upload_does_not_block_sampling_or_telemetry(example_config):
    app, clock, transport = _app(example_config)
    worker = HttpStub()
    pending = Future()
    worker.submit = lambda *args: pending
    app._http = worker
    app._commands._snapshot_handler = app._begin_snapshot
    app.start()
    clock.advance_ns(500_000_000)
    app.step()
    transport.deliver(_message(clock, 'slow-upload', 'camera.snapshot', {'quality':80}))
    app.step()
    assert app._snapshot is not None
    clock.advance_ns(500_000_000)
    app.step()
    assert app._sampler.state.speed.monotonic_ns == 1_000_000_000
    assert len([x for x in transport.published if x.topic.endswith('/telemetry')]) == 2
    app.stop()
    # Graceful shutdown records cancellation rather than reporting upload success.
    acks = [json.loads(x.payload) for x in transport.published if x.topic.endswith('/command-acks')]
    assert acks[-1]['error']['code'] == 'CANCELLED'


def test_snapshot_without_camera_returns_explicit_failure(example_config):
    app, clock, transport = _app(example_config)
    app._http = HttpStub()
    app._commands._snapshot_handler = app._begin_snapshot
    app.start()
    transport.deliver(_message(clock, 'no-camera', 'camera.snapshot', {'quality':80}))
    app.step()
    ack = json.loads([x.payload for x in transport.published if x.topic.endswith('/command-acks')][-1])
    assert ack['error']['code'] == 'CAMERA_UNAVAILABLE'
    app.stop()


def test_failed_confirmation_keeps_cached_events(example_config):
    app, clock, transport = _app(example_config)
    worker = HttpStub()
    def unavailable(events):
        raise OSError('platform unreachable')
    worker.confirm = unavailable
    app._http = worker
    app._store.append(make_event(device_id='car-001', event_id='keep-me', event_type='sensor_failure', action='started', level='warning', timestamp=clock.utc(), data={'sensor':'gyro','reason':'error'}))
    app.start()
    app.step()
    app.step()
    assert app._store.counts() == (1, 1)
    assert app.network_error == 'platform HTTP unavailable'
    app.stop()
