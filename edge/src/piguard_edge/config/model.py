"""Immutable configuration snapshot produced by the loader."""

from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True, slots=True)
class DeviceConfig:
    id: str
    mode: str
    config_version: int


@dataclass(frozen=True, slots=True)
class MqttConfig:
    host: str
    port: int
    client_id: str
    keepalive_s: int
    telemetry_qos: int
    event_qos: int
    status_qos: int
    command_qos: int
    ack_qos: int
    status_heartbeat_s: int
    username: str
    password: str


@dataclass(frozen=True, slots=True)
class HttpConfig:
    base_url: str
    timeout_s: float
    username: str = ""
    password: str = ""


@dataclass(frozen=True, slots=True)
class RateConfig:
    driver: str
    sample_hz: int


@dataclass(frozen=True, slots=True)
class CameraConfig:
    driver: str
    capture_fps: int
    process_fps: int
    width: int
    height: int
    video_path: str
    remote_url: str = ""
    remote_token_env: str = "PIGUARD_CAMERA_TOKEN"
    remote_timeout_s: float = 0.5
    remote_max_age_ms: int = 1000


@dataclass(frozen=True, slots=True)
class SensorsConfig:
    distance: RateConfig
    gyro: RateConfig
    temperature: RateConfig
    camera: CameraConfig


@dataclass(frozen=True, slots=True)
class SimulationConfig:
    scenario: str
    playback_speed: float


@dataclass(frozen=True, slots=True)
class MappingConfig:
    distance_scale: float


@dataclass(frozen=True, slots=True)
class TtlConfig:
    speed: int
    distance: int
    gyro: int
    lane: int
    temperature: int


@dataclass(frozen=True, slots=True)
class RuntimeConfig:
    rule_hz: int
    telemetry_hz: int
    ttl_ms: TtlConfig


@dataclass(frozen=True, slots=True)
class ObstacleRuleConfig:
    warning_distance_m: float
    danger_distance_m: float
    headway_enabled: bool
    warning_headway_s: float
    danger_headway_s: float


@dataclass(frozen=True, slots=True)
class LaneRuleConfig:
    offset_threshold: float
    duration_ms: int


@dataclass(frozen=True, slots=True)
class SharpTurnRuleConfig:
    yaw_threshold_deg_s: float
    min_speed_kmh: float
    duration_ms: int


@dataclass(frozen=True, slots=True)
class TemperatureRuleConfig:
    trigger_c: float
    recover_c: float
    duration_ms: int
    recover_duration_ms: int


@dataclass(frozen=True, slots=True)
class RulesConfig:
    obstacle: ObstacleRuleConfig
    lane_departure: LaneRuleConfig
    sharp_turn: SharpTurnRuleConfig
    temperature: TemperatureRuleConfig


@dataclass(frozen=True, slots=True)
class ActuatorsConfig:
    driver: str


@dataclass(frozen=True, slots=True)
class StorageConfig:
    sqlite_path: Path
    event_confirmation: str
    confirmed_event_retention_days: int


@dataclass(frozen=True, slots=True)
class FeaturesConfig:
    remote_config: bool
    frame_upload: bool


@dataclass(frozen=True, slots=True)
class DistanceHardware:
    trigger_bcm: int
    echo_bcm: int
    max_distance_m: float
    pulse_timeout_ms: int


@dataclass(frozen=True, slots=True)
class GyroHardware:
    address: int
    yaw_axis: str
    yaw_sign: int
    full_scale_dps: int


@dataclass(frozen=True, slots=True)
class TemperatureHardware:
    aht20_address: int
    bmp280_address: int


@dataclass(frozen=True, slots=True)
class CameraHardware:
    width: int
    height: int


@dataclass(frozen=True, slots=True)
class HardwareConfig:
    i2c_bus: int
    distance: DistanceHardware
    gyro: GyroHardware
    temperature: TemperatureHardware
    camera: CameraHardware


@dataclass(frozen=True, slots=True)
class EdgeConfig:
    device: DeviceConfig
    mqtt: MqttConfig
    http: HttpConfig
    sensors: SensorsConfig
    speed: RateConfig
    simulation: SimulationConfig
    mapping: MappingConfig
    runtime: RuntimeConfig
    rules: RulesConfig
    actuators: ActuatorsConfig
    storage: StorageConfig
    features: FeaturesConfig
    hardware: HardwareConfig
    source_path: Path
