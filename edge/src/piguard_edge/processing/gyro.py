"""Select the configured vertical-axis rate. Acceleration is not a speed."""


def apply_yaw(x: float, y: float, z: float, axis: str, sign: int) -> float:
    component = {"x": x, "y": y, "z": z}[axis]
    return component * sign
