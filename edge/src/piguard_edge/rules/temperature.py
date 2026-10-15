"""High temperature with a lower recovery threshold."""

from piguard_edge.config.model import TemperatureRuleConfig
from piguard_edge.models.sample import Sample
from piguard_edge.rules.alarm import Alarm, Transition
from piguard_edge.rules.reading import as_float, classify

_NS_PER_MS = 1_000_000


class TemperatureRule:
    event_type = "high_temperature"

    def __init__(self, config: TemperatureRuleConfig, ttl_ms: int) -> None:
        self._config = config
        self._ttl_ms = ttl_ms
        self.alarm = Alarm()
        self.data: dict[str, object] = {}

    def update(self, sample: Sample[float] | None, now_ns: int) -> Transition | None:
        kind, _reason = classify(sample, now_ns, self._ttl_ms)
        value = as_float(sample.value) if kind == "ok" and sample is not None else None
        if value is None:
            self.alarm.note_invalid()
            return None
        self.data = {"temperature_c": value}
        if value >= self._config.trigger_c:
            target: str | None = "warning"
        elif value <= self._config.recover_c:
            target = "normal"
        else:
            target = None
        return self.alarm.consider(
            now_ns,
            target,
            enter_ns=self._config.duration_ms * _NS_PER_MS,
            recover_ns=self._config.recover_duration_ms * _NS_PER_MS,
        )
