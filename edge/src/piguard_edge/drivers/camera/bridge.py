"""Wire limits and credentials shared by the PC server and edge driver."""

import os

MAX_JPEG_BYTES = 2 * 1024 * 1024
AGE_HEADER = "X-Frame-Age-Ms"


def camera_token(env_name: str) -> str:
    token = os.environ.get(env_name, "")
    if not token or any(ord(char) < 33 or ord(char) > 126 for char in token):
        raise ValueError(f"set {env_name} to a non-empty ASCII token without spaces")
    return token
