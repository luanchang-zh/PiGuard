"""Load and validate the edge YAML snapshot.

PyYAML 6.0.3 documents yaml.safe_load for untrusted documents. This loader
uses that function and then checks keys itself. Relative paths resolve from
the directory of the file passed to load_config. A sibling local.yaml deep
merges over it.
"""

import math
from pathlib import Path
from typing import Any
from urllib.parse import urlsplit

import yaml

from piguard_edge.config.model import (
    ActuatorsConfig,
    CameraConfig,
    CameraHardware,
    DeviceConfig,
    DistanceHardware,
    EdgeConfig,
    FeaturesConfig,
    GyroHardware,
    HardwareConfig,
    HttpConfig,
    LaneRuleConfig,
    MappingConfig,
    MqttConfig,
    ObstacleRuleConfig,
    RateConfig,
    RulesConfig,
    RuntimeConfig,
    SensorsConfig,
    SharpTurnRuleConfig,
    SimulationConfig,
    StorageConfig,
    TemperatureHardware,
    TemperatureRuleConfig,
    TtlConfig,
)
from piguard_edge.simulation.engine import PLAYBACK_SPEEDS
from piguard_edge.simulation.scenarios import SCENARIO_NAMES

_TOP_LEVEL = {
    "device",
    "mqtt",
    "http",
    "sensors",
    "speed",
    "simulation",
    "mapping",
    "runtime",
    "rules",
    "actuators",
    "storage",
    "features",
    "hardware",
}
_MODES = {"mock", "hardware", "replay"}
_YAW_AXES = {"x", "y", "z"}


class ConfigError(ValueError):
    pass


def load_config(path: str | Path) -> EdgeConfig:
    source = Path(path)
    data = _read_mapping(source)
    local = source.parent / "local.yaml"
    if local.is_file():
        data = _deep_merge(data, _read_mapping(local))
    return _parse(data, source)


def _read_mapping(path: Path) -> dict[str, Any]:
    if not path.is_file():
        raise ConfigError(f"config not found: {path}")
    try:
        loaded = yaml.safe_load(path.read_text(encoding="utf-8"))
    except yaml.YAMLError as exc:
        raise ConfigError(f"{path} is not valid YAML") from exc
    if not isinstance(loaded, dict):
        raise ConfigError(f"{path} must be a mapping")
    return loaded


def _deep_merge(base: dict[str, Any], override: dict[str, Any]) -> dict[str, Any]:
    merged = dict(base)
    for key, value in override.items():
        current = merged.get(key)
        if isinstance(current, dict) and isinstance(value, dict):
            merged[key] = _deep_merge(current, value)
        else:
            merged[key] = value
    return merged


def _parse(data: dict[str, Any], source: Path) -> EdgeConfig:
    _unknown(data, _TOP_LEVEL, "config")
    _require(data, _TOP_LEVEL, "config")
    config_dir = source.parent
    return EdgeConfig(
        device=_device(data["device"]),
        mqtt=_mqtt(data["mqtt"]),
        http=_http(data["http"]),
        sensors=_sensors(data["sensors"]),
        speed=_rate(data["speed"], "speed"),
        simulation=_simulation(data["simulation"]),
        mapping=_mapping(data["mapping"]),
        runtime=_runtime(data["runtime"]),
        rules=_rules(data["rules"]),
        actuators=_actuators(data["actuators"]),
        storage=_storage(data["storage"], config_dir),
        features=_features(data["features"]),
        hardware=_hardware(data["hardware"]),
        source_path=source,
    )


def _device(value: Any) -> DeviceConfig:
    data = _section(value, {"id", "mode", "config_version"}, "device")
    return DeviceConfig(
        id=_text(data["id"], "device.id"),
        mode=_choice(data["mode"], "device.mode", _MODES),
        config_version=_int(data["config_version"], "device.config_version", minimum=0),
    )


def _mqtt(value: Any) -> MqttConfig:
    keys = {
        "host",
        "port",
        "client_id",
        "keepalive_s",
        "telemetry_qos",
        "event_qos",
        "status_qos",
        "command_qos",
        "ack_qos",
        "status_heartbeat_s",
        "username",
        "password",
    }
    data = _section(value, keys, "mqtt")
    return MqttConfig(
        host=_text(data["host"], "mqtt.host"),
        port=_int(data["port"], "mqtt.port", minimum=1, maximum=65535),
        client_id=_text(data["client_id"], "mqtt.client_id"),
        keepalive_s=_int(data["keepalive_s"], "mqtt.keepalive_s", minimum=1),
        telemetry_qos=_int(data["telemetry_qos"], "mqtt.telemetry_qos", minimum=0, maximum=2),
        event_qos=_int(data["event_qos"], "mqtt.event_qos", minimum=0, maximum=2),
        status_qos=_int(data["status_qos"], "mqtt.status_qos", minimum=0, maximum=2),
        command_qos=_int(data["command_qos"], "mqtt.command_qos", minimum=0, maximum=2),
        ack_qos=_int(data["ack_qos"], "mqtt.ack_qos", minimum=0, maximum=2),
        status_heartbeat_s=_int(data["status_heartbeat_s"], "mqtt.status_heartbeat_s", minimum=1),
        username=_optional_text(data["username"], "mqtt.username"),
        password=_optional_text(data["password"], "mqtt.password"),
    )


def _http(value: Any) -> HttpConfig:
    if not isinstance(value, dict):
        raise ConfigError("http must be a mapping")
    data = value
    _unknown(data, {"base_url", "timeout_s", "username", "password"}, "http")
    _require(data, {"base_url", "timeout_s"}, "http")
    return HttpConfig(
        base_url=_text(data["base_url"], "http.base_url"),
        timeout_s=_number(data["timeout_s"], "http.timeout_s", minimum=0, inclusive=False),
        username=_optional_text(data.get("username", ""), "http.username"),
        password=_optional_text(data.get("password", ""), "http.password"),
    )


def _sensors(value: Any) -> SensorsConfig:
    data = _section(value, {"distance", "gyro", "temperature", "camera"}, "sensors")
    return SensorsConfig(
        distance=_rate(data["distance"], "sensors.distance"),
        gyro=_rate(data["gyro"], "sensors.gyro"),
        temperature=_rate(data["temperature"], "sensors.temperature"),
        camera=_camera(data["camera"]),
    )


def _camera(value: Any) -> CameraConfig:
    path = "sensors.camera"
    required = {"driver", "capture_fps", "process_fps", "width", "height", "video_path"}
    optional = {"remote_url", "remote_token_env", "remote_timeout_s", "remote_max_age_ms"}
    if not isinstance(value, dict):
        raise ConfigError(f"{path} must be a mapping")
    _unknown(value, required | optional, path)
    _require(value, required, path)
    config = CameraConfig(
        driver=_text(value["driver"], f"{path}.driver"),
        capture_fps=_int(value["capture_fps"], f"{path}.capture_fps", minimum=1),
        process_fps=_int(value["process_fps"], f"{path}.process_fps", minimum=1),
        width=_int(value["width"], f"{path}.width", minimum=1, maximum=1920),
        height=_int(value["height"], f"{path}.height", minimum=1, maximum=1080),
        video_path=_optional_text(value["video_path"], f"{path}.video_path"),
        remote_url=_optional_text(value.get("remote_url", ""), f"{path}.remote_url"),
        remote_token_env=_text(value.get("remote_token_env", "PIGUARD_CAMERA_TOKEN"), f"{path}.remote_token_env"),
        remote_timeout_s=_number(value.get("remote_timeout_s", 0.5), f"{path}.remote_timeout_s", minimum=0, inclusive=False),
        remote_max_age_ms=_int(value.get("remote_max_age_ms", 1000), f"{path}.remote_max_age_ms", minimum=1),
    )
    if not math.isfinite(config.remote_timeout_s) or config.remote_timeout_s > 5:
        raise ConfigError(f"{path}.remote_timeout_s must be finite and <= 5")
    if config.driver == "remote":
        try:
            url = urlsplit(config.remote_url)
            valid = url.scheme in {"http", "https"} and url.hostname and url.port != 0
        except ValueError:
            valid = False
        if not valid or url.username or url.password or url.query or url.fragment or url.path != "/frame.jpg":
            raise ConfigError(f"{path}.remote_url must be http(s)://host:port/frame.jpg without credentials or query")
    return config


def _rate(value: Any, path: str) -> RateConfig:
    data = _section(value, {"driver", "sample_hz"}, path)
    return RateConfig(
        driver=_text(data["driver"], f"{path}.driver"),
        sample_hz=_int(data["sample_hz"], f"{path}.sample_hz", minimum=1),
    )


def _simulation(value: Any) -> SimulationConfig:
    data = _section(value, {"scenario", "playback_speed"}, "simulation")
    speed = _number(data["playback_speed"], "simulation.playback_speed")
    if speed not in PLAYBACK_SPEEDS:
        raise ConfigError("simulation.playback_speed must be 0.5, 1.0, or 2.0")
    return SimulationConfig(
        scenario=_choice(data["scenario"], "simulation.scenario", set(SCENARIO_NAMES)),
        playback_speed=speed,
    )


def _mapping(value: Any) -> MappingConfig:
    data = _section(value, {"distance_scale"}, "mapping")
    return MappingConfig(
        distance_scale=_number(data["distance_scale"], "mapping.distance_scale", minimum=0, inclusive=False)
    )


def _runtime(value: Any) -> RuntimeConfig:
    data = _section(value, {"rule_hz", "telemetry_hz", "ttl_ms"}, "runtime")
    ttl = _section(data["ttl_ms"], {"speed", "distance", "gyro", "lane", "temperature"}, "runtime.ttl_ms")
    return RuntimeConfig(
        rule_hz=_int(data["rule_hz"], "runtime.rule_hz", minimum=1),
        telemetry_hz=_int(data["telemetry_hz"], "runtime.telemetry_hz", minimum=1),
        ttl_ms=TtlConfig(
            speed=_int(ttl["speed"], "runtime.ttl_ms.speed", minimum=1),
            distance=_int(ttl["distance"], "runtime.ttl_ms.distance", minimum=1),
            gyro=_int(ttl["gyro"], "runtime.ttl_ms.gyro", minimum=1),
            lane=_int(ttl["lane"], "runtime.ttl_ms.lane", minimum=1),
            temperature=_int(ttl["temperature"], "runtime.ttl_ms.temperature", minimum=1),
        ),
    )


def _rules(value: Any) -> RulesConfig:
    data = _section(value, {"obstacle", "lane_departure", "sharp_turn", "temperature"}, "rules")
    obstacle = _section(
        data["obstacle"],
        {
            "warning_distance_m",
            "danger_distance_m",
            "headway_enabled",
            "warning_headway_s",
            "danger_headway_s",
        },
        "rules.obstacle",
    )
    lane = _section(data["lane_departure"], {"offset_threshold", "duration_ms"}, "rules.lane_departure")
    turn = _section(
        data["sharp_turn"],
        {"yaw_threshold_deg_s", "min_speed_kmh", "duration_ms"},
        "rules.sharp_turn",
    )
    temperature = _section(
        data["temperature"],
        {"trigger_c", "recover_c", "duration_ms", "recover_duration_ms"},
        "rules.temperature",
    )
    return RulesConfig(
        obstacle=ObstacleRuleConfig(
            warning_distance_m=_number(obstacle["warning_distance_m"], "rules.obstacle.warning_distance_m"),
            danger_distance_m=_number(obstacle["danger_distance_m"], "rules.obstacle.danger_distance_m"),
            headway_enabled=_bool(obstacle["headway_enabled"], "rules.obstacle.headway_enabled"),
            warning_headway_s=_number(obstacle["warning_headway_s"], "rules.obstacle.warning_headway_s"),
            danger_headway_s=_number(obstacle["danger_headway_s"], "rules.obstacle.danger_headway_s"),
        ),
        lane_departure=LaneRuleConfig(
            offset_threshold=_number(lane["offset_threshold"], "rules.lane_departure.offset_threshold"),
            duration_ms=_int(lane["duration_ms"], "rules.lane_departure.duration_ms", minimum=0),
        ),
        sharp_turn=SharpTurnRuleConfig(
            yaw_threshold_deg_s=_number(turn["yaw_threshold_deg_s"], "rules.sharp_turn.yaw_threshold_deg_s"),
            min_speed_kmh=_number(turn["min_speed_kmh"], "rules.sharp_turn.min_speed_kmh"),
            duration_ms=_int(turn["duration_ms"], "rules.sharp_turn.duration_ms", minimum=0),
        ),
        temperature=TemperatureRuleConfig(
            trigger_c=_number(temperature["trigger_c"], "rules.temperature.trigger_c"),
            recover_c=_number(temperature["recover_c"], "rules.temperature.recover_c"),
            duration_ms=_int(temperature["duration_ms"], "rules.temperature.duration_ms", minimum=0),
            recover_duration_ms=_int(
                temperature["recover_duration_ms"],
                "rules.temperature.recover_duration_ms",
                minimum=0,
            ),
        ),
    )


def _actuators(value: Any) -> ActuatorsConfig:
    data = _section(value, {"driver"}, "actuators")
    return ActuatorsConfig(driver=_text(data["driver"], "actuators.driver"))


def _storage(value: Any, config_dir: Path) -> StorageConfig:
    data = _section(
        value,
        {"sqlite_path", "event_confirmation", "confirmed_event_retention_days"},
        "storage",
    )
    raw_path = Path(_text(data["sqlite_path"], "storage.sqlite_path"))
    if not raw_path.is_absolute():
        raw_path = config_dir / raw_path
    return StorageConfig(
        sqlite_path=raw_path.resolve(),
        event_confirmation=_text(data["event_confirmation"], "storage.event_confirmation"),
        confirmed_event_retention_days=_int(
            data["confirmed_event_retention_days"],
            "storage.confirmed_event_retention_days",
            minimum=0,
        ),
    )


def _features(value: Any) -> FeaturesConfig:
    data = _section(value, {"remote_config", "frame_upload"}, "features")
    return FeaturesConfig(
        remote_config=_bool(data["remote_config"], "features.remote_config"),
        frame_upload=_bool(data["frame_upload"], "features.frame_upload"),
    )


def _hardware(value: Any) -> HardwareConfig:
    data = _section(value, {"i2c_bus", "distance", "gyro", "temperature", "camera"}, "hardware")
    distance = _section(
        data["distance"],
        {"trigger_bcm", "echo_bcm", "max_distance_m", "pulse_timeout_ms"},
        "hardware.distance",
    )
    gyro = _section(
        data["gyro"],
        {"address", "yaw_axis", "yaw_sign", "full_scale_dps"},
        "hardware.gyro",
    )
    temperature = _section(
        data["temperature"],
        {"aht20_address", "bmp280_address"},
        "hardware.temperature",
    )
    camera = _section(data["camera"], {"width", "height"}, "hardware.camera")
    yaw_sign = _int(gyro["yaw_sign"], "hardware.gyro.yaw_sign")
    if yaw_sign not in (-1, 1):
        raise ConfigError("hardware.gyro.yaw_sign must be -1 or 1")
    return HardwareConfig(
        i2c_bus=_int(data["i2c_bus"], "hardware.i2c_bus", minimum=0),
        distance=DistanceHardware(
            trigger_bcm=_int(distance["trigger_bcm"], "hardware.distance.trigger_bcm"),
            echo_bcm=_int(distance["echo_bcm"], "hardware.distance.echo_bcm"),
            max_distance_m=_number(
                distance["max_distance_m"],
                "hardware.distance.max_distance_m",
                minimum=0,
                inclusive=False,
            ),
            pulse_timeout_ms=_int(distance["pulse_timeout_ms"], "hardware.distance.pulse_timeout_ms", minimum=1),
        ),
        gyro=GyroHardware(
            address=_int(gyro["address"], "hardware.gyro.address", minimum=0, maximum=127),
            yaw_axis=_choice(gyro["yaw_axis"], "hardware.gyro.yaw_axis", _YAW_AXES),
            yaw_sign=yaw_sign,
            full_scale_dps=_int(gyro["full_scale_dps"], "hardware.gyro.full_scale_dps", minimum=1),
        ),
        temperature=TemperatureHardware(
            aht20_address=_int(temperature["aht20_address"], "hardware.temperature.aht20_address", minimum=0, maximum=127),
            bmp280_address=_int(
                temperature["bmp280_address"],
                "hardware.temperature.bmp280_address",
                minimum=0,
                maximum=127,
            ),
        ),
        camera=CameraHardware(
            width=_int(camera["width"], "hardware.camera.width", minimum=1),
            height=_int(camera["height"], "hardware.camera.height", minimum=1),
        ),
    )


def _section(value: Any, keys: set[str], path: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise ConfigError(f"{path} must be a mapping")
    _unknown(value, keys, path)
    _require(value, keys, path)
    return value


def _unknown(data: dict[str, Any], keys: set[str], path: str) -> None:
    extra = sorted(set(data) - keys)
    if extra:
        raise ConfigError(f"{path} has unknown keys: {', '.join(extra)}")


def _require(data: dict[str, Any], keys: set[str], path: str) -> None:
    missing = sorted(keys - set(data))
    if missing:
        raise ConfigError(f"{path} is missing {', '.join(missing)}")


def _optional_text(value: Any, path: str) -> str:
    if value is None:
        return ""
    if not isinstance(value, str):
        raise ConfigError(f"{path} must be a string")
    return value


def _text(value: Any, path: str) -> str:
    if not isinstance(value, str) or value.strip() == "":
        raise ConfigError(f"{path} must be a non-empty string")
    return value


def _choice(value: Any, path: str, options: set[str]) -> str:
    text = _text(value, path)
    if text not in options:
        raise ConfigError(f"{path} must be one of {', '.join(sorted(options))}")
    return text


def _bool(value: Any, path: str) -> bool:
    if not isinstance(value, bool):
        raise ConfigError(f"{path} must be a boolean")
    return value


def _int(value: Any, path: str, minimum: int | None = None, maximum: int | None = None) -> int:
    if isinstance(value, bool) or not isinstance(value, int):
        raise ConfigError(f"{path} must be an integer")
    if minimum is not None and value < minimum:
        raise ConfigError(f"{path} must be >= {minimum}")
    if maximum is not None and value > maximum:
        raise ConfigError(f"{path} must be <= {maximum}")
    return value


def _number(
    value: Any,
    path: str,
    minimum: float | None = None,
    inclusive: bool = True,
) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ConfigError(f"{path} must be a number")
    number = float(value)
    if minimum is not None and (number < minimum or (not inclusive and number == minimum)):
        raise ConfigError(f"{path} must be greater than {minimum}")
    return number
