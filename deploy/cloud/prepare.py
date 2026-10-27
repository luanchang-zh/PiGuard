#!/usr/bin/env python3
"""Generate private server credentials and matching cloud device profiles."""

import argparse
import json
import os
from pathlib import Path
import re
import secrets
import shlex

ROOT = Path(__file__).resolve().parents[2]
PRIVATE = ROOT / "deploy/cloud/.secrets"


def private_write(path: Path, value: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.parent.chmod(0o700)
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as stream:
        stream.write(value)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="106.54.188.216")
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9.-]*", args.host):
        parser.error("host must be an IPv4 address or DNS hostname")

    settings_path = PRIVATE / "credentials.json"
    if settings_path.exists():
        settings = json.loads(settings_path.read_text())
        if settings["host"] != args.host:
            parser.error("existing credentials belong to a different host")
    else:
        settings = {"host": args.host, "platform_username": "platform"}
        for name in ("platform_http", "device_http", "platform_mqtt", "device_mqtt", "health_mqtt"):
            settings[name] = secrets.token_urlsafe(24)
        private_write(settings_path, json.dumps(settings, indent=2) + "\n")

    env = {
        "GIN_MODE": "release",
        "PIGUARD_HTTP_ADDR": "127.0.0.1:18080",
        "PIGUARD_HTTP_AUTH_DISABLED": "false",
        "PIGUARD_HTTP_PLATFORM_USERNAME": settings["platform_username"],
        "PIGUARD_HTTP_PLATFORM_PASSWORD": settings["platform_http"],
        "PIGUARD_HTTP_SEED_DEVICE_PASSWORD": settings["device_http"],
        "PIGUARD_HTTP_SESSION_TTL_MINUTES": "120",
        "PIGUARD_HTTP_ALLOWED_ORIGINS": "[]",
        "PIGUARD_MQTT_BROKER": "tcp://127.0.0.1:1883",
        "PIGUARD_MQTT_CLIENT_ID": "piguard-cloud-backend",
        "PIGUARD_MQTT_USERNAME": "platform",
        "PIGUARD_MQTT_PASSWORD": settings["platform_mqtt"],
        "PIGUARD_DEVICE_MQTT_PASSWORD": settings["device_mqtt"],
        "PIGUARD_BROKER_HEALTH_USERNAME": "broker-health",
        "PIGUARD_BROKER_HEALTH_PASSWORD": settings["health_mqtt"],
        "PIGUARD_SEED_DEVICE_ID": "car-001",
        "PIGUARD_SEED_NAME": "PiGuard Car",
        "PIGUARD_SEED_MODE": "hardware",
        "PIGUARD_DATABASE_SQLITE_PATH": "/var/lib/piguard/piguard.db",
        "PIGUARD_STORAGE_SNAPSHOT_DIR": "/var/lib/piguard/snapshots",
    }
    private_write(PRIVATE / "backend.env", "".join(f"{key}={shlex.quote(value)}\n" for key, value in env.items()))

    hardware = (ROOT / "edge/config/raspberrypi-connected-demo.yaml").read_text()
    hardware = hardware.replace("  host: 10.12.194.2", f"  host: {args.host}")
    hardware = hardware.replace("  port: 1883", "  port: 80\n  transport: websockets\n  websocket_path: /mqtt")
    hardware = hardware.replace("http://10.12.194.2:8080/api/v1", f"http://{args.host}/api/v1")
    hardware = hardware.replace("piguard-device-mqtt-demo", settings["device_mqtt"])
    hardware = hardware.replace("piguard-device-http-demo", settings["device_http"])
    hardware = hardware.replace("../data/edge-demo.db", "../../data/edge-cloud.db")
    hardware = hardware.replace("# 与 go-backend/.env.demo 中的设备 HTTP 身份一致。", "# 与云服务器 /etc/piguard/backend.env 中的设备身份一致。")
    private_write(ROOT / "edge/config/cloud/raspberrypi.yaml", hardware)

    # The smoke-test profile uses synthetic inputs and never reads local hardware.
    mock = hardware.replace("mode: hardware", "mode: mock")
    mock = mock.replace("driver: hcsr04", "driver: mock").replace("driver: mpu", "driver: mock")
    mock = mock.replace("driver: aht20", "driver: mock").replace("driver: network", "driver: synthetic")
    mock = mock.replace("driver: gpio", "driver: mock")
    mock = mock.replace("../../data/edge-cloud.db", "../../data/cloud-smoke.db")
    private_write(ROOT / "edge/config/cloud/mock.yaml", mock)

    private_write(PRIVATE / "access.txt", f"PiGuard: http://{args.host}/\nUsername: {settings['platform_username']}\nPassword: {settings['platform_http']}\nMQTT: ws://{args.host}/mqtt (port 80)\nDevice: car-001\n")
    print(f"Cloud configuration ready for http://{args.host}/")
    print("Private login details: deploy/cloud/.secrets/access.txt")
    print("Device profile: edge/config/cloud/raspberrypi.yaml")


if __name__ == "__main__":
    main()
