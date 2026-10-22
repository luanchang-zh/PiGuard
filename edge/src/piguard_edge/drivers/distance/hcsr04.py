"""HC-SR04 with edge timestamps and an independent bounded sampling worker."""

import threading
import time

from piguard_edge.drivers.polling import PollingSensor, hardware_sample
from piguard_edge.models.sample import UNIT_M


class Hcsr04Distance(PollingSensor):
    def __init__(self, settings, sample_hz=10, gpio_chip=0, *, gpio=None, start=True):
        if gpio is None:
            import lgpio as gpio
        self._gpio, self._settings = gpio, settings
        self._edge = threading.Event()
        self._echo_lock = threading.Lock()
        self._waiting = False
        self._rise_ns = self._pulse_ns = None
        self._handle = gpio.gpiochip_open(gpio_chip)
        trigger_claimed = echo_claimed = False
        try:
            gpio.gpio_claim_output(self._handle, settings.trigger_bcm, 0); trigger_claimed = True
            gpio.gpio_claim_alert(self._handle, settings.echo_bcm, gpio.BOTH_EDGES); echo_claimed = True
            self._callback = gpio.callback(self._handle, settings.echo_bcm, gpio.BOTH_EDGES, self._on_edge)
        except Exception:
            if echo_claimed: gpio.gpio_free(self._handle, settings.echo_bcm)
            if trigger_claimed: gpio.gpio_free(self._handle, settings.trigger_bcm)
            gpio.gpiochip_close(self._handle)
            raise
        super().__init__(UNIT_M, sample_hz, start=start)

    def _on_edge(self, chip, gpio, level, tick):
        with self._echo_lock:
            if not self._waiting:
                return
            if level == 1:
                self._rise_ns = tick
            elif level == 0 and self._rise_ns is not None and tick > self._rise_ns:
                self._pulse_ns = tick - self._rise_ns
                self._edge.set()

    def _measure(self):
        settings = self._settings
        if self._gpio.gpio_read(self._handle, settings.echo_bcm):
            raise TimeoutError("HC-SR04 echo is stuck high")
        with self._echo_lock:
            self._edge.clear()
            self._rise_ns = self._pulse_ns = None
            self._waiting = True
        try:
            self._gpio.gpio_write(self._handle, settings.trigger_bcm, 1)
            time.sleep(.00001)
            self._gpio.gpio_write(self._handle, settings.trigger_bcm, 0)
            if not self._edge.wait(settings.pulse_timeout_ms / 1000):
                raise TimeoutError("HC-SR04 echo timeout")
            with self._echo_lock:
                pulse = self._pulse_ns
            if pulse is None or self._stop.is_set():
                raise TimeoutError("HC-SR04 stopped")
            distance = pulse / 1_000_000_000 * 343 / 2
            if not 0 < distance <= settings.max_distance_m:
                raise TimeoutError("HC-SR04 echo outside configured range")
            return hardware_sample(distance, UNIT_M)
        finally:
            with self._echo_lock:
                self._waiting = False
            self._gpio.gpio_write(self._handle, settings.trigger_bcm, 0)

    def _cancel_pending(self):
        self._edge.set()

    def _close_device(self):
        try:
            self._callback.cancel()
            self._gpio.gpio_write(self._handle, self._settings.trigger_bcm, 0)
        finally:
            try:
                self._gpio.gpio_free(self._handle, self._settings.echo_bcm)
                self._gpio.gpio_free(self._handle, self._settings.trigger_bcm)
            finally:
                self._gpio.gpiochip_close(self._handle)
