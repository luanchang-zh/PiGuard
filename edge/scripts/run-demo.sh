#!/bin/sh
set -eu
edge_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
python_bin=${PIGUARD_EDGE_PYTHON:-$HOME/.venvs/piguard-edge/bin/python}
config_path=${PIGUARD_EDGE_CONFIG:-$edge_dir/config/raspberrypi-demo.yaml}
cd "$edge_dir"
exec "$python_bin" -m piguard_edge --config "$config_path" "$@"
