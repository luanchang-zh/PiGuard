#!/bin/sh
set -eu
edge_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_path=${PIGUARD_EDGE_CONFIG:-$edge_dir/config/cloud/raspberrypi.yaml}
if [ ! -f "$config_path" ]; then
    printf 'Cloud profile missing: %s\nCopy the generated private profile from the deployment computer first.\n' "$config_path" >&2
    exit 1
fi
export PIGUARD_EDGE_CONFIG="$config_path"
exec "$edge_dir/scripts/run-demo.sh" "$@"
