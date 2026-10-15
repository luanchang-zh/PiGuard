"""Process entry: python -m piguard_edge --config edge/config/default.example.yaml"""

import argparse
import signal
import threading

from piguard_edge.config.loader import load_config
from piguard_edge.runtime.app import EdgeApp
from piguard_edge.runtime.clock import SystemClock
from piguard_edge.transport.mqtt import PahoMqtt


def main(argv: list[str] | None = None) -> None:
    parser = argparse.ArgumentParser(prog="piguard-edge")
    parser.add_argument("--config", required=True, help="path to the edge YAML file")
    args = parser.parse_args(argv)
    config = load_config(args.config)
    app = EdgeApp.from_config(config, SystemClock(), PahoMqtt(config))
    stop = threading.Event()

    def request_stop(signum: int, frame: object) -> None:
        stop.set()

    signal.signal(signal.SIGINT, request_stop)
    app.run(stop=stop)


if __name__ == "__main__":
    main()
