"""MQTT session. Tests use FakeMqtt; the process uses paho-mqtt 2.1."""

import threading
from dataclasses import dataclass

from piguard_edge.config.model import EdgeConfig


class Published:
    def is_published(self) -> bool:
        return True


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
        self._inbox: list[MqttMessage] = []
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

    def deliver(self, message: MqttMessage) -> None:
        self._inbox.append(message)

    def take_messages(self) -> list[MqttMessage]:
        found = self._inbox
        self._inbox = []
        return found

    def publish(self, message: MqttMessage) -> Published:
        self.published.append(message)
        return Published()

    def subscribe(self, topic: str, qos: int) -> None:
        self.subscriptions.append((topic, qos))

    def disconnect(self) -> None:
        self._events.append("down")


class PahoMqtt:
    """Connect and reconnect in the background; callbacks only queue messages."""

    def __init__(self, config: EdgeConfig, client: object | None = None) -> None:
        self._config = config
        self._client = client if client is not None else _paho_client(config.mqtt.client_id)
        self._events: list[str] = []
        self._inbox: list[MqttMessage] = []
        self._lock = threading.Lock()
        self._will_ready = False
        self._client.on_connect = self._on_connect
        self._client.on_disconnect = self._on_disconnect
        self._client.on_message = self._on_message

    def set_will(self, message: MqttMessage) -> None:
        self._client.will_set(message.topic, message.payload, message.qos, message.retain)
        self._will_ready = True

    def connect(self) -> None:
        if not self._will_ready:
            raise RuntimeError("will must be set before connect")
        mqtt = self._config.mqtt
        if mqtt.username:
            self._client.username_pw_set(mqtt.username, mqtt.password)
        self._client.connect_async(mqtt.host, mqtt.port, mqtt.keepalive_s)
        self._client.loop_start()

    def poll(self) -> list[str]:
        with self._lock:
            events = self._events
            self._events = []
            return events

    def take_messages(self) -> list[MqttMessage]:
        with self._lock:
            found = self._inbox
            self._inbox = []
            return found

    def publish(self, message: MqttMessage) -> object:
        receipt = self._client.publish(message.topic, message.payload, message.qos, message.retain)
        if receipt.rc != 0:
            raise OSError("MQTT publish was not accepted")
        return receipt

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

    def _on_message(self, client: object, userdata: object, message: object) -> None:
        with self._lock:
            self._inbox.append(MqttMessage(
                getattr(message, "topic"),
                bytes(getattr(message, "payload")),
                int(getattr(message, "qos", 0)),
                bool(getattr(message, "retain", False)),
            ))


def _paho_client(client_id: str) -> object:
    import paho.mqtt.client as mqtt

    return mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id=client_id)
