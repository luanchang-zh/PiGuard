"""Exercise register values and edge timing with fake buses/GPIO only."""

import struct
import threading
from pathlib import Path
from types import SimpleNamespace

import pytest
import yaml

from piguard_edge.config.loader import ConfigError, load_config
from piguard_edge.config.model import DistanceHardware
from piguard_edge.drivers.distance.hcsr04 import Hcsr04Distance
from piguard_edge.drivers.gyro.mpu import MpuGyroscope
from piguard_edge.drivers.polling import PollingSensor, hardware_sample
from piguard_edge.drivers.temperature.aht20 import Aht20Temperature, crc8, decode_measurement
from piguard_edge.models.sample import SampleStatus
from piguard_edge.protocol.telemetry import build_telemetry
from piguard_edge.runtime.clock import SystemClock
from piguard_edge.runtime.workers import Sampler
from piguard_edge.simulation.engine import ScenarioEngine


class MpuBus:
    identity = 0x70
    closed = False

    def __init__(self):
        self.writes = []

    def read_byte_data(self, address, register):
        assert (address, register) == (0x68, 0x75)
        return self.identity

    def write_byte_data(self, address, register, value):
        self.writes.append((address, register, value))

    def read_i2c_block_data(self, address, register, count):
        assert (address, register, count) == (0x68, 0x43, 6)
        return list(struct.pack('>3h', 655, -1310, 0))

    def close(self):
        self.closed = True


@pytest.mark.parametrize('identity', [0x68, 0x70, 0x71])
def test_mpu_signed_scaling_initialization_and_cleanup(identity):
    bus = MpuBus()
    bus.identity = identity
    sensor = MpuGyroscope(1, 0x68, 500, bus=bus, start=False)
    try:
        result = sensor._measure()
        assert (result.value.x, result.value.y, result.value.z) == (10, -20, 0)
        assert result.source == 'hardware'
        assert bus.writes == [(0x68, 0x6b, 1), (0x68, 0x1b, 8)]
        sensor._measure()
        assert len(bus.writes) == 2
    finally:
        sensor.close()
    assert bus.closed


def test_mpu_unsupported_identity_is_a_fault_instead_of_zero():
    bus = MpuBus()
    bus.identity = 0xff
    entered = threading.Event()
    read = bus.read_byte_data
    def read_once(*args):
        entered.set()
        return read(*args)
    bus.read_byte_data = read_once
    sensor = MpuGyroscope(1, 0x68, 500, bus=bus)
    try:
        assert entered.wait(1)
        sensor._stop.set()
        sensor._thread.join()
        sample = sensor.read(SystemClock())
        assert sample.status is SampleStatus.ERROR
        assert sample.value is None
        assert 'identity' in sensor.last_error
    finally:
        sensor.close()


def aht_packet(status=0x18):
    data = [status, 0x80, 0, 0x06, 0, 0]  # 50% RH, 25 C
    return data + [crc8(data)]


def test_aht20_decodes_and_checks_crc_and_status():
    assert decode_measurement(aht_packet()) == (25, 50)
    corrupt = aht_packet()
    corrupt[2] ^= 1
    with pytest.raises(ValueError, match='CRC'):
        decode_measurement(corrupt)
    with pytest.raises(TimeoutError, match='busy'):
        decode_measurement(aht_packet(0x98))
    with pytest.raises(ValueError, match='calibration'):
        decode_measurement(aht_packet(0))


class AhtMessages:
    @staticmethod
    def write(address, data):
        return ('write', address, data)

    @staticmethod
    def read(address, size):
        assert address == 0x38 and size == 7
        return [0] * size


class AhtBus:
    closed = False
    calibrated = False
    busy = False

    def __init__(self):
        self.writes = []

    def read_byte(self, address):
        return 8 if self.calibrated else 0

    def i2c_rdwr(self, message):
        if isinstance(message, tuple):
            self.writes.append(message[2])
        else:
            message[:] = aht_packet(0x98 if self.busy else 0x18)

    def close(self):
        self.closed = True


def test_aht20_initializes_and_reads_real_temperature_and_humidity():
    bus = AhtBus()
    sensor = Aht20Temperature(1, 0x38, bus=bus, messages=AhtMessages, start=False)
    try:
        sample = sensor._measure()
        assert sample.value == 25 and sensor.humidity_rh == 50
        assert sample.source == 'hardware'
        assert bus.writes == [[0xbe, 8, 0], [0xac, 0x33, 0]]
    finally:
        sensor.close()
    assert bus.closed


def test_aht20_busy_conversion_has_a_deadline():
    bus = AhtBus()
    bus.busy = True
    sensor = Aht20Temperature(1, 0x38, bus=bus, messages=AhtMessages, start=False)
    try:
        with pytest.raises(TimeoutError, match='conversion'):
            sensor._measure()
    finally:
        sensor.close()


class EchoGpio:
    BOTH_EDGES = 3
    stuck = False
    pulse_ns = 2_000_000  # 0.343 metres
    closed = False
    cancelled = False

    def __init__(self):
        self.freed, self.writes = [], []

    def gpiochip_open(self, chip): return 7
    def gpio_claim_output(self, handle, pin, value): pass
    def gpio_claim_alert(self, handle, pin, edges): pass
    def callback(self, handle, pin, edges, callback):
        self.on_edge = callback
        return SimpleNamespace(cancel=lambda: setattr(self, 'cancelled', True))
    def gpio_read(self, handle, pin): return int(self.stuck)
    def gpio_write(self, handle, pin, value):
        self.writes.append(value)
        if value and self.pulse_ns is not None:
            self.on_edge(0, 24, 1, 1_000_000)
            self.on_edge(0, 24, 0, 1_000_000 + self.pulse_ns)
    def gpio_free(self, handle, pin): self.freed.append(pin)
    def gpiochip_close(self, handle): self.closed = True


def test_hcsr04_uses_edge_timestamps_and_releases_claims():
    gpio = EchoGpio()
    sensor = Hcsr04Distance(DistanceHardware(23, 24, 4, 1), gpio=gpio, start=False)
    try:
        assert sensor._measure().value == pytest.approx(.343)
    finally:
        sensor.close()
    assert gpio.cancelled and gpio.closed and sorted(gpio.freed) == [23, 24]
    assert gpio.writes[-1] == 0


@pytest.mark.parametrize('pulse,stuck', [(None, False), (100_000_000, False), (None, True)])
def test_hcsr04_missing_echo_out_of_range_and_stuck_high(pulse, stuck):
    gpio = EchoGpio()
    gpio.pulse_ns, gpio.stuck = pulse, stuck
    sensor = Hcsr04Distance(DistanceHardware(23, 24, 4, 1), gpio=gpio, start=False)
    try:
        with pytest.raises(TimeoutError): sensor._measure()
    finally:
        sensor.close()


def test_background_io_does_not_block_sampler_and_keeps_old_sample_time(example_config):
    entered, resume = threading.Event(), threading.Event()

    class SlowSensor(PollingSensor):
        def _measure(self):
            entered.set()
            resume.wait(2)
            return hardware_sample(25, 'C')

    sensor = SlowSensor('C', 1)
    try:
        assert entered.wait(1)
        assert sensor.read(SystemClock()).status is SampleStatus.UNAVAILABLE
        sample = hardware_sample(23, 'C')
        with sensor._lock: sensor._latest = sample
        assert sensor.read(SystemClock()) is sample
        assert sensor.read(SystemClock()).monotonic_ns == sample.monotonic_ns
    finally:
        resume.set()
        sensor.close()


def test_real_sources_flow_through_mapping_yaw_and_telemetry(example_config):
    clock = SystemClock()
    sampler = Sampler.from_config(example_config, ScenarioEngine('normal_drive', 1), clock)
    sampler.sources.distance = SimpleNamespace(read=lambda _: hardware_sample(.2, 'm'))
    from piguard_edge.models.sample import GyroAxes
    sampler.sources.gyro = SimpleNamespace(read=lambda _: hardware_sample(GyroAxes(1, 2, -3), 'deg/s'))
    sampler.sources.temperature = SimpleNamespace(read=lambda _: hardware_sample(26.5, 'C'))
    for name in sampler._next_ns: sampler._next_ns[name] = 0
    sampler.step()
    data = build_telemetry(sampler.state, device_id='car-001', timestamp=clock.utc(), seq=0,
                           message_id='real-fake', distance_scale=100, source='hardware')
    assert data['distance']['raw_value'] == .2
    assert data['distance']['mapped_value'] == 20
    assert data['gyro']['yaw_rate'] == -3
    assert data['temperature']['value'] == 26.5
    assert all(data[name]['source'] == 'hardware' for name in ('distance', 'gyro', 'temperature'))
    assert data['speed']['source'] == 'mock'
    sampler.close()


def test_connected_config_selects_hardware_and_usb():
    path = Path(__file__).resolve().parents[2] / 'config/raspberrypi-connected-demo.yaml'
    config = load_config(path)
    assert config.device.mode == 'hardware'
    assert config.mqtt.host == '10.12.194.2'
    assert config.sensors.camera.driver == 'network'
    assert config.sensors.distance.driver == 'hcsr04'
    assert config.sensors.gyro.driver == 'mpu'
    assert config.sensors.temperature.driver == 'aht20'
    assert config.actuators.buzzer_bcm == 17


@pytest.mark.parametrize('section,key,value', [
    ('camera', 'stream_timeout_s', 10), ('camera', 'stream_timeout_s', float('nan')),
    ('gyro', 'full_scale_dps', 123), ('distance', 'echo_bcm', 23),
])
def test_invalid_hardware_configuration_is_rejected(example_config, tmp_path, section, key, value):
    data = yaml.safe_load(example_config.source_path.read_text())
    root = data['sensors'] if section == 'camera' else data['hardware']
    root[section][key] = value
    target = tmp_path / 'bad.yaml'
    target.write_text(yaml.safe_dump(data))
    with pytest.raises(ConfigError): load_config(target)
