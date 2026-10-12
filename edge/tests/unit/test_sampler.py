from dataclasses import replace
from datetime import datetime, timedelta, timezone

from piguard_edge.models.sample import UNIT_M, Sample, SampleStatus
from piguard_edge.runtime.clock import FakeClock
from piguard_edge.runtime.workers import Sampler, SamplingLoop
from piguard_edge.simulation.engine import ScenarioEngine
from piguard_edge.simulation.scenarios import normal_scene

EPOCH = datetime(2001, 2, 3, tzinfo=timezone.utc)
_STEP_NS = 1_000_000


class _Counting:
    def __init__(self, inner) -> None:
        self.inner = inner
        self.calls = 0

    def read(self, clock):
        self.calls += 1
        return self.inner.read(clock)


class _TimeoutZero:
    def read(self, clock):
        return Sample(0.0, clock.utc(), clock.monotonic_ns(), "mock", SampleStatus.TIMEOUT, UNIT_M)


class _Boom:
    def read(self, clock):
        raise RuntimeError("distance disconnected")


def test_one_second_matches_the_sample_rates(example_config) -> None:
    clock = FakeClock(epoch=EPOCH)
    engine = ScenarioEngine("normal_drive", 1.0)
    sampler = Sampler.from_config(example_config, engine, clock)
    counted = {
        "distance": _Counting(sampler.sources.distance),
        "speed": _Counting(sampler.sources.speed),
        "gyro": _Counting(sampler.sources.gyro),
        "temperature": _Counting(sampler.sources.temperature),
        "camera": _Counting(sampler.sources.camera),
    }
    sampler.sources.distance = counted["distance"]
    sampler.sources.speed = counted["speed"]
    sampler.sources.gyro = counted["gyro"]
    sampler.sources.temperature = counted["temperature"]
    sampler.sources.camera = counted["camera"]

    for _ in range(1000):
        clock.advance_ns(_STEP_NS)
        sampler.step()

    assert counted["distance"].calls == 10
    assert counted["speed"].calls == 10
    assert counted["gyro"].calls == 50
    assert counted["temperature"].calls == 1
    assert counted["camera"].calls == 10
    assert sampler.state.temperature is not None
    assert sampler.state.temperature.monotonic_ns == 1_000_000_000
    assert sampler.state.temperature.sample_at == EPOCH + timedelta(seconds=1)
    assert sampler.state.distance is not None
    assert sampler.state.distance.value is not None
    assert sampler.state.distance.value.raw_value == 0.5
    assert sampler.state.distance.value.mapped_value == 50


def test_timeout_zero_is_stored_empty(example_config) -> None:
    clock = FakeClock(epoch=EPOCH)
    sampler = Sampler.from_config(example_config, ScenarioEngine("normal_drive", 1.0), clock)
    sampler.sources.distance = _TimeoutZero()
    clock.advance_ns(100_000_000)
    sampler.step()

    assert sampler.state.distance is not None
    assert sampler.state.distance.status is SampleStatus.TIMEOUT
    assert sampler.state.distance.value is None


def test_one_failed_source_does_not_stop_the_others(example_config) -> None:
    clock = FakeClock(epoch=EPOCH)
    sampler = Sampler.from_config(example_config, ScenarioEngine("normal_drive", 1.0), clock)
    sampler.sources.distance = _Boom()
    clock.advance_ns(100_000_000)
    sampler.step()

    assert sampler.state.distance is not None
    assert sampler.state.distance.status is SampleStatus.ERROR
    assert sampler.state.distance.value is None
    assert sampler.state.speed is not None
    assert sampler.state.speed.value == 30
    assert sampler.state.gyro is not None
    assert sampler.state.gyro.value is not None
    assert sampler.state.gyro.value.yaw_rate == 2


def test_sampler_uses_configured_yaw_axis(example_config, tmp_path) -> None:
    target = tmp_path / "default.example.yaml"
    target.write_text(
        example_config.source_path.read_text(encoding="utf-8"),
        encoding="utf-8",
    )
    (tmp_path / "local.yaml").write_text("hardware:\n  gyro:\n    yaw_axis: x\n", encoding="utf-8")
    from piguard_edge.config.loader import load_config

    loaded = load_config(target)
    clock = FakeClock(epoch=EPOCH)
    sampler = Sampler.from_config(loaded, ScenarioEngine("normal_drive", 1.0), clock)
    clock.advance_ns(20_000_000)
    sampler.step()

    assert sampler.state.gyro is not None
    assert sampler.state.gyro.value is not None
    assert sampler.state.gyro.value.z == 2
    assert sampler.state.gyro.value.yaw_rate == 0
    assert not hasattr(sampler.sources.gyro, "address")


def test_zero_speed_scene_stays_zero(example_config) -> None:
    clock = FakeClock(epoch=EPOCH)
    engine = _Fixed(replace(normal_scene(), speed_kmh=0.0))
    sampler = Sampler.from_config(example_config, engine, clock)
    clock.advance_ns(100_000_000)
    sampler.step()

    assert sampler.state.speed is not None
    assert sampler.state.speed.value == 0.0


def test_loop_calls_the_same_step(example_config) -> None:
    clock = FakeClock(epoch=EPOCH)
    sampler = Sampler.from_config(example_config, ScenarioEngine("normal_drive", 1.0), clock)
    seen: list[int] = []
    original = sampler.step

    def wrapped() -> None:
        seen.append(clock.monotonic_ns())
        original()

    sampler.step = wrapped  # type: ignore[method-assign]
    loop = SamplingLoop(sampler, sleep=lambda _seconds: loop.stop(), tick_s=0)
    loop.run()

    assert seen == [0]


class _Fixed:
    def __init__(self, scene) -> None:
        self._scene = scene

    def observe(self, monotonic_ns: int):
        return self._scene
