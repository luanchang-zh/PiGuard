#!/bin/sh
set -eu
[ -f /mosquitto/auth/ready ] && [ -s /mosquitto/auth/passwords ] && [ -s /mosquitto/auth/acl ] || { printf 'Broker credentials not initialized\n' >&2; exit 1; }
chown -R mosquitto:mosquitto /mosquitto/data
exec /usr/sbin/mosquitto -c /mosquitto/config/mosquitto.conf
