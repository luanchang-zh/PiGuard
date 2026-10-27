#!/bin/bash
# Run as root on Ubuntu after uploading a prepared release bundle.
set -Eeuo pipefail
[[ $EUID == 0 ]] || { echo 'Run install.sh with sudo.' >&2; exit 1; }
release=$(realpath "${1:?release directory required}")
host=${2:?public IPv4 address or DNS hostname required}
[[ $release == /opt/piguard/releases/* && $host =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*$ ]] || exit 1
for file in server config.yaml backend.env frontend/index.html deploy/init-broker-auth.sh; do
    [[ -s $release/$file ]] || { echo "Missing bundle file: $file" >&2; exit 1; }
done

if ! command -v mosquitto >/dev/null; then
    export DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=l
    apt-get update
    apt-get install -y --no-install-recommends mosquitto mosquitto-clients
    # Only disable the default instance when this script installed it.
    systemctl disable --now mosquitto.service
fi
command -v nginx >/dev/null || { echo 'Install Nginx before deployment.' >&2; exit 1; }
if ! systemctl is-active --quiet piguard-mqtt.service && ss -H -lnt 'sport = :1883' | read -r _; then
    echo 'Port 1883 is already occupied by another service.' >&2
    exit 1
fi
if ! systemctl is-active --quiet piguard-backend.service && ss -H -lnt 'sport = :18080' | read -r _; then
    echo 'Port 18080 is already occupied by another service.' >&2
    exit 1
fi
id piguard >/dev/null 2>&1 || useradd --system --home-dir /var/lib/piguard --shell /usr/sbin/nologin piguard
install -d -m 755 /etc/piguard /opt/piguard /opt/piguard/releases
install -d -o piguard -g piguard -m 750 /var/lib/piguard /var/lib/piguard/snapshots
install -d -o mosquitto -g mosquitto -m 750 /var/lib/piguard-mqtt
if [[ -e /etc/piguard/backend.env ]]; then
    cmp -s "$release/backend.env" /etc/piguard/backend.env || {
        echo 'Server credentials differ. Recover the original local .secrets directory before updating.' >&2
        exit 1
    }
else
    install -m 600 "$release/backend.env" /etc/piguard/backend.env
fi
rm -f "$release/backend.env"
set -a
source /etc/piguard/backend.env
set +a
PIGUARD_AUTH_CONTAINER=true sh "$release/deploy/init-broker-auth.sh" /etc/piguard/mqtt-auth
install -m 644 "$release/deploy/mosquitto.conf" /etc/piguard/mosquitto.conf
install -m 644 "$release/deploy/piguard-mqtt.service" /etc/systemd/system/piguard-mqtt.service
install -m 644 "$release/deploy/piguard-backend.service" /etc/systemd/system/piguard-backend.service
chmod 755 "$release/server"

previous=$(readlink -f /opt/piguard/current || true)
nginx_config=/etc/nginx/sites-available/piguard
nginx_backup="$release/nginx.previous"
[[ ! -f $nginx_config ]] || cp -p "$nginx_config" "$nginx_backup"
rollback() {
    trap - ERR
    if [[ -n $previous && -d $previous ]]; then
        ln -sfn "$previous" /opt/piguard/current
        systemctl restart piguard-backend.service || true
    fi
    if [[ -f $nginx_backup ]]; then
        cp -p "$nginx_backup" "$nginx_config"
    else
        rm -f /etc/nginx/sites-enabled/piguard "$nginx_config"
    fi
    nginx -t && systemctl reload nginx || true
    echo 'Deployment failed. Check journalctl -u piguard-backend -u piguard-mqtt.' >&2
    exit 1
}
trap rollback ERR
ln -sfn "$release" /opt/piguard/current
sed "s/__PIGUARD_HOST__/$host/g" "$release/deploy/nginx.conf.template" > "$nginx_config"
ln -sfn "$nginx_config" /etc/nginx/sites-enabled/piguard
nginx -t
systemctl daemon-reload
systemctl enable piguard-mqtt.service piguard-backend.service nginx.service
systemctl restart piguard-mqtt.service
systemctl restart piguard-backend.service
healthy=false
for attempt in {1..30}; do
    if curl -fsS http://127.0.0.1:18080/health 2>/dev/null | grep -q '"status":"ok"'; then
        healthy=true
        break
    fi
    sleep 1
done
[[ $healthy == true ]]
systemctl reload nginx.service
healthy=false
for attempt in {1..10}; do
    if curl -fsS -H "Host: $host" http://127.0.0.1/health | grep -q '"status":"ok"'; then
        healthy=true
        break
    fi
    sleep 1
done
[[ $healthy == true ]]
trap - ERR
printf '\nPiGuard deployed: http://%s/\nRelease: %s\n' "$host" "$release"
