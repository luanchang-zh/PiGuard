"""Parse a v1 command and encode one Ack. Success and failure do not share fields."""

import json
from dataclasses import dataclass
from datetime import datetime, timezone

from piguard_edge.models.sample import format_utc
from piguard_edge.simulation.scenarios import SCENARIO_NAMES

_COMMAND_FIELDS = {"schema_version", "command_id", "device_id", "type", "issued_at", "expires_at", "params"}
_SUPPORTED = {"buzzer.test", "indicator.test", "scenario.start", "scenario.stop", "camera.snapshot"}
_COLORS = {"green", "yellow", "red"}
_PLAYBACK = {0.5, 1.0, 2.0}


@dataclass(frozen=True, slots=True)
class DeviceCommand:
    command_id: str
    device_id: str
    command_type: str
    issued_at: datetime
    expires_at: datetime
    params: dict[str, object]
    fingerprint: str


@dataclass(frozen=True, slots=True)
class CommandProblem:
    code: str
    message: str
    command_id: str | None
    device_id: str | None
    fingerprint: str | None


def parse_command(topic: str, payload: bytes, *, device_id: str) -> DeviceCommand | CommandProblem | None:
    try:
        body = json.loads(payload.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        return None
    if not isinstance(body, dict):
        return None
    command_id = body.get("command_id")
    payload_device = body.get("device_id")
    if not isinstance(payload_device, str) or payload_device != device_id:
        return None
    if not _valid_id(command_id):
        return None
    if set(body) != _COMMAND_FIELDS or body.get("schema_version") != 1 or topic != f"car/{device_id}/commands":
        return _problem(command_id, device_id, body, "INVALID_PARAMS", "command identity or topic is invalid")
    try:
        issued_at = _time(body.get("issued_at"))
        expires_at = _time(body.get("expires_at"))
    except ValueError:
        return _problem(command_id, device_id, body, "INVALID_PARAMS", "issued_at and expires_at must be UTC timestamps")
    if expires_at <= issued_at:
        return _problem(command_id, device_id, body, "INVALID_PARAMS", "expires_at must be later than issued_at")
    command_type = body.get("type")
    params = body.get("params")
    if not isinstance(command_type, str) or not isinstance(params, dict):
        return _problem(command_id, device_id, body, "INVALID_PARAMS", "type and params are required")
    checked = _check_params(command_type, params)
    if isinstance(checked, str):
        code = "UNSUPPORTED_COMMAND" if checked == "unsupported" else "INVALID_PARAMS"
        message = "command type is not supported" if code == "UNSUPPORTED_COMMAND" else checked
        return _problem(command_id, device_id, body, code, message)
    return DeviceCommand(
        command_id,
        device_id,
        command_type,
        issued_at,
        expires_at,
        checked,
        _fingerprint(command_id, device_id, command_type, issued_at, expires_at, checked),
    )


def encode_success(command_id: str, device_id: str, executed_at: datetime, result: dict[str, object]) -> bytes:
    return _encode({
        "schema_version": 1,
        "command_id": command_id,
        "device_id": device_id,
        "status": "success",
        "executed_at": format_utc(executed_at),
        "result": result,
    })


def encode_failure(command_id: str, device_id: str, executed_at: datetime, code: str, message: str) -> bytes:
    return _encode({
        "schema_version": 1,
        "command_id": command_id,
        "device_id": device_id,
        "status": "failed",
        "executed_at": format_utc(executed_at),
        "error": {"code": code, "message": message},
    })


def _check_params(command_type: str, params: dict[str, object]) -> dict[str, object] | str:
    if command_type == "camera.snapshot":
        quality = params.get("quality", 80)
        if set(params) - {"quality"} or type(quality) is not int or not 1 <= quality <= 100:
            return "quality must be an integer from 1 to 100"
        return {"quality": quality}
    if command_type == "buzzer.test":
        duration = _duration(params, {"duration_ms"})
        return duration if isinstance(duration, str) else {"duration_ms": duration}
    if command_type == "indicator.test":
        if set(params) != {"color", "duration_ms"}:
            return "indicator.test params must be color and duration_ms"
        color = params.get("color")
        duration = _duration(params, {"color", "duration_ms"})
        if not isinstance(color, str) or color not in _COLORS:
            return "indicator color must be green, yellow, or red"
        if isinstance(duration, str):
            return duration
        return {"color": color, "duration_ms": duration}
    if command_type == "scenario.start":
        if set(params) != {"scenario", "speed"}:
            return "scenario.start params must be scenario and speed"
        scenario = params.get("scenario")
        speed = params.get("speed")
        if not isinstance(scenario, str) or scenario not in SCENARIO_NAMES:
            return "scenario must be one of the six named scenes"
        if isinstance(speed, bool) or not isinstance(speed, (int, float)) or float(speed) not in _PLAYBACK:
            return "playback speed must be 0.5, 1, or 2"
        return {"scenario": scenario, "speed": float(speed)}
    if command_type == "scenario.stop":
        if set(params) != set():
            return "scenario.stop params must be an empty object"
        return {}
    if command_type in _SUPPORTED:
        return "invalid command params"
    return "unsupported"


def _duration(params: dict[str, object], fields: set[str]) -> int | str:
    if set(params) != fields:
        return "duration_ms must be an integer from 100 to 5000"
    value = params.get("duration_ms")
    if isinstance(value, bool) or not isinstance(value, int) or value < 100 or value > 5000:
        return "duration_ms must be an integer from 100 to 5000"
    return value


def _problem(command_id: str, device_id: str, body: dict[str, object], code: str, message: str) -> CommandProblem:
    return CommandProblem(code, message, command_id, device_id, _raw_fingerprint(body))


def _fingerprint(
    command_id: str,
    device_id: str,
    command_type: str,
    issued_at: datetime,
    expires_at: datetime,
    params: dict[str, object],
) -> str:
    return _canonical({
        "schema_version": 1,
        "command_id": command_id,
        "device_id": device_id,
        "type": command_type,
        "issued_at": format_utc(issued_at),
        "expires_at": format_utc(expires_at),
        "params": params,
    })


def _raw_fingerprint(body: dict[str, object]) -> str:
    return _canonical(body)


def _canonical(body: dict[str, object]) -> str:
    return json.dumps(body, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def _valid_id(value: object) -> bool:
    return isinstance(value, str) and value.strip() != "" and len(value) <= 64 and value == value.strip()


def _time(value: object) -> datetime:
    if not isinstance(value, str):
        raise ValueError("timestamp must be a string")
    parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    if parsed.utcoffset() is None:
        raise ValueError("timestamp must be timezone-aware")
    return parsed.astimezone(timezone.utc)


def _encode(body: dict[str, object]) -> bytes:
    return json.dumps(body, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
