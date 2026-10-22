"""Sample each source on its own period. One failure does not stop the others."""

import threading
from collections.abc import Callable

from piguard_edge.config.model import EdgeConfig
from piguard_edge.drivers.base import Clock, Sources
from piguard_edge.drivers.factory import build_sources
from piguard_edge.models.frame import Frame, invalid_lane
from piguard_edge.models.sample import (
    SOURCE_MOCK,
    UNIT_C,
    UNIT_DPS,
    UNIT_KMH,
    UNIT_M,
    DistanceReading,
    GyroAxes,
    GyroReading,
    Sample,
    SampleStatus,
)
from piguard_edge.processing.distance import map_distance
from piguard_edge.processing.gyro import apply_yaw
from piguard_edge.runtime.state import LatestState
from piguard_edge.simulation.engine import ScenarioEngine

_NS_PER_SECOND = 1_000_000_000


class Sampler:
    def __init__(
        self,
        sources: Sources,
        clock: Clock,
        periods_ns: dict[str, int],
        distance_scale: float,
        yaw_axis: str,
        yaw_sign: int,
    ) -> None:
        self.sources = sources
        self._clock = clock
        self._periods_ns = periods_ns
        self._distance_scale = distance_scale
        self._yaw_axis = yaw_axis
        self._yaw_sign = yaw_sign
        now = clock.monotonic_ns()
        self._next_ns = {name: now + period for name, period in periods_ns.items()}
        self.state = LatestState()

    @classmethod
    def from_config(cls, config: EdgeConfig, engine: ScenarioEngine, clock: Clock) -> "Sampler":
        if config.device.mode not in ("mock", "hardware"):
            raise ValueError("sampler supports mock or hardware mode")
        sources = build_sources(
            engine,
            distance_scale=config.mapping.distance_scale,
            distance_driver=config.sensors.distance.driver,
            camera_driver=config.sensors.camera.driver,
            gyro_driver=config.sensors.gyro.driver,
            temperature_driver=config.sensors.temperature.driver,
            speed_driver=config.speed.driver,
            video_path=config.sensors.camera.video_path,
            width=config.sensors.camera.width,
            height=config.sensors.camera.height,
            process_fps=config.sensors.camera.process_fps,
            camera_config=config.sensors.camera,
            capture_fps=config.sensors.camera.capture_fps,
            hardware=config.hardware,
            gpio_chip=config.actuators.gpio_chip,
            distance_hz=config.sensors.distance.sample_hz,
            temperature_hz=config.sensors.temperature.sample_hz,
            gyro_hz=config.sensors.gyro.sample_hz,
            stream_url=config.sensors.camera.stream_url,
            stream_token=config.sensors.camera.stream_token,
            stream_timeout_s=config.sensors.camera.stream_timeout_s,
        )
        return cls(
            sources,
            clock,
            periods_ns=_periods_ns(config),
            distance_scale=config.mapping.distance_scale,
            yaw_axis=config.hardware.gyro.yaw_axis,
            yaw_sign=config.hardware.gyro.yaw_sign,
        )

    def step(self) -> None:
        now = self._clock.monotonic_ns()
        for name in ("distance", "speed", "gyro", "temperature", "camera"):
            if now < self._next_ns[name]:
                continue
            self._sample(name)
            period = self._periods_ns[name]
            self._next_ns[name] += period
            if self._next_ns[name] <= now:
                self._next_ns[name] = now + period

    def close(self) -> None:
        first_error = None
        for source in (self.sources.camera, self.sources.distance, self.sources.gyro,
                       self.sources.temperature, self.sources.speed):
            close = getattr(source, "close", None) or getattr(source, "release", None)
            if close is not None:
                try:
                    close()
                except Exception as exc:
                    first_error = first_error or exc
        if first_error is not None:
            raise first_error

    def _sample(self, name: str) -> None:
        try:
            if name == "distance":
                self._sample_distance()
            elif name == "speed":
                self._sample_speed()
            elif name == "gyro":
                self._sample_gyro()
            elif name == "temperature":
                self._sample_temperature()
            elif name == "camera":
                self._sample_camera()
        except Exception:
            self._store_error(name)

    def _sample_distance(self) -> None:
        raw = self.sources.distance.read(self._clock)
        if raw.status is not SampleStatus.OK or raw.value is None:
            self.state.distance = _empty(raw)
            return
        mapped = map_distance(raw.value, self._distance_scale)
        self.state.distance = _with_value(raw, DistanceReading(raw.value, mapped, self._distance_scale))

    def _sample_speed(self) -> None:
        sample = self.sources.speed.read(self._clock)
        self.state.speed = sample if sample.status is SampleStatus.OK else _empty(sample)

    def _sample_gyro(self) -> None:
        raw = self.sources.gyro.read(self._clock)
        if raw.status is not SampleStatus.OK or raw.value is None:
            self.state.gyro = _empty(raw)
            return
        axes: GyroAxes = raw.value
        rate = apply_yaw(axes.x, axes.y, axes.z, self._yaw_axis, self._yaw_sign)
        self.state.gyro = _with_value(raw, GyroReading(axes.x, axes.y, axes.z, rate))

    def _sample_temperature(self) -> None:
        sample = self.sources.temperature.read(self._clock)
        self.state.temperature = sample if sample.status is SampleStatus.OK else _empty(sample)

    def _sample_camera(self) -> None:
        frame = self.sources.camera.read(self._clock)
        if frame.status is not SampleStatus.OK:
            frame = Frame(
                frame.sample_at,
                frame.monotonic_ns,
                frame.source,
                frame.status,
                None,
                invalid_lane(frame.sample_at),
            )
        self.state.frame = frame

    def _store_error(self, name: str) -> None:
        source = getattr(getattr(self.sources, name), "source", SOURCE_MOCK)
        sample_at = self._clock.utc()
        monotonic_ns = self._clock.monotonic_ns()
        if name == "camera":
            self.state.frame = Frame(
                sample_at,
                monotonic_ns,
                source,
                SampleStatus.ERROR,
                None,
                invalid_lane(sample_at),
            )
            return
        units = {
            "distance": UNIT_M,
            "speed": UNIT_KMH,
            "gyro": UNIT_DPS,
            "temperature": UNIT_C,
        }
        failed: Sample[None] = Sample(
            None,
            sample_at,
            monotonic_ns,
            source,
            SampleStatus.ERROR,
            units[name],
        )
        if name == "distance":
            self.state.distance = failed
        elif name == "speed":
            self.state.speed = failed
        elif name == "gyro":
            self.state.gyro = failed
        else:
            self.state.temperature = failed


class SamplingLoop:
    """Thread shell around Sampler.step. Tests call step directly."""

    def __init__(
        self,
        sampler: Sampler,
        sleep: Callable[[float], None],
        tick_s: float = 0.005,
    ) -> None:
        self._sampler = sampler
        self._sleep = sleep
        self._tick_s = tick_s
        self._stop = threading.Event()

    def run_once(self) -> None:
        self._sampler.step()

    def stop(self) -> None:
        self._stop.set()

    def run(self) -> None:
        while not self._stop.is_set():
            self.run_once()
            self._sleep(self._tick_s)


def _periods_ns(config: EdgeConfig) -> dict[str, int]:
    return {
        "distance": _hz_to_ns(config.sensors.distance.sample_hz),
        "speed": _hz_to_ns(config.speed.sample_hz),
        "gyro": _hz_to_ns(config.sensors.gyro.sample_hz),
        "temperature": _hz_to_ns(config.sensors.temperature.sample_hz),
        "camera": _hz_to_ns(config.sensors.camera.capture_fps),
    }


def _hz_to_ns(hz: int) -> int:
    return _NS_PER_SECOND // hz


def _empty(sample: Sample[object]) -> Sample[None]:
    return Sample(None, sample.sample_at, sample.monotonic_ns, sample.source, sample.status, sample.unit)


def _with_value(sample: Sample[object], value: object) -> Sample[object]:
    return Sample(value, sample.sample_at, sample.monotonic_ns, sample.source, sample.status, sample.unit)
