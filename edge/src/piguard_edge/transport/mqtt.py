"""MQTT session. Tests use FakeMqtt; the process uses paho-mqtt 2.1."""

import threading
from dataclasses import dataclass

from piguard_edge.config.model import EdgeConfig


@dataclass(frozen=True, slots=True)
class MqttMessage:
    topic: str
    payload: bytes
    qos: int
    retain: bool


class FakeMqtt:
    def __init__(self) -> None:
        self.will: MqttMessage | None = None
        self.published: list[MqttMessage] = []
        self.subscriptions: list[tuple[str, int]] = []
        self._events: list[str] = []
        self.fail_connect = False

    def set_will(self, message: MqttMessage) -> None:
        self.will = message

    def connect(self) -> None:
        if self.fail_connect:
            raise OSError("broker unreachable")
        self._events.append("up")

    def lose_connection(self) -> None:
        self._events.append("down")

    def restore_connection(self) -> None:
        self._events.append("up")

    def poll(self) -> list[str]:
        events = self._events
        self._events = []
        return events

    def publish(self, message: MqttMessage) -> None:
        self.published.append(message)

    def subscribe(self, topic: str, qos: int) -> None:
        self.subscriptions.append((topic, qos))

    def disconnect(self) -> None:
        self._events.append("down")


class PahoMqtt:
    """Blocking connect and a background loop. Callbacks only record up/down."""

    def __init__(self, config: EdgeConfig, client: object | None = None) -> None:
        self._config = config
        self._client = client if client is not None else _paho_client(config.mqtt.client_id)
        self._events: list[str] = []
        self._lock = threading.Lock()
        self._will_ready = False
        self._client.on_connect = self._on_connect
        self._client.on_disconnect = self._on_disconnect

    def set_will(self, message: MqttMessage) -> None:
        self._client.will_set(message.topic, message.payload, message.qos, message.retain)
        self._will_ready = True

    def connect(self) -> None:
        if not self._will_ready:
            raise RuntimeError("will must be set before connect")
        mqtt = self._config.mqtt
        self._client.connect(mqtt.host, mqtt.port, mqtt.keepalive_s)
        self._client.loop_start()

    def poll(self) -> list[str]:
        with self._lock:
            events = self._events
            self._events = []
            return events

    def publish(self, message: MqttMessage) -> None:
        self._client.publish(message.topic, message.payload, message.qos, message.retain)

    def subscribe(self, topic: str, qos: int) -> None:
        self._client.subscribe(topic, qos)

    def disconnect(self) -> None:
        self._client.disconnect()
        self._client.loop_stop()

    def _on_connect(self, client: object, userdata: object, flags: object, reason_code: object, properties: object) -> None:
        if getattr(reason_code, "is_failure", False):
            return
        with self._lock:
            self._events.append("up")

    def _on_disconnect(self, client: object, userdata: object, flags: object, reason_code: object, properties: object) -> None:
        with self._lock:
            self._events.append("down")


def _paho_client(client_id: str) -> object:
    import paho.mqtt.client as mqtt

    return mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id=client_id)
