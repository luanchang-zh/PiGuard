import json
from datetime import datetime, timezone

from piguard_edge.protocol.event import make_event
from piguard_edge.protocol.topics import events_topic
from piguard_edge.runtime.app import EdgeApp
from piguard_edge.runtime.clock import FakeClock
from piguard_edge.runtime.workers import Sampler
from piguard_edge.simulation.engine import ScenarioEngine
from piguard_edge.storage.events import EventStore
from piguard_edge.transport.mqtt import FakeMqtt


def _app(example_config, store: EventStore, scenario: str, connect: bool = True):
    clock = FakeClock()
    transport = FakeMqtt()
    transport.fail_connect = not connect
    sampler = Sampler.from_config(example_config, ScenarioEngine(scenario, 1.0), clock)
    app = EdgeApp(example_config, clock, transport, sampler, store=store)
    app.start()
    return app, clock, transport


def _advance(app: EdgeApp, clock: FakeClock, milliseconds: int) -> None:
    for _ in range(milliseconds // 100):
        clock.advance_ns(100_000_000)
        app.step()


def _events(transport: FakeMqtt) -> list[dict]:
    return [json.loads(item.payload.decode("utf-8")) for item in transport.published if item.topic.endswith("/events")]


def _event_messages(transport: FakeMqtt):
    return [item for item in transport.published if item.topic.endswith("/events")]


def test_store_keeps_one_row_and_purges_only_confirmed(tmp_path) -> None:
    store = EventStore(tmp_path / "edge.db")
    old = _stored("evt-old", datetime(2020, 1, 1, tzinfo=timezone.utc))
    waiting = _stored("evt-waiting", datetime(2020, 1, 1, tzinfo=timezone.utc))
    recent = _stored("evt-recent", datetime(2026, 1, 1, tzinfo=timezone.utc))
    store.append(old)
    store.append(waiting)
    store.append(recent)
    store.append(old)

    with_other_data = _stored("evt-old", datetime(2020, 1, 1, tzinfo=timezone.utc), distance_m=1)
    try:
        store.append(with_other_data)
    except ValueError as exc:
        assert "evt-old" in str(exc)
    else:
        raise AssertionError("a changed payload reused an event_id")

    store.mark_platform_confirmed("evt-old")
    store.mark_platform_confirmed("evt-recent")
    removed = store.purge_confirmed_before("2025-01-01T00:00:00.000Z")

    assert removed == 1
    assert store.counts() == (2, 1)
    assert [item.event_id for item in store.unconfirmed()] == ["evt-waiting"]
    store.close()


def test_startup_purges_old_confirmed_events_only(example_config, tmp_path) -> None:
    store = EventStore(tmp_path / "edge.db")
    store.append(_stored("evt-old", datetime(2020, 1, 1, tzinfo=timezone.utc)))
    store.append(_stored("evt-open", datetime(2020, 1, 1, tzinfo=timezone.utc)))
    store.mark_platform_confirmed("evt-old")
    app, _, _ = _app(example_config, store, "normal_drive")

    assert store.counts() == (1, 1)
    assert store.unconfirmed()[0].event_id == "evt-open"
    app.stop()


def test_obstacle_scenario_changes_output_only_when_the_level_changes(example_config) -> None:
    app, clock, transport = _app(example_config, EventStore(":memory:"), "obstacle_approach")
    _advance(app, clock, 6000)

    assert [(item["action"], item["level"], item["data"]["distance_m"]) for item in _events(transport)] == [
        ("started", "warning", 15.0),
        ("level_changed", "danger", 5.0),
    ]
    assert [item.topic for item in _event_messages(transport)] == [events_topic("car-001"), events_topic("car-001")]
    assert [(item.qos, item.retain) for item in _event_messages(transport)] == [(1, False), (1, False)]
    assert app._actuators.actuator.indicator == "red"
    assert app._actuators.actuator.buzzer == "fast"
    assert app._risk_level == "danger"
    assert app._active_events == ["obstacle_warning"]
    app.stop()


def test_sensor_failure_scenario_recovers_each_sensor(example_config) -> None:
    app, clock, transport = _app(example_config, EventStore(":memory:"), "sensor_failure")
    _advance(app, clock, 2100)

    assert [(item["action"], item["data"]) for item in _events(transport)] == [
        ("started", {"sensor": "distance", "reason": "timeout"}),
        ("recovered", {"sensor": "distance", "reason": "ok"}),
        ("started", {"sensor": "camera", "reason": "unavailable"}),
        ("recovered", {"sensor": "camera", "reason": "ok"}),
        ("started", {"sensor": "gyro", "reason": "error"}),
    ]
    assert app._active_events == ["sensor_failure"]
    assert app._actuators.actuator.indicator == "yellow"
    assert app._actuators.actuator.buzzer == "intermittent"
    app.stop()


def test_offline_rules_keep_running_and_reconnect_repeats_the_same_payload(example_config) -> None:
    store = EventStore(":memory:")
    app, clock, transport = _app(example_config, store, "obstacle_approach", connect=False)
    _advance(app, clock, 6000)

    assert transport.published == []
    assert app._actuators.actuator.indicator == "red"
    assert store.counts() == (2, 2)

    transport.fail_connect = False
    transport.connect()
    app.step()
    first = [item.payload for item in _event_messages(transport)]
    assert first == [item.payload for item in store.unconfirmed()]
    assert store.counts() == (2, 2)

    transport.lose_connection()
    app.step()
    transport.restore_connection()
    app.step()
    second = [item.payload for item in _event_messages(transport)]

    assert second == first + first
    assert store.counts() == (2, 2)
    app.stop()


def _stored(event_id: str, timestamp: datetime, distance_m: float = 10.0):
    return make_event(
        device_id="car-001",
        event_id=event_id,
        event_type="obstacle_warning",
        action="started",
        level="warning",
        timestamp=timestamp,
        data={"distance_m": distance_m, "speed_kmh": None, "headway_s": None},
    )
