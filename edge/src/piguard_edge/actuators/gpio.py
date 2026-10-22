"""Bounded buzzer tones on an exclusively claimed GPIO; lgpio is optional."""

import threading
import time


class GpioBuzzer:
    def __init__(self, chip: int, bcm: int, frequency_hz: int, active_high: bool, *, gpio=None) -> None:
        if gpio is None:
            import lgpio as gpio
        self._gpio = gpio
        self._bcm = bcm
        self._active = int(active_high)
        self._idle = 1 - self._active
        self._half_period = .5 / frequency_hz
        self._condition = threading.Condition()
        self._pattern = "off"
        self._until_ns = None
        self._epoch = time.monotonic()
        self._closed = False
        self.error: str | None = None
        self._handle = gpio.gpiochip_open(chip)
        try:
            gpio.gpio_claim_output(self._handle, bcm, self._idle)
        except Exception:
            gpio.gpiochip_close(self._handle)
            raise
        self._thread = threading.Thread(target=self._run, name="piguard-buzzer", daemon=True)
        self._thread.start()

    def set_pattern(self, pattern: str, until_ns: int | None = None) -> None:
        with self._condition:
            if self._closed:
                raise RuntimeError("GPIO buzzer is closed")
            if self.error:
                raise RuntimeError(self.error)
            if (pattern, until_ns) == (self._pattern, self._until_ns):
                return
            self._pattern, self._until_ns = pattern, until_ns
            self._epoch = time.monotonic()
            if pattern == "off":
                self._gpio.gpio_write(self._handle, self._bcm, self._idle)
            self._condition.notify_all()

    def _run(self) -> None:
        try:
            with self._condition:
                while not self._closed:
                    if self._until_ns is not None and time.monotonic_ns() >= self._until_ns:
                        self._pattern = "off"
                    elapsed = time.monotonic() - self._epoch
                    enabled = self._pattern == "on" or (
                        self._pattern in ("fast", "intermittent") and
                        elapsed % (.25 if self._pattern == "fast" else 1) < (.125 if self._pattern == "fast" else .2)
                    )
                    if not enabled:
                        self._gpio.gpio_write(self._handle, self._bcm, self._idle)
                        self._condition.wait(.02 if self._pattern != "off" else None)
                        continue
                    self._gpio.gpio_write(self._handle, self._bcm, self._active)
                    self._condition.wait(self._half_period)
                    self._gpio.gpio_write(self._handle, self._bcm, self._idle)
                    self._condition.wait(self._half_period)
        except Exception as exc:
            self.error = str(exc)
            try:
                self._gpio.gpio_write(self._handle, self._bcm, self._idle)
            except Exception:
                pass

    def close(self) -> None:
        with self._condition:
            if self._closed:
                return
            self._closed = True
            self._condition.notify_all()
        self._thread.join()
        try:
            self._gpio.gpio_write(self._handle, self._bcm, self._idle)
        finally:
            try:
                self._gpio.gpio_free(self._handle, self._bcm)
            finally:
                self._gpio.gpiochip_close(self._handle)
