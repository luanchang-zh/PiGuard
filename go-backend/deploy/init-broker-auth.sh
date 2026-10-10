#!/bin/sh
set -eu
umask 077

fail() { printf '%s\n' "$1" >&2; exit 1; }
username() {
  case "$2" in ''|*[!a-zA-Z0-9_-]*) fail "$1: invalid username";; esac
  [ "${#2}" -le 64 ] || fail "$1: username too long"
}
password() {
  [ -n "$2" ] || fail "$1: password required"
  case "$2" in *"$(printf '\r')"*|*"
"*) fail "$1: newline not permitted";; esac
}

platform=${PIGUARD_MQTT_USERNAME:?PIGUARD_MQTT_USERNAME required}
device=${PIGUARD_SEED_DEVICE_ID:?PIGUARD_SEED_DEVICE_ID required}
health=${PIGUARD_BROKER_HEALTH_USERNAME:?PIGUARD_BROKER_HEALTH_USERNAME required}
username platform "$platform"
username device "$device"
username health "$health"
[ "$platform" != "$device" ] && [ "$platform" != "$health" ] && [ "$device" != "$health" ] || fail 'Broker identities must be distinct'
password PIGUARD_MQTT_PASSWORD "${PIGUARD_MQTT_PASSWORD:-}"
password PIGUARD_DEVICE_MQTT_PASSWORD "${PIGUARD_DEVICE_MQTT_PASSWORD:-}"
password PIGUARD_BROKER_HEALTH_PASSWORD "${PIGUARD_BROKER_HEALTH_PASSWORD:-}"
command -v mosquitto_passwd >/dev/null 2>&1 || fail 'mosquitto_passwd required'

auth_dir=${1:-/mosquitto/auth}
mkdir -p "$auth_dir"
# Only these managed files are replaced. A failed rotation cannot mark a partial pair ready.
rm -f -- "$auth_dir/ready"
password_file=$(mktemp "$auth_dir/passwords.XXXXXX")
acl_file=$(mktemp "$auth_dir/acl.XXXXXX")
trap 'rm -f -- "$password_file" "$acl_file"' EXIT HUP INT TERM
mosquitto_passwd -b "$password_file" "$platform" "$PIGUARD_MQTT_PASSWORD" >/dev/null
mosquitto_passwd -b "$password_file" "$device" "$PIGUARD_DEVICE_MQTT_PASSWORD" >/dev/null
mosquitto_passwd -b "$password_file" "$health" "$PIGUARD_BROKER_HEALTH_PASSWORD" >/dev/null
{
  printf 'user %s\n' "$platform"
  for topic in telemetry events status command-acks config-acks; do printf 'topic read car/+/%s\n' "$topic"; done
  printf 'topic write car/+/commands\ntopic write car/+/config\n\n'
  printf 'user %s\n' "$device"
  printf 'topic read car/%s/commands\ntopic read car/%s/config\n' "$device" "$device"
  for topic in telemetry events status command-acks config-acks; do printf 'topic write car/%s/%s\n' "$device" "$topic"; done
  printf '\nuser %s\ntopic read $SYS/broker/uptime\n' "$health"
} > "$acl_file"
if [ "${PIGUARD_AUTH_CONTAINER:-false}" = true ]; then
  chown mosquitto:mosquitto "$auth_dir" "$password_file" "$acl_file"
fi
chmod 750 "$auth_dir"
chmod 640 "$password_file" "$acl_file"
mv -f -- "$password_file" "$auth_dir/passwords"
mv -f -- "$acl_file" "$auth_dir/acl"
printf 'initialized\n' > "$auth_dir/ready"
chmod 644 "$auth_dir/ready"
printf 'Broker credentials and ACL initialized\n'
