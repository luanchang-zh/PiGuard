"""Command results. A finished Ack is stored once and resent unchanged."""

import sqlite3
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True, slots=True)
class CommandRecord:
    command_id: str
    device_id: str
    fingerprint: str
    status: str
    ack: bytes | None


@dataclass(frozen=True, slots=True)
class StoredAck:
    command_id: str
    payload: bytes


class CommandStore:
    def __init__(self, path: str | Path) -> None:
        self._path = str(path)
        if self._path != ":memory:":
            Path(self._path).parent.mkdir(parents=True, exist_ok=True)
        self._conn: sqlite3.Connection | None = sqlite3.connect(self._path)
        self._conn.row_factory = sqlite3.Row
        self._conn.execute(
            """
            CREATE TABLE IF NOT EXISTS commands (
                command_id TEXT PRIMARY KEY,
                device_id TEXT NOT NULL,
                fingerprint TEXT NOT NULL,
                status TEXT NOT NULL,
                ack BLOB,
                published INTEGER NOT NULL DEFAULT 0
            )
            """
        )
        self._conn.commit()

    def get(self, command_id: str) -> CommandRecord | None:
        row = self._connection().execute(
            "SELECT command_id, device_id, fingerprint, status, ack FROM commands WHERE command_id = ?",
            (command_id,),
        ).fetchone()
        if row is None:
            return None
        ack = None if row["ack"] is None else bytes(row["ack"])
        return CommandRecord(row["command_id"], row["device_id"], row["fingerprint"], row["status"], ack)

    def insert_running(self, command_id: str, device_id: str, fingerprint: str) -> None:
        self._insert(command_id, device_id, fingerprint, "running", None)

    def insert_finished(self, command_id: str, device_id: str, fingerprint: str, status: str, ack: bytes) -> None:
        if status not in ("success", "failed"):
            raise ValueError(f"unknown command status: {status}")
        self._insert(command_id, device_id, fingerprint, status, ack)

    def finish(self, command_id: str, status: str, ack: bytes) -> None:
        if status not in ("success", "failed"):
            raise ValueError(f"unknown command status: {status}")
        conn = self._connection()
        conn.execute(
            "UPDATE commands SET status = ?, ack = ?, published = 0 WHERE command_id = ? AND status = 'running'",
            (status, ack, command_id),
        )
        conn.commit()

    def request_republish(self, command_id: str) -> None:
        conn = self._connection()
        conn.execute("UPDATE commands SET published = 0 WHERE command_id = ? AND ack IS NOT NULL", (command_id,))
        conn.commit()

    def running(self) -> list[CommandRecord]:
        rows = self._connection().execute(
            "SELECT command_id, device_id, fingerprint, status, ack FROM commands WHERE status = 'running' ORDER BY rowid"
        ).fetchall()
        return [CommandRecord(row["command_id"], row["device_id"], row["fingerprint"], row["status"], None) for row in rows]

    def unpublished(self) -> list[StoredAck]:
        rows = self._connection().execute(
            "SELECT command_id, ack FROM commands WHERE published = 0 AND ack IS NOT NULL ORDER BY rowid"
        ).fetchall()
        return [StoredAck(row["command_id"], bytes(row["ack"])) for row in rows]

    def mark_published(self, command_id: str) -> None:
        conn = self._connection()
        conn.execute("UPDATE commands SET published = 1 WHERE command_id = ?", (command_id,))
        conn.commit()

    def close(self) -> None:
        if self._conn is None:
            return
        self._conn.close()
        self._conn = None

    def _insert(self, command_id: str, device_id: str, fingerprint: str, status: str, ack: bytes | None) -> None:
        conn = self._connection()
        conn.execute(
            """
            INSERT INTO commands (command_id, device_id, fingerprint, status, ack)
            VALUES (?, ?, ?, ?, ?)
            """,
            (command_id, device_id, fingerprint, status, ack),
        )
        conn.commit()

    def _connection(self) -> sqlite3.Connection:
        if self._conn is None:
            raise sqlite3.ProgrammingError("command store is closed")
        return self._conn
