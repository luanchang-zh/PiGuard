"""Persist each configuration outcome before acknowledging it."""

import json
import sqlite3
from pathlib import Path


class ConfigStore:
    def __init__(self, path: Path) -> None:
        self._conn = sqlite3.connect(path)
        self._conn.execute("CREATE TABLE IF NOT EXISTS configs (version INTEGER PRIMARY KEY, rules TEXT NOT NULL, ack BLOB NOT NULL, published INTEGER NOT NULL DEFAULT 0)")
        self._conn.commit()

    def get(self, version: int):
        return self._conn.execute("SELECT version, rules, ack FROM configs WHERE version = ?", (version,)).fetchone()

    def latest(self):
        rows = self._conn.execute("SELECT version, rules, ack FROM configs ORDER BY version DESC")
        for row in rows:
            if json.loads(row[2])["status"] == "success":
                return row
        return None

    def save(self, version: int, rules: str, ack: bytes) -> None:
        self._conn.execute("INSERT INTO configs (version, rules, ack) VALUES (?, ?, ?)", (version, rules, ack))
        self._conn.commit()

    def republish(self, version: int | None = None) -> None:
        if version is None:
            self._conn.execute("UPDATE configs SET published = 0 WHERE version = (SELECT MAX(version) FROM configs)")
        else:
            self._conn.execute("UPDATE configs SET published = 0 WHERE version = ?", (version,))
        self._conn.commit()

    def unpublished(self):
        return self._conn.execute("SELECT version, ack FROM configs WHERE published = 0 ORDER BY version LIMIT 64").fetchall()

    def mark_published(self, version: int) -> None:
        self._conn.execute("UPDATE configs SET published = 1 WHERE version = ?", (version,))
        self._conn.commit()

    def close(self) -> None:
        self._conn.close()
