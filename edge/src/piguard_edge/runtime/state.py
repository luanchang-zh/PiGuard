"""Latest sample of each source. Readers take a snapshot before use."""

from dataclasses import dataclass

from piguard_edge.models.frame import Frame
from piguard_edge.models.sample import DistanceReading, GyroReading, Sample


@dataclass
class LatestState:
    distance: Sample[DistanceReading] | None = None
    speed: Sample[float] | None = None
    gyro: Sample[GyroReading] | None = None
    temperature: Sample[float] | None = None
    frame: Frame | None = None

    def snapshot(self) -> "LatestState":
        return LatestState(
            distance=self.distance,
            speed=self.speed,
            gyro=self.gyro,
            temperature=self.temperature,
            frame=self.frame,
        )
