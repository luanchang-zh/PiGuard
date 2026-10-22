"""MPU6050/6500/9250 angular velocity, using the detected full-scale range."""

import struct

from piguard_edge.drivers.polling import PollingSensor, hardware_sample
from piguard_edge.models.sample import GyroAxes, UNIT_DPS

_RANGES = {250: (0, 131), 500: (1, 65.5), 1000: (2, 32.8), 2000: (3, 16.4)}


class MpuGyroscope(PollingSensor):
    def __init__(self, bus_number, address, full_scale_dps, sample_hz=50, *, bus=None, start=True):
        if full_scale_dps not in _RANGES:
            raise ValueError("MPU full scale must be 250, 500, 1000 or 2000 dps")
        if bus is None:
            from smbus2 import SMBus
            bus = SMBus(bus_number)
        self._bus = bus
        self._address = address
        self._range, self._scale = _RANGES[full_scale_dps]
        self._initialized = False
        self.identity = None
        super().__init__(UNIT_DPS, sample_hz, start=start)

    def _measure(self):
        try:
            if not self._initialized:
                self.identity = self._bus.read_byte_data(self._address, 0x75)
                if self.identity not in (0x68, 0x70, 0x71):
                    raise ValueError(f"unsupported MPU identity: {self.identity:#x}")
                self._bus.write_byte_data(self._address, 0x6b, 1)
                self._bus.write_byte_data(self._address, 0x1b, self._range << 3)
                if self._stop.wait(.1):
                    raise TimeoutError("MPU stopped")
                self._initialized = True
            raw = self._bus.read_i2c_block_data(self._address, 0x43, 6)
            x, y, z = struct.unpack('>3h', bytes(raw))
            return hardware_sample(GyroAxes(x / self._scale, y / self._scale, z / self._scale), UNIT_DPS)
        except (OSError, ValueError, struct.error):
            self._initialized = False
            raise

    def _close_device(self):
        self._bus.close()
