"""Keyframe sequences from the course-design scenes.

Each keyframe lasts one second of scenario time. Values a scene does not
mention stay at the normal-drive baseline. sensor_failure steps through its
three listed faults; it does not fail every sensor at once.
"""

from dataclasses import dataclass, replace

from piguard_edge.models.sample import SampleStatus

KEYFRAME_NS = 1_000_000_000
SIMULATED_LANE_CONFIDENCE = 1.0

SCENARIO_NAMES = (
    "normal_drive",
    "obstacle_approach",
    "lane_departure",
    "sharp_turn",
    "high_temperature",
    "sensor_failure",
)


@dataclass(frozen=True, slots=True)
class Scene:
    road_distance_m: float | None
    distance_status: SampleStatus
    speed_kmh: float | None
    speed_status: SampleStatus
    gyro_x: float | None
    gyro_y: float | None
    gyro_z: float | None
    gyro_status: SampleStatus
    temperature_c: float | None
    temperature_status: SampleStatus
    lane_offset: float | None
    lane_valid: bool
    camera_status: SampleStatus


def normal_scene() -> Scene:
    return Scene(
        road_distance_m=50.0,
        distance_status=SampleStatus.OK,
        speed_kmh=30.0,
        speed_status=SampleStatus.OK,
        gyro_x=0.0,
        gyro_y=0.0,
        gyro_z=2.0,
        gyro_status=SampleStatus.OK,
        temperature_c=25.0,
        temperature_status=SampleStatus.OK,
        lane_offset=0.0,
        lane_valid=True,
        camera_status=SampleStatus.OK,
    )


def _frames(values: tuple[float, ...], field: str) -> tuple[Scene, ...]:
    baseline = normal_scene()
    return tuple(replace(baseline, **{field: value}) for value in values)


def _failure_frames() -> tuple[Scene, ...]:
    baseline = normal_scene()
    distance = replace(
        baseline,
        road_distance_m=None,
        distance_status=SampleStatus.TIMEOUT,
    )
    camera = replace(
        baseline,
        lane_offset=None,
        lane_valid=False,
        camera_status=SampleStatus.UNAVAILABLE,
    )
    gyro = replace(
        baseline,
        gyro_x=None,
        gyro_y=None,
        gyro_z=None,
        gyro_status=SampleStatus.ERROR,
    )
    return (distance, camera, gyro)


SCENES: dict[str, tuple[Scene, ...]] = {
    "normal_drive": (normal_scene(),),
    "obstacle_approach": _frames((50, 40, 30, 20, 15, 10, 5), "road_distance_m"),
    "lane_departure": _frames((0.0, 0.1, 0.2, 0.3, 0.4, 0.5), "lane_offset"),
    "sharp_turn": _frames((5, 10, 20, 35, 45, 55), "gyro_z"),
    "high_temperature": _frames((25, 28, 30, 32, 34, 36, 38), "temperature_c"),
    "sensor_failure": _failure_frames(),
}


def scene_at(name: str, elapsed_ns: int, playback_speed: float) -> Scene:
    frames = SCENES[name]
    if elapsed_ns < 0:
        elapsed_ns = 0
    scaled_ns = _scale_elapsed(elapsed_ns, playback_speed)
    index = scaled_ns // KEYFRAME_NS
    if index >= len(frames):
        index = len(frames) - 1
    return frames[index]


def _scale_elapsed(elapsed_ns: int, playback_speed: float) -> int:
    if playback_speed == 1.0:
        return elapsed_ns
    if playback_speed == 2.0:
        return elapsed_ns * 2
    if playback_speed == 0.5:
        return elapsed_ns // 2
    raise ValueError(f"playback_speed must be 0.5, 1.0, or 2.0, got {playback_speed}")
