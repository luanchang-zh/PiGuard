"""MQTT topic names. The device id in the topic and the payload must match."""


def telemetry_topic(device_id: str) -> str:
    return f"car/{device_id}/telemetry"


def status_topic(device_id: str) -> str:
    return f"car/{device_id}/status"


def commands_topic(device_id: str) -> str:
    return f"car/{device_id}/commands"


def command_acks_topic(device_id: str) -> str:
    return f"car/{device_id}/command-acks"


def events_topic(device_id: str) -> str:
    return f"car/{device_id}/events"
