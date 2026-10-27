#!/usr/bin/env python3
"""Check the public frontend, authenticated API, Cookie WebSocket and device MQTT."""

import base64
from http.cookiejar import CookieJar
import json
from pathlib import Path
import threading
from urllib.error import HTTPError
from urllib.request import HTTPCookieProcessor, ProxyHandler, Request, build_opener

import paho.mqtt.client as mqtt
from websockets.sync.client import connect

ROOT = Path(__file__).resolve().parents[2]


def main() -> None:
    settings = json.loads((ROOT / "deploy/cloud/.secrets/credentials.json").read_text())
    origin = f"http://{settings['host']}"
    checks = []
    cookies = CookieJar()
    http = build_opener(ProxyHandler({}), HTTPCookieProcessor(cookies))

    def request(path, *, method="GET", headers=None, data=None, expected=200):
        req = Request(origin + path, data=data, headers=headers or {}, method=method)
        try:
            response = http.open(req, timeout=15)
        except HTTPError as error:
            response = error
        assert response.status == expected, f"{method} {path}: {response.status} != {expected}"
        body = response.read()
        if response.headers.get_content_type() == "application/json":
            return json.loads(body)
        return body

    page = request("/")
    assert b"/assets/" in page
    assert request("/dashboard") == page
    checks.append("frontend and SPA refresh")
    assert request("/health")["status"] == "ok"
    request("/api/v1/devices", expected=401)
    checks.append("health and anonymous API denial")
    basic = base64.b64encode(f"{settings['platform_username']}:{settings['platform_http']}".encode()).decode()
    session = request("/api/v1/auth/session", method="POST", data=b"{}", headers={
        "Authorization": "Basic " + basic, "Content-Type": "application/json", "Origin": origin,
    }, expected=201)["data"]
    assert request("/api/v1/auth/session")["data"]["auth_mode"] == "cookie"
    assert any(device["device_id"] == "car-001" for device in request("/api/v1/devices")["data"])
    request("/api/v1/devices", headers={"Origin": "http://untrusted.invalid"}, expected=403)
    request("/api/v1/auth/session", method="DELETE", expected=403)
    checks.append("login, Cookie session, device listing, Origin and CSRF guards")

    cookie = "; ".join(f"{item.name}={item.value}" for item in cookies)
    with connect(f"ws://{settings['host']}/api/v1/ws", origin=origin,
                 additional_headers={"Cookie": cookie}, proxy=None, open_timeout=10) as ws:
        assert ws.ping().wait(5), "WebSocket pong timeout"
    checks.append("authenticated public WebSocket upgrade and ping")

    connected = threading.Event()
    subscribed = threading.Event()
    result = {}

    def on_connect(client, userdata, flags, reason_code, properties):
        result["connected"] = not reason_code.is_failure
        connected.set()

    def on_subscribe(client, userdata, mid, reasons, properties):
        result["subscribed"] = all(not reason.is_failure for reason in reasons)
        subscribed.set()

    client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id="piguard-cloud-verification", transport="websockets")
    client.ws_set_options(path="/mqtt")
    client.username_pw_set("car-001", settings["device_mqtt"])
    client.on_connect = on_connect
    client.on_subscribe = on_subscribe
    try:
        client.connect(settings["host"], 80, keepalive=15)
        client.loop_start()
        assert connected.wait(10) and result.get("connected"), "MQTT authentication failed"
        client.subscribe("car/car-001/commands", qos=1)
        assert subscribed.wait(10) and result.get("subscribed"), "MQTT subscription failed"
    finally:
        client.disconnect()
        client.loop_stop()
    checks.append("public device MQTT over WebSocket authentication and command subscription")

    request("/api/v1/auth/session", method="DELETE", headers={
        "Origin": origin, "X-CSRF-Token": session["csrf_token"],
    }, expected=204)
    request("/api/v1/devices", expected=401)
    checks.append("logout and session revocation")
    print(json.dumps({"origin": origin, "passed": checks}, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
