import json
import sqlite3

from piguard_edge.models.frame import Frame, LaneResult, invalid_lane, lane_direction
from piguard_edge.models.sample import (
    UNIT_C,
    UNIT_DPS,
    UNIT_KMH,
    UNIT_M,
    DistanceReading,
    GyroReading,
    Sample,
    SampleStatus,
)
from piguard_edge.rules.engine import RuleEngine
from piguard_edge.runtime.clock import FakeClock
from piguard_edge.runtime.state import LatestState


def _ids():
    counter = {"n": 0}

    def next_id() -> str:
        counter["n"] += 1
        return f"evt-{counter['n']}"

    return next_id


def _engine(example_config) -> RuleEngine:
    return RuleEngine.from_config(example_config, event_ids=_ids())


def _fresh(clock: FakeClock) -> LatestState:
    now = clock.utc()
    ns = clock.monotonic_ns()
    state = LatestState()
    state.speed = Sample(30.0, now, ns, "mock", SampleStatus.OK, UNIT_KMH)
    state.distance = Sample(DistanceReading(0.5, 50.0, 100.0), now, ns, "mock", SampleStatus.OK, UNIT_M)
    state.temperature = Sample(25, now, ns, "mock", SampleStatus.OK, UNIT_C)
    state.gyro = Sample(GyroReading(0.0, 0.0, 2.0, 2.0), now, ns, "mock", SampleStatus.OK, UNIT_DPS)
    state.frame = Frame(now, ns, "mock", SampleStatus.OK, None, LaneResult(True, 0.0, "center", 1.0, now))
    return state


def _eval(engine: RuleEngine, clock: FakeClock, edit=None):
    state = _fresh(clock)
    if edit is not None:
        edit(state, clock)
    return engine.evaluate(state, clock.monotonic_ns(), clock.utc())


def _kinds(update) -> list[tuple[str, str, str]]:
    return [(event.event_type, event.action, event.level) for event in update.events]


def _distance(state, clock, mapped: float, speed: float | None = 30.0, speed_status: SampleStatus = SampleStatus.OK) -> None:
    now = clock.utc()
    ns = clock.monotonic_ns()
    state.distance = Sample(DistanceReading(mapped / 100, mapped, 100.0), now, ns, "mock", SampleStatus.OK, UNIT_M)
    if speed is None:
        state.speed = Sample(None, now, ns, "mock", speed_status, UNIT_KMH)
    else:
        state.speed = Sample(speed, now, ns, "mock", speed_status, UNIT_KMH)


def _lane(state, clock, offset: float) -> None:
    now = clock.utc()
    ns = clock.monotonic_ns()
    state.frame = Frame(
        now,
        ns,
        "mock",
        SampleStatus.OK,
        None,
        LaneResult(True, offset, lane_direction(offset, True), 1.0, now),
    )


def _yaw(state, clock, yaw: float, speed: float) -> None:
    now = clock.utc()
    ns = clock.monotonic_ns()
    state.gyro = Sample(GyroReading(0.0, 0.0, yaw, yaw), now, ns, "mock", SampleStatus.OK, UNIT_DPS)
    state.speed = Sample(speed, now, ns, "mock", SampleStatus.OK, UNIT_KMH)


def _temp(state, clock, value: float) -> None:
    now = clock.utc()
    ns = clock.monotonic_ns()
    state.temperature = Sample(value, now, ns, "mock", SampleStatus.OK, UNIT_C)


def test_startup_without_samples_is_unknown(example_config) -> None:
    update = _engine(example_config).evaluate(LatestState(), 0, FakeClock().utc())

    assert update.level == "unknown"
    assert update.active_events == ()
    assert update.events == ()


def test_mapped_distance_is_not_scaled_again(example_config) -> None:
    clock = FakeClock()
    update = _eval(_engine(example_config), clock, lambda state, now: _distance(state, now, 20))

    assert update.level == "normal"
    assert update.events == ()


def test_obstacle_levels_use_distance_and_headway(example_config) -> None:
    engine = _engine(example_config)
    clock = FakeClock()

    warning = _eval(engine, clock, lambda state, now: _distance(state, now, 15))
    danger = _eval(engine, clock, lambda state, now: _distance(state, now, 5))
    eased = _eval(engine, clock, lambda state, now: _distance(state, now, 10, speed=0))
    cleared = _eval(engine, clock, lambda state, now: _distance(state, now, 50))

    assert _kinds(warning) == [("obstacle_warning", "started", "warning")]
    assert warning.events[0].data["distance_m"] == 15
    assert _kinds(danger) == [("obstacle_warning", "level_changed", "danger")]
    assert _kinds(eased) == [("obstacle_warning", "level_changed", "warning")]
    assert eased.events[0].data["headway_s"] is None
    assert _kinds(cleared) == [("obstacle_warning", "recovered", "warning")]


def test_headway_can_raise_the_level_and_zero_speed_does_not_divide(example_config) -> None:
    engine = _engine(example_config)
    clock = FakeClock()

    stopped = _eval(engine, clock, lambda state, now: _distance(state, now, 5, speed=0))
    engine = _engine(example_config)
    safe_headway = _eval(engine, clock, lambda state, now: _distance(state, now, 20, speed=48))
    fast = _eval(engine, clock, lambda state, now: _distance(state, now, 20, speed=108))

    assert stopped.level == "danger"
    assert stopped.events[0].data["headway_s"] is None
    assert safe_headway.events == ()
    assert safe_headway.level == "normal"
    assert _kinds(fast) == [("obstacle_warning", "started", "danger")]


def test_invalid_speed_still_uses_distance_and_does_not_emit_sensor_failure(example_config) -> None:
    clock = FakeClock()
    update = _eval(
        _engine(example_config),
        clock,
        lambda state, now: _distance(state, now, 10, speed=None, speed_status=SampleStatus.ERROR),
    )

    assert _kinds(update) == [("obstacle_warning", "started", "warning")]
    assert update.events[0].data["speed_kmh"] is None
    assert update.events[0].data["headway_s"] is None
    assert update.active_events == ("obstacle_warning",)


def test_distance_fault_does_not_clear_an_active_obstacle(example_config) -> None:
    engine = _engine(example_config)
    clock = FakeClock()
    _eval(engine, clock, lambda state, now: _distance(state, now, 5))

    def fail(state, now) -> None:
        state.distance = Sample(None, now.utc(), now.monotonic_ns(), "mock", SampleStatus.TIMEOUT, UNIT_M)

    update = _eval(engine, clock, fail)

    assert _kinds(update) == [("sensor_failure", "started", "warning")]
    assert update.events[0].data == {"sensor": "distance", "reason": "timeout"}
    assert update.level == "danger"
    assert update.active_events == ("obstacle_warning", "sensor_failure")


def test_lane_requires_one_second_and_keeps_the_sign(example_config) -> None:
    engine = _engine(example_config)
    clock = FakeClock()

    assert _eval(engine, clock, lambda state, now: _lane(state, now, 0.35)).events == ()
    clock.advance_ns(2_000_000_000)
    assert _eval(engine, clock, lambda state, now: _lane(state, now, 0.35)).events == ()

    engine = _engine(example_config)
    clock = FakeClock()
    assert _eval(engine, clock, lambda state, now: _lane(state, now, -0.5)).events == ()
    clock.advance_ns(1_000_000_000)
    update = _eval(engine, clock, lambda state, now: _lane(state, now, -0.5))

    assert _kinds(update) == [("lane_departure", "started", "warning")]
    assert update.events[0].data == {"offset_ratio": -0.5, "direction": "left"}
    assert update.level == "warning"


def test_stale_lane_interrupts_pending_without_a_departure_event(example_config) -> None:
    engine = _engine(example_config)
    clock = FakeClock()
    state = _fresh(clock)
    _lane(state, clock, 0.5)
    frame = state.frame
    assert engine.evaluate(state, clock.monotonic_ns(), clock.utc()).events == ()

    clock.advance_ns(1_000_000_001)
    later = _fresh(clock)
    later.frame = frame
    update = engine.evaluate(later, clock.monotonic_ns(), clock.utc())

    assert _kinds(update) == [("sensor_failure", "started", "warning")]
    assert update.events[0].data == {"sensor": "camera", "reason": "stale"}
    assert "lane_departure" not in update.active_events


def test_invalid_lane_is_not_normal_and_does_not_recover(example_config) -> None:
    engine = _engine(example_config)
    clock = FakeClock()
    _eval(engine, clock, lambda state, now: _lane(state, now, 0.5))
    clock.advance_ns(1_000_000_000)
    started = _eval(engine, clock, lambda state, now: _lane(state, now, 0.5))
    assert _kinds(started) == [("lane_departure", "started", "warning")]

    def hide(state, now) -> None:
        state.frame = Frame(now.utc(), now.monotonic_ns(), "mock", SampleStatus.OK, None, invalid_lane(now.utc()))

    held = _eval(engine, clock, hide)
    cleared = _eval(engine, clock, lambda state, now: _lane(state, now, 0))

    assert held.events == ()
    assert held.level == "warning"
    assert _kinds(cleared) == [("lane_departure", "recovered", "warning")]


def test_sharp_turn_duration_low_speed_and_jitter(example_config) -> None:
    engine = _engine(example_config)
    clock = FakeClock()
    assert _eval(engine, clock, lambda state, now: _yaw(state, now, 30, 30)).events == ()
    clock.advance_ns(1_000_000_000)
    assert _eval(engine, clock, lambda state, now: _yaw(state, now, 55, 20)).events == ()

    engine = _engine(example_config)
    clock = FakeClock()
    assert _eval(engine, clock, lambda state, now: _yaw(state, now, 40, 30)).events == ()
    clock.advance_ns(200_000_000)
    assert _eval(engine, clock, lambda state, now: _yaw(state, now, 10, 30)).events == ()
    assert _eval(engine, clock, lambda state, now: _yaw(state, now, 40, 30)).events == ()
    clock.advance_ns(200_000_000)
    assert _eval(engine, clock, lambda state, now: _yaw(state, now, 40, 30)).events == ()
    clock.advance_ns(100_000_000)
    update = _eval(engine, clock, lambda state, now: _yaw(state, now, 40, 30))

    assert _kinds(update) == [("sharp_turn", "started", "warning")]
    assert update.events[0].data == {"yaw_rate_deg_s": 40.0, "speed_kmh": 30.0}


def test_temperature_hysteresis_and_integer_sample(example_config) -> None:
    engine = _engine(example_config)
    clock = FakeClock()
    assert _eval(engine, clock, lambda state, now: _temp(state, now, 36)).events == ()
    clock.advance_ns(4_000_000_000)
    assert _eval(engine, clock, lambda state, now: _temp(state, now, 34.5)).events == ()
    assert _eval(engine, clock, lambda state, now: _temp(state, now, 36)).events == ()
    clock.advance_ns(4_000_000_000)
    assert _eval(engine, clock, lambda state, now: _temp(state, now, 36)).events == ()
    clock.advance_ns(1_000_000_000)
    started = _eval(engine, clock, lambda state, now: _temp(state, now, 36))
    assert _kinds(started) == [("high_temperature", "started", "warning")]

    clock.advance_ns(6_000_000_000)
    assert _eval(engine, clock, lambda state, now: _temp(state, now, 34)).events == ()
    clock.advance_ns(100_000_000)
    assert _eval(engine, clock, lambda state, now: _temp(state, now, 33)).events == ()
    clock.advance_ns(4_900_000_000)
    assert _eval(engine, clock, lambda state, now: _temp(state, now, 33)).events == ()
    clock.advance_ns(100_000_000)
    recovered = _eval(engine, clock, lambda state, now: _temp(state, now, 33))

    assert _kinds(recovered) == [("high_temperature", "recovered", "warning")]
    assert recovered.level == "normal"


def test_sensor_failure_is_emitted_once_per_episode(example_config) -> None:
    engine = _engine(example_config)
    clock = FakeClock()

    def fail(state, now) -> None:
        state.distance = Sample(None, now.utc(), now.monotonic_ns(), "mock", SampleStatus.TIMEOUT, UNIT_M)

    first = _eval(engine, clock, fail)
    second = _eval(engine, clock, fail)
    recovered = _eval(engine, clock)
    again = _eval(engine, clock, fail)

    assert _kinds(first) == [("sensor_failure", "started", "warning")]
    assert second.events == ()
    assert _kinds(recovered) == [("sensor_failure", "recovered", "warning")]
    assert recovered.events[0].data == {"sensor": "distance", "reason": "ok"}
    assert again.events[0].event_id != first.events[0].event_id


def test_highest_active_alarm_sets_the_risk(example_config) -> None:
    engine = _engine(example_config)
    clock = FakeClock()
    _eval(engine, clock, lambda state, now: _lane(state, now, 0.5))
    clock.advance_ns(1_000_000_000)
    _eval(engine, clock, lambda state, now: _lane(state, now, 0.5))

    def both(state, now) -> None:
        _lane(state, now, 0.5)
        _distance(state, now, 5)

    update = _eval(engine, clock, both)

    assert update.level == "danger"
    assert update.active_events == ("obstacle_warning", "lane_departure")
    assert _kinds(update) == [("obstacle_warning", "started", "danger")]


def test_event_payload_repeats_without_changing_bytes() -> None:
    from datetime import datetime, timezone

    from piguard_edge.protocol.event import make_event

    timestamp = datetime(2026, 1, 1, tzinfo=timezone.utc)
    data = {"distance_m": 6.8, "speed_kmh": 32.0, "headway_s": 0.76}
    first = make_event(
        device_id="car-001",
        event_id="evt-1",
        event_type="obstacle_warning",
        action="started",
        level="danger",
        timestamp=timestamp,
        data=data,
    )
    second = make_event(
        device_id="car-001",
        event_id="evt-1",
        event_type="obstacle_warning",
        action="started",
        level="danger",
        timestamp=timestamp,
        data=dict(data),
    )

    assert first.payload == second.payload
    assert json.loads(first.payload.decode("utf-8"))["data"]["headway_s"] == 0.76
    assert b"snapshot_id" not in first.payload


def test_disk_full_is_retried_instead_of_dropped(example_config) -> None:
    from piguard_edge.runtime.app import EdgeApp
    from piguard_edge.runtime.workers import Sampler
    from piguard_edge.simulation.engine import ScenarioEngine
    from piguard_edge.storage.commands import CommandStore
    from piguard_edge.storage.events import EventStore
    from piguard_edge.transport.mqtt import FakeMqtt

    clock = FakeClock()
    transport = FakeMqtt()
    sampler = Sampler.from_config(example_config, ScenarioEngine("normal_drive", 1.0), clock)
    store = EventStore(":memory:")
    app = EdgeApp(example_config, clock, transport, sampler, store=store, command_store=CommandStore(":memory:"))
    app.start()
    clock.advance_ns(100_000_000)
    for name in app._sampler._next_ns:
        app._sampler._next_ns[name] = 10**18
    now = clock.utc()
    ns = clock.monotonic_ns()
    app._sampler.state.distance = Sample(DistanceReading(0.05, 5.0, 100.0), now, ns, "mock", SampleStatus.OK, UNIT_M)
    app._sampler.state.speed = Sample(0.0, now, ns, "mock", SampleStatus.OK, UNIT_KMH)
    app._sampler.state.gyro = Sample(GyroReading(0.0, 0.0, 2.0, 2.0), now, ns, "mock", SampleStatus.OK, UNIT_DPS)
    app._sampler.state.temperature = Sample(25.0, now, ns, "mock", SampleStatus.OK, UNIT_C)
    app._sampler.state.frame = Frame(now, ns, "mock", SampleStatus.OK, None, LaneResult(True, 0.0, "center", 1.0, now))
    real_append = store.append

    def fail_once(event) -> None:
        store.append = real_append
        raise sqlite3.OperationalError("disk full")

    store.append = fail_once
    app.step()

    assert app.storage_error == "disk full"
    assert _published_events(transport) == []
    assert store.counts() == (0, 0)

    app.step()

    published = _published_events(transport)
    assert app.storage_error is None
    assert store.counts() == (1, 1)
    assert len(published) == 1
    assert published[0]["level"] == "danger"
    assert published[0]["data"]["headway_s"] is None


def _published_events(transport) -> list[dict]:
    return [json.loads(item.payload.decode("utf-8")) for item in transport.published if item.topic.endswith("/events")]
