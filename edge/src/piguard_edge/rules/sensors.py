"""One failure alarm per sensor. A missing sample is not a failure."""

from piguard_edge.rules.alarm import Alarm, Transition


class SensorWatch:
    def __init__(self, name: str) -> None:
        self.name = name
        self.alarm = Alarm()
        self.reason = ""

    def update(self, kind: str, reason: str, now_ns: int) -> Transition | None:
        if kind == "missing":
            self.alarm.note_invalid()
            return None
        if kind == "fault":
            self.reason = reason
            return self.alarm.consider(now_ns, "warning", enter_ns=0, recover_ns=0)
        return self.alarm.consider(now_ns, "normal", enter_ns=0, recover_ns=0)

    def event_data(self, transition: Transition) -> dict[str, object]:
        reason = "ok" if transition.action == "recovered" else self.reason
        return {"sensor": self.name, "reason": reason}
