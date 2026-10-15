from pathlib import Path

import pytest

from piguard_edge.config.loader import load_config
from piguard_edge.config.model import EdgeConfig

EXAMPLE_CONFIG = Path(__file__).resolve().parents[1] / "config" / "default.example.yaml"


@pytest.fixture
def example_config() -> EdgeConfig:
    return load_config(EXAMPLE_CONFIG)
