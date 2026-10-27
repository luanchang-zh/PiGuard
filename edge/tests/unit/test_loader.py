from pathlib import Path

import pytest
import yaml

from piguard_edge.config.loader import ConfigError, load_config
from piguard_edge.drivers.factory import build_sources
from piguard_edge.runtime.workers import Sampler
from piguard_edge.simulation.engine import ScenarioEngine


def test_example_config_keeps_hardware_and_resolves_storage(example_config) -> None:
    example_path = example_config.source_path
    assert example_config.device.id == "car-001"
    assert example_config.device.mode == "mock"
    assert example_config.hardware.distance.trigger_bcm == 23
    assert example_config.hardware.distance.echo_bcm == 24
    assert example_config.hardware.gyro.address == 104
    assert example_config.hardware.gyro.yaw_axis == "z"
    assert example_config.hardware.gyro.yaw_sign == 1
    assert example_config.hardware.temperature.aht20_address == 56
    assert example_config.hardware.temperature.bmp280_address == 118
    assert example_config.mapping.distance_scale == 100
    assert example_config.storage.sqlite_path == (example_path.parent / "../data/edge.db").resolve()


def test_mock_drivers_do_not_receive_hardware(example_config) -> None:
    engine = ScenarioEngine(example_config.simulation.scenario, example_config.simulation.playback_speed)
    sources = build_sources(
        engine,
        distance_scale=example_config.mapping.distance_scale,
        distance_driver=example_config.sensors.distance.driver,
        camera_driver=example_config.sensors.camera.driver,
        gyro_driver=example_config.sensors.gyro.driver,
        temperature_driver=example_config.sensors.temperature.driver,
        speed_driver=example_config.speed.driver,
    )

    for sensor in (sources.distance, sources.camera, sources.gyro, sources.temperature, sources.speed):
        fields = vars(sensor)
        assert "trigger_bcm" not in fields
        assert "echo_bcm" not in fields
        assert "address" not in fields
        assert "aht20_address" not in fields
        assert "width" not in fields
        assert "height" not in fields


def test_missing_hardware_is_rejected(example_config, tmp_path: Path) -> None:
    data = yaml.safe_load(example_config.source_path.read_text(encoding="utf-8"))
    data.pop("hardware")
    path = tmp_path / "no-hardware.yaml"
    path.write_text(yaml.safe_dump(data), encoding="utf-8")

    with pytest.raises(ConfigError, match="hardware"):
        load_config(path)


def test_unknown_top_level_key_is_rejected(example_config, tmp_path: Path) -> None:
    path = tmp_path / "extra.yaml"
    path.write_text(example_config.source_path.read_text(encoding="utf-8") + "\nnot_a_section: 1\n", encoding="utf-8")

    with pytest.raises(ConfigError, match="unknown"):
        load_config(path)


def test_local_yaml_overrides_scenario_only(example_config, tmp_path: Path) -> None:
    target = tmp_path / "default.example.yaml"
    target.write_text(example_config.source_path.read_text(encoding="utf-8"), encoding="utf-8")
    (tmp_path / "local.yaml").write_text(
        "simulation:\n  scenario: sharp_turn\n  playback_speed: 2.0\n",
        encoding="utf-8",
    )

    loaded = load_config(target)

    assert loaded.simulation.scenario == "sharp_turn"
    assert loaded.simulation.playback_speed == 2.0
    assert loaded.hardware.distance.trigger_bcm == 23
    assert loaded.storage.sqlite_path == (tmp_path / "../data/edge.db").resolve()


def test_replay_mode_is_not_built(example_config, tmp_path: Path) -> None:
    text = example_config.source_path.read_text(encoding="utf-8").replace("mode: mock", "mode: replay", 1)
    path = tmp_path / "hardware-mode.yaml"
    path.write_text(text, encoding="utf-8")
    loaded = load_config(path)
    engine = ScenarioEngine("normal_drive", 1.0)

    with pytest.raises(ValueError, match="mock"):
        Sampler.from_config(loaded, engine, _clock())


def _clock():
    from piguard_edge.runtime.clock import FakeClock

    return FakeClock()


def test_cloud_mqtt_websocket_config_and_legacy_defaults(example_config, tmp_path):
    assert example_config.mqtt.transport == "tcp"
    assert example_config.mqtt.websocket_path == "/mqtt"
    data = yaml.safe_load(example_config.source_path.read_text())
    data["mqtt"].update(transport="websockets", websocket_path="/mqtt", port=80)
    path = tmp_path / "cloud.yaml"
    path.write_text(yaml.safe_dump(data))
    config = load_config(path)
    assert config.mqtt.transport == "websockets"
    assert config.mqtt.port == 80
    from piguard_edge.transport.mqtt import PahoMqtt
    session = PahoMqtt(config)
    assert session._client.transport == "websockets"


@pytest.mark.parametrize("key,value", [
    ("transport", "http"), ("websocket_path", "mqtt"),
    ("websocket_path", "/mqtt\r\nInjected: yes"), ("websocket_path", "/mqtt?token=secret"),
])
def test_invalid_cloud_mqtt_settings_are_rejected(example_config, tmp_path, key, value):
    data = yaml.safe_load(example_config.source_path.read_text())
    data["mqtt"][key] = value
    path = tmp_path / "bad-cloud.yaml"
    path.write_text(yaml.safe_dump(data))
    with pytest.raises(ConfigError, match="mqtt"):
        load_config(path)
