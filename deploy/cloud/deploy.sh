#!/bin/bash
# Build locally and deploy to the designated Ubuntu server over SSH.
set -Eeuo pipefail
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
host=${PIGUARD_CLOUD_HOST:-106.54.188.216}
target=${PIGUARD_SSH_TARGET:-ubuntu@$host}
ssh_options=()
if [[ -n ${PIGUARD_SSH_CONTROL_PATH:-} ]]; then
    ssh_options+=(-S "$PIGUARD_SSH_CONTROL_PATH")
fi
python3 "$repo/deploy/cloud/prepare.py" --host "$host"
stage=$(mktemp -d /tmp/piguard-cloud.XXXXXXXX)
trap 'rm -rf "$stage"' EXIT
mkdir -p "$stage/bundle/frontend" "$stage/bundle/deploy"
(
    cd "$repo/frontend"
    npm ci --no-audit --no-fund
    npm run build
)
(
    cd "$repo/go-backend"
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o "$stage/bundle/server" ./cmd/server
)
cp -a "$repo/frontend/dist/." "$stage/bundle/frontend/"
cp "$repo/go-backend/config.yaml" "$stage/bundle/config.yaml"
cp "$repo/deploy/cloud/.secrets/backend.env" "$stage/bundle/backend.env"
cp "$repo/deploy/cloud/"{install.sh,nginx.conf.template,mosquitto.conf,piguard-backend.service,piguard-mqtt.service} "$stage/bundle/deploy/"
cp "$repo/go-backend/deploy/init-broker-auth.sh" "$stage/bundle/deploy/"
tar -C "$stage/bundle" -czf "$stage/release.tar.gz" .
release="$(date -u +%Y%m%dT%H%M%SZ)-$(git -C "$repo" rev-parse --short HEAD)"
remote_archive="/tmp/piguard-$release.tar.gz"
ssh "${ssh_options[@]}" "$target" "umask 077; cat > '$remote_archive'" < "$stage/release.tar.gz"
ssh "${ssh_options[@]}" "$target" "sudo install -d -m 755 /opt/piguard/releases/'$release' && sudo tar -xzf '$remote_archive' -C /opt/piguard/releases/'$release' && sudo bash /opt/piguard/releases/'$release'/deploy/install.sh /opt/piguard/releases/'$release' '$host'; result=\$?; rm -f '$remote_archive'; exit \$result"
