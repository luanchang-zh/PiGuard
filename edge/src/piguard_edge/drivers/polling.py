"""Keep slow physical conversions out of the sampling/command loop."""

import threading
import time
from datetime import datetime, timezone

from piguard_edge.drivers.base import stamped
from piguard_edge.models.sample import Sample, SampleStatus


def hardware_sample(value, unit, status=SampleStatus.OK):
    return Sample(value, datetime.now(timezone.utc), time.monotonic_ns(), "hardware", status, unit)


class PollingSensor:
    source = "hardware"
    def __init__(self, unit, sample_hz, *, start=True):
        self._unit = unit
        self._period = 1 / sample_hz
        self._stop = threading.Event()
        self._lock = threading.Lock()
        self._latest = None
        self.last_error = None
        self._thread = None
        self._closed = False
        if start:
            self._thread = threading.Thread(target=self._run, name=type(self).__name__, daemon=True)
            self._thread.start()

    def _run(self):
        while not self._stop.is_set():
            began = time.monotonic()
            try:
                sample = self._measure()
                self.last_error = None
            except Exception as exc:
                self.last_error = str(exc)
                status = SampleStatus.TIMEOUT if isinstance(exc, TimeoutError) else SampleStatus.ERROR
                sample = hardware_sample(None, self._unit, status)
            with self._lock:
                self._latest = sample
            self._stop.wait(max(0, self._period - (time.monotonic() - began)))

    def read(self, clock):
        with self._lock:
            sample = self._latest
        if sample is not None:
            return sample
        at, ns = stamped(clock)
        return Sample(None, at, ns, "hardware", SampleStatus.UNAVAILABLE, self._unit)

    def close(self):
        if self._closed:
            return
        self._closed = True
        self._stop.set()
        self._cancel_pending()
        if self._thread is not None:
            self._thread.join()
        self._close_device()

    def _cancel_pending(self):
        pass

    def _close_device(self):
        pass
