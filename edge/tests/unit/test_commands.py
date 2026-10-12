import json
import sqlite3
from datetime import timedelta

from piguard_edge.actuators.manager import ActuatorManager
from piguard_edge.actuators.mock import MockActuator
from piguard_edge.commands.handler import CommandHandler
from piguard_edge.models.sample import format_utc
from piguard_edge.protocol.command import DeviceCommand, parse_command
from piguard_edge.protocol.topics import command_acks_topic, commands_topic
from piguard_edge.runtime.app import EdgeApp
from piguard_edge.runtime.clock import FakeClock
from piguard_edge.runtime.workers import Sampler
from piguard_edge.simulation.engine import ScenarioEngine
from piguard_edge.storage.commands import CommandStore
from piguard_edge.storage.events import EventStore
from piguard_edge.transport.mqtt import FakeMqtt, MqttMessage


def _app(example_config, scenario: str = "normal_drive", connect: bool = True, command_store: CommandStore | None = None):
    clock = FakeClock()
    transport = FakeMqtt()
    transport.fail_connect = not connect
    engine = ScenarioEngine(scenario, 1.0)
    sampler = Sampler.from_config(example_config, engine, clock)
    store = command_store if command_store is not None else CommandStore(":memory:")
    app = EdgeApp(
        example_config,
        clock,
        transport,
        sampler,
        store=EventStore(":memory:"),
        engine=engine,
        command_store=store,
    )
    return app, clock, transport, engine, store


def _advance(app: EdgeApp, clock: FakeClock, milliseconds: int) -> None:
    for _ in range(milliseconds // 100):
        clock.advance_ns(100_000_000)
        app.step()


def _message(clock: FakeClock, command_id: str, command_type: str, params: dict, *, expires_in_s: float = 10.0, issued_s_ago: float = 0.0, device_id: str = "car-001") -> MqttMessage:
    now = clock.utc()
    body = {
        "schema_version": 1,
        "command_id": command_id,
        "device_id": device_id,
        "type": command_type,
        "issued_at": format_utc(now - timedelta(seconds=issued_s_ago)),
        "expires_at": format_utc(now + timedelta(seconds=expires_in_s)),
        "params": params,
    }
    return MqttMessage(commands_topic(device_id), json.dumps(body).encode("utf-8"), 1, False)


def _acks(transport: FakeMqtt) -> list[MqttMessage]:
    return [item for item in transport.published if item.topic == command_acks_topic("car-001")]


def _ack(message: MqttMessage) -> dict:
    return json.loads(message.payload.decode("utf-8"))


def _telemetry(transport: FakeMqtt) -> list[dict]:
    return [json.loads(item.payload.decode("utf-8")) for item in transport.published if item.topic.endswith("/telemetry")]


def _distance_m(app: EdgeApp) -> float:
    reading = app._sampler.state.distance.value
    return reading.mapped_value


def _error(store: CommandStore, command_id: str) -> str:
    record = store.get(command_id)
    assert record is not None and record.ack is not None
    return json.loads(record.ack.decode("utf-8"))["error"]["code"]


def test_buzzer_test_runs_without_stopping_sampling(example_config) -> None:
    app, clock, transport, _, store = _app(example_config)
    app.start()
    _advance(app, clock, 1000)
    assert app._risk_level == "normal"

    transport.deliver(_message(clock, "cmd-buzz", "buzzer.test", {"duration_ms": 2000}))
    app.step()
    assert app._actuators.actuator.buzzer == "on"
    assert app._actuators.actuator.indicator == "green"
    assert app._actuators.manual is not None
    assert app._actuators.manual.command_id == "cmd-buzz"

    before = len(_telemetry(transport))
    transport.deliver(_message(clock, "cmd-other", "indicator.test", {"color": "red", "duration_ms": 200}))
    app.step()
    assert _error(store, "cmd-other") == "DEVICE_BUSY"
    assert json.loads(store.get("cmd-other").ack.decode("utf-8"))["error"]["message"] == "another test is still running"
    assert app._actuators.manual.command_id == "cmd-buzz"

    _advance(app, clock, 500)
    assert len(_telemetry(transport)) > before
    assert app._sampler.state.speed.value == 30
    assert app._actuators.actuator.buzzer == "on"

    _advance(app, clock, 1500)
    assert app._actuators.actuator.buzzer == "off"
    assert app._actuators.actuator.indicator == "green"
    assert app._actuators.manual is None
    finished = [item for item in _acks(transport) if _ack(item)["command_id"] == "cmd-buzz"]
    assert finished[-1].qos == 1
    assert finished[-1].retain is False
    assert _ack(finished[-1])["status"] == "success"
    assert _ack(finished[-1])["result"] == {"message": "buzzer test completed"}
    assert "error" not in _ack(finished[-1])
    app.stop()


def test_indicator_test_uses_the_requested_color(example_config) -> None:
    app, clock, transport, _, _ = _app(example_config)
    app.start()
    _advance(app, clock, 1000)
    transport.deliver(_message(clock, "cmd-lamp", "indicator.test", {"color": "yellow", "duration_ms": 200}))
    app.step()

    assert app._actuators.actuator.indicator == "yellow"
    assert app._actuators.actuator.buzzer == "off"
    _advance(app, clock, 200)

    body = _ack(_acks(transport)[-1])
    assert body["status"] == "success"
    assert body["result"] == {"color": "yellow", "message": "indicator test completed"}
    assert app._actuators.actuator.indicator == "green"
    assert app._actuators.actuator.buzzer == "off"
    app.stop()


def test_danger_refuses_a_manual_test(example_config) -> None:
    app, clock, transport, _, store = _app(example_config, "obstacle_approach")
    app.start()
    _advance(app, clock, 6000)
    assert app._risk_level == "danger"

    transport.deliver(_message(clock, "cmd-busy", "buzzer.test", {"duration_ms": 200}))
    app.step()

    assert app._actuators.manual is None
    assert app._actuators.actuator.indicator == "red"
    assert app._actuators.actuator.buzzer == "fast"
    assert _error(store, "cmd-busy") == "DEVICE_BUSY"
    assert json.loads(store.get("cmd-busy").ack.decode("utf-8"))["error"]["message"] == "automatic warning is active"
    assert store.get("cmd-busy").status == "failed"
    app.stop()


def test_warning_preempts_a_running_test(example_config) -> None:
    app, clock, transport, engine, store = _app(example_config)
    app.start()
    _advance(app, clock, 1000)
    transport.deliver(_message(clock, "cmd-preempt", "buzzer.test", {"duration_ms": 5000}))
    app.step()
    assert app._actuators.actuator.buzzer == "on"

    transport.deliver(_message(clock, "cmd-scene", "scenario.start", {"scenario": "obstacle_approach", "speed": 1}))
    app.step()
    assert engine.name == "obstacle_approach"
    _advance(app, clock, 100)
    assert _distance_m(app) == 50.0
    _advance(app, clock, 4000)

    assert _distance_m(app) == 15.0
    assert app._risk_level == "warning"
    assert app._actuators.manual is None
    assert app._actuators.actuator.indicator == "yellow"
    assert app._actuators.actuator.buzzer == "intermittent"
    body = json.loads(store.get("cmd-preempt").ack.decode("utf-8"))
    assert body["error"]["code"] == "DEVICE_BUSY"
    assert body["error"]["message"] == "automatic warning preempted the test"
    assert json.loads(store.get("cmd-scene").ack.decode("utf-8"))["status"] == "success"
    app.stop()


def test_scenario_commands_change_the_next_sample(example_config) -> None:
    app, clock, transport, engine, store = _app(example_config)
    app.start()
    _advance(app, clock, 1000)
    transport.deliver(_message(clock, "cmd-start", "scenario.start", {"scenario": "obstacle_approach", "speed": 2}))
    app.step()

    assert engine.name == "obstacle_approach"
    assert engine.playback_speed == 2.0
    _advance(app, clock, 100)
    assert _distance_m(app) == 50.0
    _advance(app, clock, 1000)
    assert _distance_m(app) == 30.0

    started = json.loads(store.get("cmd-start").ack.decode("utf-8"))
    assert started["result"] == {"scenario": "obstacle_approach", "speed": 2.0}
    transport.deliver(_message(clock, "cmd-stop", "scenario.stop", {}))
    app.step()
    _advance(app, clock, 100)

    assert engine.name == "normal_drive"
    assert engine.playback_speed == 1.0
    assert _distance_m(app) == 50.0
    assert app._sampler.state.speed.value == 30
    stopped = json.loads(store.get("cmd-stop").ack.decode("utf-8"))
    assert stopped["status"] == "success"
    assert stopped["result"] == {"scenario": "normal_drive", "speed": 1.0}
    app.stop()


def test_expired_and_unfinishable_commands_do_not_start(example_config) -> None:
    app, clock, transport, _, store = _app(example_config)
    app.start()
    _advance(app, clock, 1000)
    transport.deliver(_message(clock, "cmd-old", "buzzer.test", {"duration_ms": 200}, issued_s_ago=3, expires_in_s=-1))
    transport.deliver(_message(clock, "cmd-tight", "indicator.test", {"color": "red", "duration_ms": 5000}, expires_in_s=1))
    transport.deliver(_message(clock, "cmd-fit", "buzzer.test", {"duration_ms": 1000}, expires_in_s=1))
    app.step()

    assert _error(store, "cmd-old") == "COMMAND_EXPIRED"
    assert json.loads(store.get("cmd-old").ack.decode("utf-8"))["error"]["message"] == "command expired before execution"
    assert _error(store, "cmd-tight") == "COMMAND_EXPIRED"
    assert json.loads(store.get("cmd-tight").ack.decode("utf-8"))["error"]["message"] == "command cannot finish before it expires"
    assert store.get("cmd-fit").status == "running"
    assert app._actuators.manual is not None
    assert app._actuators.manual.command_id == "cmd-fit"
    assert app._actuators.actuator.buzzer == "on"
    app.stop()


def test_same_command_is_acknowledged_once_and_resent_unchanged(example_config) -> None:
    app, clock, transport, _, _ = _app(example_config)
    app.start()
    _advance(app, clock, 1000)
    message = _message(clock, "cmd-once", "buzzer.test", {"duration_ms": 1000})
    transport.deliver(message)
    app.step()
    until_ns = app._actuators.manual.until_ns
    transport.deliver(message)
    app.step()

    assert app._actuators.manual.until_ns == until_ns
    assert [item for item in _acks(transport) if _ack(item)["command_id"] == "cmd-once"] == []
    _advance(app, clock, 1000)
    finished = [item for item in _acks(transport) if _ack(item)["command_id"] == "cmd-once"]
    assert len(finished) == 1
    transport.deliver(message)
    app.step()

    repeated = [item for item in _acks(transport) if _ack(item)["command_id"] == "cmd-once"]
    assert len(repeated) == 2
    assert repeated[1].payload == repeated[0].payload
    assert app._actuators.manual is None
    assert app._actuators.actuator.buzzer == "off"
    app.stop()


def test_same_id_with_different_content_does_not_run(example_config) -> None:
    app, clock, transport, engine, store = _app(example_config)
    app.start()
    _advance(app, clock, 1000)
    transport.deliver(_message(clock, "cmd-same", "scenario.start", {"scenario": "obstacle_approach", "speed": 1}))
    app.step()
    _advance(app, clock, 100)
    _advance(app, clock, 2000)
    assert engine.name == "obstacle_approach"
    assert _distance_m(app) == 30.0
    original = store.get("cmd-same").ack
    published = len(_acks(transport))

    transport.deliver(_message(clock, "cmd-same", "scenario.start", {"scenario": "lane_departure", "speed": 1}))
    app.step()
    _advance(app, clock, 100)

    assert engine.name == "obstacle_approach"
    assert _distance_m(app) == 30.0
    assert store.get("cmd-same").ack == original
    assert len(_acks(transport)) == published
    app.stop()


def test_invalid_and_unsupported_commands_fail_without_output(example_config) -> None:
    app, clock, transport, engine, store = _app(example_config)
    app.start()
    transport.deliver(_message(clock, "cmd-bad", "buzzer.test", {"duration_ms": 50}))
    transport.deliver(_message(clock, "cmd-shot", "camera.snapshot", {"quality": 80}))
    transport.deliver(_message(clock, "   ", "buzzer.test", {"duration_ms": 200}))
    transport.deliver(_message(clock, "cmd-other-car", "buzzer.test", {"duration_ms": 200}, device_id="car-002"))
    app.step()

    assert _error(store, "cmd-bad") == "INVALID_PARAMS"
    assert _error(store, "cmd-shot") == "UNSUPPORTED_COMMAND"
    assert store.get("   ") is None
    assert store.get("cmd-other-car") is None
    assert app._actuators.manual is None
    assert engine.name == "normal_drive"
    original = store.get("cmd-bad").ack
    transport.deliver(_message(clock, "cmd-bad", "buzzer.test", {"duration_ms": 60}))
    app.step()
    assert store.get("cmd-bad").ack == original
    app.stop()


def test_restart_marks_a_running_command_uncertain(example_config, tmp_path) -> None:
    clock = FakeClock()
    message = _message(clock, "cmd-crash", "buzzer.test", {"duration_ms": 200})
    parsed = parse_command(message.topic, message.payload, device_id="car-001")
    assert isinstance(parsed, DeviceCommand)
    store = CommandStore(tmp_path / "edge.db")
    store.insert_running(parsed.command_id, parsed.device_id, parsed.fingerprint)
    transport = FakeMqtt()
    engine = ScenarioEngine("normal_drive", 1.0)
    sampler = Sampler.from_config(example_config, engine, clock)
    app = EdgeApp(
        example_config,
        clock,
        transport,
        sampler,
        store=EventStore(":memory:"),
        engine=engine,
        command_store=store,
    )
    app.start()

    record = store.get("cmd-crash")
    assert record.status == "failed"
    body = json.loads(record.ack.decode("utf-8"))
    assert body["error"]["code"] == "RESULT_UNCERTAIN"
    assert body["error"]["message"] == "previous test did not finish; it was not run again"
    assert app._actuators.manual is None
    assert app._actuators.actuator.buzzer == "off"
    transport.deliver(message)
    app.step()

    assert app._actuators.manual is None
    assert store.get("cmd-crash").ack == record.ack
    published = [item.payload for item in _acks(transport)]
    assert published == [record.ack, record.ack]
    app.stop()


def test_hardware_mode_refuses_scenario_and_actuator() -> None:
    clock = FakeClock()
    store = CommandStore(":memory:")
    actuators = ActuatorManager(MockActuator())
    engine = ScenarioEngine("normal_drive", 1.0)
    handler = CommandHandler("car-001", "hardware", clock, engine, actuators, store)
    handler.handle([_message(clock, "cmd-scene", "scenario.start", {"scenario": "obstacle_approach", "speed": 1})])
    handler.handle([_message(clock, "cmd-buzz", "buzzer.test", {"duration_ms": 200})])

    assert engine.name == "normal_drive"
    assert actuators.manual is None
    assert _error(store, "cmd-scene") == "DEVICE_NOT_SUPPORTED"
    assert json.loads(store.get("cmd-scene").ack.decode("utf-8"))["error"]["message"] == "scenario commands are not available in hardware mode"
    assert _error(store, "cmd-buzz") == "ACTUATOR_UNAVAILABLE"
    assert json.loads(store.get("cmd-buzz").ack.decode("utf-8"))["error"]["message"] == "hardware actuator is not configured"
    handler.close()


def test_sqlite_failure_does_not_start_and_retries() -> None:
    class _FailOnce(CommandStore):
        def __init__(self) -> None:
            super().__init__(":memory:")
            self.fail = True

        def insert_running(self, command_id: str, device_id: str, fingerprint: str) -> None:
            if self.fail:
                self.fail = False
                raise sqlite3.OperationalError("disk full")
            super().insert_running(command_id, device_id, fingerprint)

    clock = FakeClock()
    store = _FailOnce()
    actuators = ActuatorManager(MockActuator())
    handler = CommandHandler("car-001", "mock", clock, ScenarioEngine("normal_drive", 1.0), actuators, store)
    message = _message(clock, "cmd-retry", "buzzer.test", {"duration_ms": 200})
    handler.handle([message])

    assert actuators.manual is None
    assert store.get("cmd-retry") is None
    assert handler.storage_error == "disk full"
    handler.handle([])

    assert actuators.manual is not None
    assert store.get("cmd-retry").status == "running"
    assert handler.storage_error is None
    handler.close()


def test_offline_ack_is_published_after_connect(example_config) -> None:
    app, clock, transport, _, store = _app(example_config, connect=False)
    app.start()
    transport.deliver(_message(clock, "cmd-offline", "scenario.stop", {}))
    app.step()

    assert _acks(transport) == []
    saved = store.unpublished()
    assert len(saved) == 1
    assert json.loads(saved[0].payload.decode("utf-8"))["status"] == "success"
    transport.fail_connect = False
    transport.connect()
    app.step()

    published = _acks(transport)
    assert len(published) == 1
    assert published[0].payload == saved[0].payload
    assert published[0].qos == 1
    assert published[0].retain is False
    app.stop()


def test_shutdown_cancels_a_running_test(example_config) -> None:
    app, clock, transport, _, _ = _app(example_config)
    app.start()
    _advance(app, clock, 1000)
    transport.deliver(_message(clock, "cmd-cancel", "buzzer.test", {"duration_ms": 5000}))
    app.step()
    assert app._actuators.actuator.buzzer == "on"
    app.stop()

    body = _ack(_acks(transport)[-1])
    assert body["command_id"] == "cmd-cancel"
    assert body["status"] == "failed"
    assert body["error"]["code"] == "CANCELLED"
    assert body["error"]["message"] == "test output cancelled during shutdown"
    assert "result" not in body
    assert app._actuators.actuator.buzzer == "off"
    assert app._actuators.actuator.indicator == "green"


def test_from_config_stores_commands_beside_events(example_config, tmp_path) -> None:
    import dataclasses

    config = dataclasses.replace(
        example_config,
        storage=dataclasses.replace(example_config.storage, sqlite_path=tmp_path / "edge.db"),
    )
    app = EdgeApp.from_config(config, FakeClock(), FakeMqtt())
    app.start()
    app.stop()
    app.stop()

    connection = sqlite3.connect(tmp_path / "edge.db")
    names = {row[0] for row in connection.execute("SELECT name FROM sqlite_master WHERE type = 'table'")}
    connection.close()
    assert {"events", "commands"} <= names
