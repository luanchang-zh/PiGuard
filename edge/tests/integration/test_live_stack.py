"""Live Broker, Go backend, HTTP, and two WebSocket clients.

The default unit run does not start this file. It needs the authenticated
Compose stack and the passwords in the environment.
"""

import asyncio
import json
import os
import subprocess
import threading
import time
import urllib.error
import urllib.request
from base64 import b64encode
from dataclasses import replace

import paho.mqtt.client as mqtt
import pytest
import websockets

from piguard_edge.runtime.app import EdgeApp
from piguard_edge.runtime.clock import SystemClock
from piguard_edge.transport.mqtt import PahoMqtt

API = "http://127.0.0.1:8080/api/v1"
WS = "ws://127.0.0.1:8080/api/v1/ws"


def _required(name: str) -> str:
    value = os.environ.get(name, "")
    if not value:
        pytest.fail(f"{name} is required for the live stack")
    return value


def _basic(username: str, password: str) -> str:
    token = b64encode(f"{username}:{password}".encode("utf-8")).decode("ascii")
    return f"Basic {token}"


def _request(method: str, path: str, auth: str, payload: dict | None = None) -> tuple[int, dict]:
    data = None if payload is None else json.dumps(payload).encode("utf-8")
    headers = {"Authorization": auth}
    if data is not None:
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(API + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=5) as response:
            return response.status, json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8")
        body = json.loads(raw) if raw else {}
        return exc.code, body
    except urllib.error.URLError as exc:
        return 0, {"error": str(exc.reason)}


def _listen(auth: str, sink: list[dict], stop: threading.Event) -> None:
    async def collect() -> None:
        while not stop.is_set():
            try:
                async with websockets.connect(
                    WS,
                    additional_headers={"Authorization": auth},
                    proxy=None,
                    open_timeout=5,
                ) as socket:
                    while not stop.is_set():
                        try:
                            raw = await asyncio.wait_for(socket.recv(), timeout=0.5)
                        except TimeoutError:
                            continue
                        sink.append(json.loads(raw))
            except (OSError, TimeoutError, websockets.exceptions.WebSocketException):
                await asyncio.sleep(0.2)

    asyncio.run(collect())


def _pump(app: EdgeApp, seconds: float) -> None:
    end = time.time() + seconds
    while time.time() < end:
        app.step()
        time.sleep(0.05)


def _wait_connected(app: EdgeApp, seconds: float = 15) -> None:
    end = time.time() + seconds
    while time.time() < end:
        app.step()
        if app._connected:
            return
        time.sleep(0.05)
    pytest.fail("edge did not connect to the broker")


def test_live_broker_stores_telemetry_events_and_command_acks(example_config, tmp_path) -> None:
    platform_user = _required("PIGUARD_HTTP_PLATFORM_USERNAME")
    platform_http = _required("PIGUARD_HTTP_PLATFORM_PASSWORD")
    platform_mqtt = _required("PIGUARD_MQTT_PASSWORD")
    device_mqtt = _required("PIGUARD_DEVICE_MQTT_PASSWORD")
    compose_dir = _required("PIGUARD_COMPOSE_DIR")
    auth = _basic(platform_user, platform_http)
    device_id = os.environ.get("PIGUARD_SEED_DEVICE_ID", "car-001")

    config = replace(
        example_config,
        mqtt=replace(example_config.mqtt, username=device_id, password=device_mqtt),
        simulation=replace(example_config.simulation, scenario="normal_drive"),
        storage=replace(example_config.storage, sqlite_path=tmp_path / "edge.db"),
    )
    app = EdgeApp.from_config(config, SystemClock(), PahoMqtt(config))
    left: list[dict] = []
    right: list[dict] = []
    stop = threading.Event()
    threads = [
        threading.Thread(target=_listen, args=(auth, left, stop), daemon=True),
        threading.Thread(target=_listen, args=(auth, right, stop), daemon=True),
    ]
    for thread in threads:
        thread.start()
    time.sleep(0.5)

    app.start()
    try:
        _wait_connected(app)
        _pump(app, 2)
        state = _request("GET", f"/devices/{device_id}/state", auth)[1]["data"]
        assert state["online"] is True
        assert state["speed_kmh"] == pytest.approx(30)
        assert state["lane"]["valid"] is True
        assert state["lane"]["direction"] == "center"
        assert state["lane"]["offset_ratio"] == pytest.approx(0, abs=0.02)

        created = _request("POST", f"/devices/{device_id}/commands", auth, {"type": "buzzer.test", "params": {"duration_ms": 400}})
        assert created[0] == 202, created[1]
        command_id = created[1]["data"]["command_id"]
        finished = _wait_command(app, auth, command_id, "success")
        assert finished["result"]["message"] == "buzzer test completed"
        executed_at = finished["executed_at"]

        _republish(device_id, platform_user, platform_mqtt, finished)
        _pump(app, 1)
        again = _request("GET", f"/commands/{command_id}", auth)[1]["data"]
        assert again["status"] == "success"
        assert again["executed_at"] == executed_at
        assert app._actuators.manual is None

        started = _request(
            "POST",
            f"/devices/{device_id}/commands",
            auth,
            {"type": "scenario.start", "params": {"scenario": "obstacle_approach", "speed": 1}},
        )
        assert started[0] == 202, started[1]
        _wait_command(app, auth, started[1]["data"]["command_id"], "success")
        _pump(app, 7.5)
        events = _request("GET", f"/devices/{device_id}/events?limit=20", auth)[1]["data"]["items"]
        ids = [item["event_id"] for item in events]
        assert len(ids) == len(set(ids))
        assert sum(item["type"] == "obstacle_warning" and item["action"] == "started" for item in events) == 1
        assert _request("GET", f"/devices/{device_id}/state", auth)[1]["data"]["risk_level"] == "danger"

        subprocess.run(
            ["docker", "compose", "-p", "piguard-edge-it", "restart", "mosquitto"],
            cwd=compose_dir,
            check=True,
        )
        _wait_reconnected(app, auth, device_id)
        after = _request("GET", f"/devices/{device_id}/events?limit=20", auth)[1]["data"]["items"]
        assert {item["event_id"] for item in after} == set(ids)
        assert _request("GET", f"/devices/{device_id}", auth)[1]["data"]["online"] is True

        for sink in (left, right):
            kinds = {item["type"] for item in sink}
            assert {"telemetry", "event", "command_update"} <= kinds
    finally:
        app.stop()
        time.sleep(0.5)
        stop.set()
        for thread in threads:
            thread.join(timeout=2)

    deadline = time.time() + 8
    offline = None
    while time.time() < deadline:
        offline = _request("GET", f"/devices/{device_id}", auth)[1]["data"]
        if offline["online"] is False:
            break
        time.sleep(0.2)
    assert offline is not None
    assert offline["online"] is False


def _wait_reconnected(app: EdgeApp, auth: str, device_id: str) -> None:
    saw_down = False
    saw_offline = False
    end = time.time() + 30
    while time.time() < end:
        app.step()
        if not app._connected:
            saw_down = True
        body = _request("GET", f"/devices/{device_id}", auth)
        if body[0] == 200 and body[1]["data"]["online"] is False:
            saw_offline = True
        if saw_down and app._connected and saw_offline and body[0] == 200 and body[1]["data"]["online"] is True:
            return
        time.sleep(0.05)
    pytest.fail(f"broker restart did not complete: down={saw_down} offline={saw_offline} connected={app._connected}")


def _wait_command(app: EdgeApp, auth: str, command_id: str, status: str) -> dict:
    end = time.time() + 8
    last = None
    while time.time() < end:
        app.step()
        last = _request("GET", f"/commands/{command_id}", auth)[1]["data"]
        if last["status"] == status:
            return last
        time.sleep(0.05)
    pytest.fail(f"command {command_id} stayed {last}")


def _republish(device_id: str, username: str, password: str, view: dict) -> None:
    params = view["params"]
    if isinstance(params, str):
        params = json.loads(params)
    payload = json.dumps(
        {
            "schema_version": 1,
            "command_id": view["command_id"],
            "device_id": device_id,
            "type": view["type"],
            "issued_at": view["issued_at"],
            "expires_at": view["expires_at"],
            "params": params,
        },
        separators=(",", ":"),
    ).encode("utf-8")
    client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id="piguard-it-platform")
    client.username_pw_set(username, password)
    client.connect("127.0.0.1", 1883, 30)
    client.loop_start()
    try:
        published = client.publish(f"car/{device_id}/commands", payload, qos=1, retain=False)
        published.wait_for_publish(5)
    finally:
        client.disconnect()
        client.loop_stop()
