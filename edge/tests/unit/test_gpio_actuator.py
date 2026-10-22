import json
import threading
import time

import pytest
import yaml

from piguard_edge.actuators.gpio import GpioBuzzer
from piguard_edge.actuators.manager import ActuatorManager
from piguard_edge.actuators.mock import ActuatorState
from piguard_edge.commands.handler import CommandHandler
from piguard_edge.config.loader import ConfigError, load_config
from piguard_edge.runtime.clock import FakeClock
from piguard_edge.storage.commands import CommandStore
from test_commands import _message


class FakeGpio:
    def __init__(self):
        self.writes = []
        self.active = threading.Event()
        self.freed = self.closed = False

    def gpiochip_open(self, chip): return 42
    def gpio_claim_output(self, handle, bcm, level): self.writes.append(level)
    def gpio_write(self, handle, bcm, level):
        self.writes.append(level)
        if level: self.active.set()
    def gpio_free(self, handle, bcm): self.freed = True
    def gpiochip_close(self, handle): self.closed = True


def test_tone_stops_at_deadline_without_command_loop_and_releases_gpio():
    gpio = FakeGpio()
    buzzer = GpioBuzzer(0, 17, 500, True, gpio=gpio)
    try:
        buzzer.set_pattern("on", time.monotonic_ns() + 50_000_000)
        assert gpio.active.wait(1)
        time.sleep(.1)
        before = len(gpio.writes)
        time.sleep(.03)
        assert len(gpio.writes) == before
        assert gpio.writes[-1] == 0
    finally:
        buzzer.close()
    assert gpio.freed and gpio.closed and gpio.writes[-1] == 0
    buzzer.close()


def test_close_stops_an_automatic_alarm():
    gpio = FakeGpio()
    buzzer = GpioBuzzer(0, 17, 500, True, gpio=gpio)
    buzzer.set_pattern("fast")
    assert gpio.active.wait(1)
    buzzer.close()
    before = len(gpio.writes)
    time.sleep(.02)
    assert len(gpio.writes) == before
    assert gpio.writes[-1] == 0 and gpio.closed


class FakeBuzzer:
    error = None
    def __init__(self): self.pattern = "off"; self.closed = False
    def set_pattern(self, pattern, until_ns=None): self.pattern = pattern
    def close(self): self.pattern = "off"; self.closed = True


def test_real_buzzer_commands_and_missing_indicator_return_truthful_acks():
    clock, store, output = FakeClock(), CommandStore(":memory:"), FakeBuzzer()
    manager = ActuatorManager(ActuatorState(), output)
    handler = CommandHandler("car-001", "hardware", clock, None, manager, store)
    manager.apply("normal")
    handler.handle([_message(clock, "buzz", "buzzer.test", {"duration_ms": 200})])
    assert output.pattern == "on" and store.get("buzz").status == "running"
    clock.advance_ns(200_000_000); handler.tick(clock.monotonic_ns())
    assert output.pattern == "off" and store.get("buzz").status == "success"
    handler.handle([_message(clock, "led", "indicator.test", {"duration_ms": 200, "color": "red"})])
    assert json.loads(store.get("led").ack)["error"]["code"] == "ACTUATOR_UNAVAILABLE"
    handler.close(); manager.close()


def test_real_buzzer_error_fails_a_running_command():
    clock, store, output = FakeClock(), CommandStore(":memory:"), FakeBuzzer()
    manager = ActuatorManager(ActuatorState(), output)
    handler = CommandHandler("car-001", "mock", clock, None, manager, store)
    handler.handle([_message(clock, "buzz", "buzzer.test", {"duration_ms": 200})])
    output.error = "GPIO I/O failed"
    handler.tick(clock.monotonic_ns())
    assert json.loads(store.get("buzz").ack)["error"]["code"] == "ACTUATOR_UNAVAILABLE"
    handler.close(); manager.close()


def test_real_buzzer_keeps_automatic_warning_priority():
    output = FakeBuzzer(); manager = ActuatorManager(ActuatorState(), output)
    assert manager.begin_test("buzz", "buzzer", 1_000_000)
    manager.apply("warning")
    assert manager.manual is None and output.pattern == "intermittent"
    assert manager.take_preempted() == ["buzz"]
    assert not manager.begin_test("another", "buzzer", 2_000_000)
    manager.apply("danger"); assert output.pattern == "fast"
    manager.close(); assert output.pattern == "off"


@pytest.mark.parametrize("settings", [
    {"driver": "gpio"}, {"driver": "gpio", "buzzer_bcm": 40},
    {"driver": "gpio", "buzzer_bcm": True},
    {"driver": "gpio", "buzzer_bcm": 17, "buzzer_frequency_hz": 0},
])
def test_invalid_gpio_configuration_is_rejected(example_config, tmp_path, settings):
    raw = yaml.safe_load(example_config.source_path.read_text())
    raw["actuators"] = settings
    path = tmp_path / "edge.yaml"; path.write_text(yaml.safe_dump(raw))
    with pytest.raises(ConfigError): load_config(path)


def test_mock_config_does_not_import_or_open_gpio(example_config, monkeypatch):
    monkeypatch.setattr(GpioBuzzer, "__init__", lambda *a, **k: pytest.fail("mock opened GPIO"))
    manager = ActuatorManager.from_config(example_config)
    assert manager.supports("indicator", "mock")
    manager.close()
