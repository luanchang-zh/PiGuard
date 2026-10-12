"""Shared alarm phases. Pending time uses the monotonic clock."""

from dataclasses import dataclass


@dataclass(frozen=True, slots=True)
class Transition:
    action: str
    level: str


class Alarm:
    def __init__(self) -> None:
        self.phase = "normal"
        self.level: str | None = None
        self.since_ns: int | None = None
        self.usable = False

    @property
    def active(self) -> bool:
        return self.phase in ("active", "recovering")

    def note_invalid(self) -> None:
        self.usable = False
        if self.phase == "pending":
            self.phase = "normal"
            self.level = None
            self.since_ns = None
        elif self.phase == "recovering":
            self.phase = "active"
            self.since_ns = None

    def consider(
        self,
        now_ns: int,
        target: str | None,
        *,
        enter_ns: int,
        recover_ns: int,
    ) -> Transition | None:
        self.usable = True
        if target is None:
            return self._hold()
        if target not in ("normal", "warning", "danger"):
            raise ValueError(f"unknown alarm target: {target}")
        if not self.active:
            return self._approach(now_ns, target, enter_ns)
        if target != "normal":
            return self._retarget(target)
        return self._clear(now_ns, recover_ns)

    def _hold(self) -> None:
        if self.phase == "pending":
            self.phase = "normal"
            self.level = None
            self.since_ns = None
        elif self.phase == "recovering":
            self.phase = "active"
            self.since_ns = None
        return None

    def _approach(self, now_ns: int, target: str, enter_ns: int) -> Transition | None:
        if target == "normal":
            self.phase = "normal"
            self.level = None
            self.since_ns = None
            return None
        if self.phase != "pending" or self.level != target:
            self.phase = "pending"
            self.level = target
            self.since_ns = now_ns
        since_ns = self.since_ns if self.since_ns is not None else now_ns
        if now_ns - since_ns < enter_ns:
            return None
        self.phase = "active"
        self.level = target
        self.since_ns = None
        return Transition("started", target)

    def _retarget(self, target: str) -> Transition | None:
        if self.phase == "recovering":
            self.phase = "active"
            self.since_ns = None
        if target == self.level:
            return None
        self.level = target
        self.phase = "active"
        self.since_ns = None
        return Transition("level_changed", target)

    def _clear(self, now_ns: int, recover_ns: int) -> Transition | None:
        if self.level is None:
            raise RuntimeError("an active alarm needs a level")
        if recover_ns > 0 and self.phase != "recovering":
            self.phase = "recovering"
            self.since_ns = now_ns
            return None
        if recover_ns > 0:
            since_ns = self.since_ns if self.since_ns is not None else now_ns
            if now_ns - since_ns < recover_ns:
                return None
        previous = self.level
        self.phase = "normal"
        self.level = None
        self.since_ns = None
        return Transition("recovered", previous)
