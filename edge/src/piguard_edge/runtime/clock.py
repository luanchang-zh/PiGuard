"""Monotonic clocks. Tests inject FakeClock; the process uses SystemClock."""

import time
from datetime import datetime, timedelta, timezone


class FakeClock:
    def __init__(self, monotonic_ns: int = 0, epoch: datetime | None = None) -> None:
        if epoch is not None and epoch.utcoffset() is None:
            raise ValueError("epoch must be timezone-aware UTC")
        self._ns = monotonic_ns
        self._epoch = epoch or datetime(2026, 1, 1, tzinfo=timezone.utc)

    def monotonic_ns(self) -> int:
        return self._ns

    def utc(self) -> datetime:
        return self._epoch + timedelta(microseconds=self._ns // 1000)

    def advance_ns(self, nanoseconds: int) -> None:
        self._ns += nanoseconds


class SystemClock:
    def monotonic_ns(self) -> int:
        return time.monotonic_ns()

    def utc(self) -> datetime:
        return datetime.now(timezone.utc)
