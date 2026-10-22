"""AHT20 temperature and relative humidity with CRC and bounded conversion."""

import time

from piguard_edge.drivers.polling import PollingSensor, hardware_sample
from piguard_edge.models.sample import UNIT_C


def crc8(data):
    crc = 0xff
    for value in data:
        crc ^= value
        for _ in range(8):
            crc = ((crc << 1) ^ 0x31) & 255 if crc & 128 else (crc << 1) & 255
    return crc


def decode_measurement(raw):
    if len(raw) != 7 or crc8(raw[:6]) != raw[6]:
        raise ValueError("AHT20 CRC mismatch")
    if raw[0] & 128:
        raise TimeoutError("AHT20 is still busy")
    if not raw[0] & 8:
        raise ValueError("AHT20 calibration is not enabled")
    humidity = ((raw[1] << 12) | (raw[2] << 4) | (raw[3] >> 4)) * 100 / 1048576
    temperature = (((raw[3] & 15) << 16) | (raw[4] << 8) | raw[5]) * 200 / 1048576 - 50
    return temperature, humidity


class Aht20Temperature(PollingSensor):
    def __init__(self, bus_number, address, sample_hz=1, *, bus=None, messages=None, start=True):
        if bus is None:
            from smbus2 import SMBus, i2c_msg
            bus, messages = SMBus(bus_number), i2c_msg
        self._bus, self._messages, self._address = bus, messages, address
        self.humidity_rh = None
        super().__init__(UNIT_C, sample_hz, start=start)

    def _measure(self):
        if not self._bus.read_byte(self._address) & 8:
            self._bus.i2c_rdwr(self._messages.write(self._address, [0xbe, 0x08, 0]))
            if self._stop.wait(.01):
                raise TimeoutError("AHT20 stopped")
        self._bus.i2c_rdwr(self._messages.write(self._address, [0xac, 0x33, 0]))
        deadline = time.monotonic() + .2
        if self._stop.wait(.08):
            raise TimeoutError("AHT20 stopped")
        while not self._stop.is_set():
            message = self._messages.read(self._address, 7)
            self._bus.i2c_rdwr(message)
            raw = list(message)
            if not raw[0] & 128:
                temperature, humidity = decode_measurement(raw)
                self.humidity_rh = humidity
                return hardware_sample(temperature, UNIT_C)
            if time.monotonic() >= deadline:
                raise TimeoutError("AHT20 conversion timeout")
            self._stop.wait(.01)
        raise TimeoutError("AHT20 stopped")

    def _close_device(self):
        self._bus.close()
