"""Source interfaces. Hardware libraries stay out of this module."""

from datetime import datetime
from typing import Protocol

from piguard_edge.models.frame import Frame
from piguard_edge.models.sample import GyroAxes, Sample
from piguard_edge.simulation.scenarios import Scene


class Clock(Protocol):
    def monotonic_ns(self) -> int: ...

    def utc(self) -> datetime: ...


class SceneSource(Protocol):
    def observe(self, monotonic_ns: int) -> Scene: ...


class DistanceSensor(Protocol):
    def read(self, clock: Clock) -> Sample[float]: ...


class CameraSource(Protocol):
    def read(self, clock: Clock) -> Frame: ...


class GyroscopeSensor(Protocol):
    def read(self, clock: Clock) -> Sample[GyroAxes]: ...


class TemperatureSensor(Protocol):
    def read(self, clock: Clock) -> Sample[float]: ...


class SpeedProvider(Protocol):
    def read(self, clock: Clock) -> Sample[float]: ...


class Sources:
    def __init__(
        self,
        distance: DistanceSensor,
        camera: CameraSource,
        gyro: GyroscopeSensor,
        temperature: TemperatureSensor,
        speed: SpeedProvider,
    ) -> None:
        self.distance = distance
        self.camera = camera
        self.gyro = gyro
        self.temperature = temperature
        self.speed = speed


def stamped(clock: Clock) -> tuple[datetime, int]:
    return clock.utc(), clock.monotonic_ns()
