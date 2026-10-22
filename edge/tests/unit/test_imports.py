import ast
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2] / "src" / "piguard_edge"
FORBIDDEN = {"gpiozero", "smbus2", "picamera2", "RPi", "lgpio"}


def test_hardware_libraries_are_imported_only_when_a_driver_is_selected() -> None:
    offenders: list[str] = []
    for path in ROOT.rglob("*.py"):
        tree = ast.parse(path.read_text(encoding="utf-8"))
        for node in tree.body:
            modules: list[str] = []
            if isinstance(node, ast.Import):
                modules = [alias.name.split(".")[0] for alias in node.names]
            elif isinstance(node, ast.ImportFrom) and node.module:
                modules = [node.module.split(".")[0]]
            for name in modules:
                if name in FORBIDDEN:
                    offenders.append(f"{path.name}:{name}")
    assert offenders == []
