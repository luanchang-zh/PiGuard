import json

import pytest

from piguard_edge import __version__
from piguard_edge.models.sample import UNIT_M, Sample, SampleStatus
from piguard_edge.protocol.telemetry import build_telemetry
from piguard_edge.protocol.topics import commands_topic, status_topic, telemetry_topic
from piguard_edge.runtime.app import EdgeApp
from piguard_edge.runtime.clock import FakeClock
from piguard_edge.runtime.state import LatestState
from piguard_edge.runtime.workers import Sampler
from piguard_edge.simulation.engine import ScenarioEngine
from piguard_edge.storage.commands import CommandStore
from piguard_edge.storage.events import EventStore
from piguard_edge.transport.mqtt import FakeMqtt, PahoMqtt

def _ids():
    counter = {"n": 0}

    def next_id() -> str:
        counter["n"] += 1
        return f"msg-{counter['n']}"

    return next_id


def _app(example_config, scenario: str = "normal_drive"):
    clock = FakeClock()
    config = example_config
    engine = ScenarioEngine(scenario, 1.0)
    sampler = Sampler.from_config(config, engine, clock)
    transport = FakeMqtt()
    app = EdgeApp(
        config,
        clock,
        transport,
        sampler,
        _ids(),
        store=EventStore(":memory:"),
        engine=engine,
        command_store=CommandStore(":memory:"),
    )
    return app, clock, transport


def _body(message) -> dict:
    return json.loads(message.payload.decode("utf-8"))


def _telemetry(transport: FakeMqtt) -> list[dict]:
    return [_body(item) for item in transport.published if item.topic.endswith("/telemetry")]


def _status(transport: FakeMqtt) -> list[dict]:
    return [_body(item) for item in transport.published if item.topic.endswith("/status")]


def test_startup_sets_will_without_timestamp_and_publishes_online(example_config) -> None:
    app, _, transport = _app(example_config)
    app.start()

    assert transport.will is not None
    will = _body(transport.will)
    assert "timestamp" not in will
    assert will == {
        "schema_version": 1,
        "device_id": "car-001",
        "online": False,
        "reason": "connection_lost",
    }
    assert transport.will.qos == 1
    assert transport.will.retain is True
    assert transport.subscriptions == [(commands_topic("car-001"), 1)]
    online = _status(transport)
    assert online[0]["online"] is True
    assert online[0]["software_version"] == __version__
    assert online[0]["config_version"] == 0
    assert online[0]["mode"] == "mock"
    assert online[0]["mqtt"] == {"connected": True}
    assert online[0]["sensors"] == {
        "distance": "unavailable",
        "camera": "unavailable",
        "gyro": "unavailable",
        "temperature": "unavailable",
    }
    assert _telemetry(transport) == []


def test_two_hertz_telemetry_uses_latest_sample_and_keeps_sample_at(example_config) -> None:
    app, clock, transport = _app(example_config)
    app.start()
    for _ in range(10):
        clock.advance_ns(100_000_000)
        app.step()

    messages = _telemetry(transport)
    assert [item.qos for item in transport.published if item.topic.endswith("/telemetry")] == [0, 0]
    assert [item.retain for item in transport.published if item.topic.endswith("/telemetry")] == [False, False]
    assert [item["seq"] for item in messages] == [0, 1]
    assert messages[0]["timestamp"] != messages[1]["timestamp"]
    assert messages[1]["speed"]["value"] == 30
    assert messages[1]["distance"]["raw_value"] == 0.5
    assert messages[1]["distance"]["mapped_value"] == 50
    assert messages[0]["risk"] == {"level": "unknown", "active_events": []}
    assert messages[1]["risk"] == {"level": "normal", "active_events": []}
    assert messages[0]["message_id"] == "msg-1"


def test_republish_does_not_refresh_an_old_sample_at(example_config) -> None:
    clock = FakeClock()
    state = LatestState()
    state.speed = Sample(30.0, clock.utc(), 0, "mock", SampleStatus.OK, "km/h")
    later = FakeClock(2_000_000_000)
    first = build_telemetry(
        state,
        device_id="car-001",
        timestamp=later.utc(),
        seq=0,
        message_id="msg-a",
        distance_scale=100,
        source="mock",
    )
    second = build_telemetry(
        state,
        device_id="car-001",
        timestamp=later.utc(),
        seq=1,
        message_id="msg-b",
        distance_scale=100,
        source="mock",
    )

    assert first["speed"]["sample_at"] == "2026-01-01T00:00:00.000Z"
    assert second["speed"]["sample_at"] == first["speed"]["sample_at"]
    assert first["timestamp"] == "2026-01-01T00:00:02.000Z"
    age_ms = example_config.runtime.ttl_ms.speed
    assert age_ms == 1000
    assert first["speed"]["status"] == "ok"


def test_fault_object_keeps_null_values(example_config) -> None:
    app, clock, transport = _app(example_config, "sensor_failure")
    app.start()
    for _ in range(5):
        clock.advance_ns(100_000_000)
        app.step()

    distance = _telemetry(transport)[-1]["distance"]
    assert set(distance) == {"raw_value", "mapped_value", "unit", "scale", "source", "status", "sample_at"}
    assert distance["status"] == "timeout"
    assert distance["raw_value"] is None
    assert distance["mapped_value"] is None
    assert distance["scale"] == 100
    assert _telemetry(transport)[-1]["speed"]["value"] == 30


def test_timeout_zero_is_encoded_as_null() -> None:
    clock = FakeClock()
    state = LatestState()
    state.distance = Sample(0.0, clock.utc(), 0, "mock", SampleStatus.TIMEOUT, UNIT_M)
    payload = build_telemetry(
        state,
        device_id="car-001",
        timestamp=clock.utc(),
        seq=0,
        message_id="msg-fault",
        distance_scale=100,
        source="mock",
    )
    assert payload["distance"]["raw_value"] is None
    assert payload["distance"]["status"] == "timeout"


def test_reconnect_keeps_seq_and_resubscribes(example_config) -> None:
    app, clock, transport = _app(example_config)
    app.start()
    clock.advance_ns(500_000_000)
    app.step()
    transport.lose_connection()
    app.step()
    assert _telemetry(transport)[-1]["seq"] == 0
    transport.restore_connection()
    app.step()
    clock.advance_ns(500_000_000)
    app.step()

    assert [item["seq"] for item in _telemetry(transport)] == [0, 1]
    assert transport.subscriptions.count((commands_topic("car-001"), 1)) == 2
    assert _status(transport)[-1]["online"] is True


def test_new_process_restarts_seq(example_config) -> None:
    first, clock, transport = _app(example_config)
    first.start()
    clock.advance_ns(500_000_000)
    first.step()
    second, clock2, transport2 = _app(example_config)
    second.start()
    clock2.advance_ns(500_000_000)
    second.step()

    assert _telemetry(transport)[0]["seq"] == 0
    assert _telemetry(transport2)[0]["seq"] == 0


def test_broker_outage_still_samples(example_config) -> None:
    app, clock, transport = _app(example_config)
    transport.fail_connect = True
    app.start()
    clock.advance_ns(100_000_000)
    app.step()

    assert transport.published == []
    assert app._sampler.state.speed is not None
    assert app._sampler.state.speed.value == 30
    assert transport.will is not None


def test_clean_shutdown_publishes_offline(example_config) -> None:
    app, _, transport = _app(example_config)
    app.start()
    app.stop()

    offline = _status(transport)[-1]
    assert offline["online"] is False
    assert offline["reason"] == "shutdown"
    assert "timestamp" in offline
    assert offline["mqtt"] == {"connected": False}
    assert transport.published[-1].topic == status_topic("car-001")
    assert transport.published[-1].retain is True


def test_status_heartbeat_while_sensors_stay_unchanged(example_config) -> None:
    app, clock, transport = _app(example_config)
    app.start()
    for name in app._sampler._next_ns:
        app._sampler._next_ns[name] = 10**18
    for _ in range(100):
        clock.advance_ns(100_000_000)
        app.step()

    assert len(_status(transport)) == 2
    assert _status(transport)[0]["sensors"] == _status(transport)[1]["sensors"]


def test_run_loop_stops_cleanly(example_config) -> None:
    app, _, transport = _app(example_config)
    calls = {"n": 0}

    def sleep(_seconds: float) -> None:
        calls["n"] += 1
        raise _Stop()

    with pytest.raises(_Stop):
        app.run(sleep=sleep)
    app.stop()
    assert calls["n"] == 1
    assert _status(transport)[-1]["reason"] == "shutdown"


def test_paho_wrapper_sets_will_before_connect(example_config) -> None:
    client = _StubClient()
    transport = PahoMqtt(example_config, client=client)
    message = app_will()
    transport.set_will(message)
    transport.connect()
    client.on_connect(client, None, None, _Success(), None)

    assert client.will[0] == status_topic("car-001")
    assert client.connect_args == ("127.0.0.1", 1883, 30)
    assert transport.poll() == ["up"]
    assert client.started is True


def test_cli_requires_a_config() -> None:
    from piguard_edge.__main__ import main

    with pytest.raises(SystemExit):
        main([])


def app_will():
    from piguard_edge.transport.mqtt import MqttMessage

    return MqttMessage(status_topic("car-001"), b"{}", 1, True)


class _Stop(Exception):
    pass


class _Success:
    is_failure = False


class _StubClient:
    def __init__(self) -> None:
        self.on_connect = None
        self.on_disconnect = None
        self.will = None
        self.started = False
        self.connect_args = None

    def will_set(self, topic, payload, qos, retain) -> None:
        self.will = (topic, payload, qos, retain)

    def connect(self, host, port, keepalive) -> None:
        self.connect_args = (host, port, keepalive)

    def loop_start(self) -> None:
        self.started = True

    def publish(self, topic, payload, qos, retain) -> None:
        return None

    def subscribe(self, topic, qos) -> None:
        return None

    def disconnect(self) -> None:
        return None

    def loop_stop(self) -> None:
        return None
