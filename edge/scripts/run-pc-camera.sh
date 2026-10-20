#!/bin/sh
set -eu
edge_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
python_bin=${PIGUARD_CAMERA_PYTHON:-$HOME/.venvs/piguard-camera/bin/python}
cd "$edge_dir"
exec "$python_bin" -m piguard_edge.pc_camera --host "${PIGUARD_CAMERA_HOST:-192.168.7.1}" "$@"
