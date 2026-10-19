#!/bin/sh
# 本机启动认证 Broker 和 Go 平台；Ctrl+C 只停止本脚本启动的进程。
set -eu
backend_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$backend_dir"
set -a
. ./.env.demo
set +a

runtime_dir="$backend_dir/data/demo"
mkdir -p "$runtime_dir"
broker_bin=${PIGUARD_MOSQUITTO_BIN:-$(command -v mosquitto || true)}
if [ -z "$broker_bin" ] && [ -x "$runtime_dir/tools/usr/sbin/mosquitto" ]; then
  broker_bin="$runtime_dir/tools/usr/sbin/mosquitto"
  PATH="$runtime_dir/tools/usr/bin:$PATH"
  export PATH
  for lib_dir in "$runtime_dir"/tools/usr/lib/*-linux-gnu; do
    if [ -d "$lib_dir" ]; then
      LD_LIBRARY_PATH="$lib_dir${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
      export LD_LIBRARY_PATH
    fi
  done
fi
[ -n "$broker_bin" ] || broker_bin=/usr/sbin/mosquitto
[ -x "$broker_bin" ] || { echo 'mosquitto is required' >&2; exit 1; }
command -v mosquitto_passwd >/dev/null
command -v python3 >/dev/null
command -v go >/dev/null

sh deploy/init-broker-auth.sh "$runtime_dir/auth"
cat > "$runtime_dir/mosquitto.conf" <<EOF
listener 1883 0.0.0.0
allow_anonymous false
password_file $runtime_dir/auth/passwords
acl_file $runtime_dir/auth/acl
persistence true
persistence_location $runtime_dir/
log_dest stdout
EOF
GOCACHE=${GOCACHE:-$runtime_dir/go-build-cache}
export GOCACHE
go build -o "$runtime_dir/piguard-server" ./cmd/server

broker_pid=
server_pid=
cleanup() {
  trap - EXIT
  if [ -n "$server_pid" ]; then kill "$server_pid" 2>/dev/null || true; fi
  if [ -n "$broker_pid" ]; then kill "$broker_pid" 2>/dev/null || true; fi
  if [ -n "$server_pid" ]; then wait "$server_pid" 2>/dev/null || true; fi
  if [ -n "$broker_pid" ]; then wait "$broker_pid" 2>/dev/null || true; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

"$broker_bin" -c "$runtime_dir/mosquitto.conf" &
broker_pid=$!
sleep 0.2
kill -0 "$broker_pid"
python3 - <<'PY'
import socket
import time
for attempt in range(30):
    try:
        with socket.create_connection(('127.0.0.1', 1883), timeout=0.2):
            break
    except OSError:
        time.sleep(0.1)
else:
    raise SystemExit('Broker did not become ready')
PY
"$runtime_dir/piguard-server" -config config.yaml &
server_pid=$!
wait "$server_pid"
