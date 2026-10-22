"""Build selected sources; hardware dependencies are loaded only when selected."""

from contextlib import ExitStack

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
    capture_fps: int = 10,
    hardware=None,
    gpio_chip: int = 0,
    distance_hz: int = 10,
    temperature_hz: int = 1,
    gyro_hz: int = 50,
    stream_url: str = "",
    stream_token: str = "",
    stream_timeout_s: float = .7,
) -> Sources:
    with ExitStack() as cleanup:
        def own(source):
            close = getattr(source, "close", None) or getattr(source, "release", None)
            if close is not None:
                cleanup.callback(close)
            return source

        if distance_driver == "hcsr04" and hardware is not None:
            from piguard_edge.drivers.distance.hcsr04 import Hcsr04Distance
            distance = Hcsr04Distance(hardware.distance, distance_hz, gpio_chip)
        elif distance_driver == "mock":
            distance = MockDistanceSensor(scene_source, distance_scale)
        else:
            raise DriverUnavailable(f"distance driver {distance_driver} is unsupported or missing its hardware configuration")
        distance = own(distance)
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
        elif camera_driver == "network":
            from piguard_edge.drivers.camera.network import NetworkCamera
            camera = NetworkCamera(stream_url, stream_token, capture_fps=capture_fps, process_fps=process_fps,
                                   timeout_s=stream_timeout_s)
        else:
            raise DriverUnavailable(f"camera driver {camera_driver} is unsupported or missing its hardware configuration")
        camera = own(camera)
        if gyro_driver == "mpu" and hardware is not None:
            from piguard_edge.drivers.gyro.mpu import MpuGyroscope
            gyro = MpuGyroscope(hardware.i2c_bus, hardware.gyro.address, hardware.gyro.full_scale_dps, gyro_hz)
        elif gyro_driver == "mock":
            gyro = MockGyroscopeSensor(scene_source)
        else:
            raise DriverUnavailable(f"gyro driver {gyro_driver} is unsupported or missing its hardware configuration")
        gyro = own(gyro)
        if temperature_driver == "aht20" and hardware is not None:
            from piguard_edge.drivers.temperature.aht20 import Aht20Temperature
            temperature = Aht20Temperature(hardware.i2c_bus, hardware.temperature.aht20_address, temperature_hz)
        elif temperature_driver == "mock":
            temperature = MockTemperatureSensor(scene_source)
        else:
            raise DriverUnavailable(f"temperature driver {temperature_driver} is unsupported or missing its hardware configuration")
        temperature = own(temperature)
        if speed_driver != "scenario":
            raise DriverUnavailable(f"speed driver {speed_driver} is unsupported or missing its hardware configuration")
        result = Sources(
            distance=distance,
            camera=camera,
            gyro=gyro,
            temperature=temperature,
            speed=ScenarioSpeedProvider(scene_source),
        )
        cleanup.pop_all()
        return result
