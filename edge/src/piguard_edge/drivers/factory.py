"""Build sensor sources, including a camera shared by another computer."""

from piguard_edge.config.model import CameraConfig
from piguard_edge.drivers.base import SceneSource, Sources
from piguard_edge.drivers.camera.remote import RemoteCamera
from piguard_edge.drivers.camera.synthetic import SyntheticCamera
from piguard_edge.drivers.camera.video import VideoCamera
from piguard_edge.drivers.distance.mock import MockDistanceSensor
from piguard_edge.drivers.gyro.mock import MockGyroscopeSensor
from piguard_edge.drivers.speed.scenario import ScenarioSpeedProvider
from piguard_edge.drivers.temperature.mock import MockTemperatureSensor


class DriverUnavailable(RuntimeError):
    pass


def build_sources(
    scene_source: SceneSource,
    *,
    distance_scale: float,
    distance_driver: str,
    camera_driver: str,
    gyro_driver: str,
    temperature_driver: str,
    speed_driver: str,
    video_path: str = "",
    width: int = 640,
    height: int = 480,
    process_fps: int | None = None,
    camera_config: CameraConfig | None = None,
) -> Sources:
    if distance_driver != "mock":
        raise DriverUnavailable(f"distance driver {distance_driver} is not available in mock mode")
    if gyro_driver != "mock":
        raise DriverUnavailable(f"gyro driver {gyro_driver} is not available in mock mode")
    if temperature_driver != "mock":
        raise DriverUnavailable(f"temperature driver {temperature_driver} is not available in mock mode")
    if speed_driver != "scenario":
        raise DriverUnavailable(f"speed driver {speed_driver} is not available in mock mode")
    if camera_driver == "synthetic":
        camera = SyntheticCamera(scene_source, width=width, height=height, process_fps=process_fps)
    elif camera_driver == "video":
        if not video_path:
            raise DriverUnavailable("video driver needs sensors.camera.video_path")
        camera = VideoCamera(video_path, process_fps=process_fps)
    elif camera_driver == "remote":
        if camera_config is None:
            raise DriverUnavailable("remote driver needs sensors.camera configuration")
        camera = RemoteCamera(camera_config)
    else:
        raise DriverUnavailable(f"camera driver {camera_driver} is not available in mock mode")
    return Sources(
        distance=MockDistanceSensor(scene_source, distance_scale),
        camera=camera,
        gyro=MockGyroscopeSensor(scene_source),
        temperature=MockTemperatureSensor(scene_source),
        speed=ScenarioSpeedProvider(scene_source),
    )
