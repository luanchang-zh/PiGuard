"""Event cache. Unconfirmed rows stay until the platform has accepted them."""

import sqlite3
from dataclasses import dataclass
from pathlib import Path

from piguard_edge.models.sample import format_utc
from piguard_edge.protocol.event import EdgeEvent


@dataclass(frozen=True, slots=True)
class StoredEvent:
    event_id: str
    payload: bytes
    row_id: int = 0


class EventStore:
    def __init__(self, path: str | Path) -> None:
        self._path = str(path)
        if self._path != ":memory:":
            Path(self._path).parent.mkdir(parents=True, exist_ok=True)
        self._conn: sqlite3.Connection | None = sqlite3.connect(self._path)
        self._conn.row_factory = sqlite3.Row
        self._conn.execute(
            """
            CREATE TABLE IF NOT EXISTS events (
                event_id TEXT PRIMARY KEY,
                device_id TEXT NOT NULL,
                event_type TEXT NOT NULL,
                action TEXT NOT NULL,
                level TEXT NOT NULL,
                timestamp TEXT NOT NULL,
                payload BLOB NOT NULL,
                broker_confirmed INTEGER NOT NULL DEFAULT 0,
                platform_confirmed INTEGER NOT NULL DEFAULT 0
            )
            """
        )
        self._conn.commit()

    def append(self, event: EdgeEvent) -> None:
        conn = self._connection()
        row = conn.execute("SELECT payload FROM events WHERE event_id = ?", (event.event_id,)).fetchone()
        if row is not None:
            if bytes(row["payload"]) != event.payload:
                raise ValueError(f"event_id conflict: {event.event_id}")
            return
        conn.execute(
            """
            INSERT INTO events (
                event_id, device_id, event_type, action, level, timestamp, payload
            ) VALUES (?, ?, ?, ?, ?, ?, ?)
            """,
            (
                event.event_id,
                event.device_id,
                event.event_type,
                event.action,
                event.level,
                format_utc(event.timestamp),
                event.payload,
            ),
        )
        conn.commit()

    def unpublished(self) -> list[StoredEvent]:
        return self._select("broker_confirmed = 0", limit=64)

    def unconfirmed(self, after_row_id: int = 0) -> list[StoredEvent]:
        return self._select("platform_confirmed = 0 AND rowid > ?", limit=64, args=(after_row_id,))

    def request_republish(self) -> None:
        conn = self._connection()
        conn.execute("UPDATE events SET broker_confirmed = 0 WHERE platform_confirmed = 0")
        conn.commit()

    def mark_broker_confirmed(self, event_id: str) -> None:
        self._mark("broker_confirmed", event_id)

    def mark_platform_confirmed(self, event_id: str) -> None:
        self._mark("platform_confirmed", event_id)

    def counts(self) -> tuple[int, int]:
        conn = self._connection()
        total = conn.execute("SELECT COUNT(*) FROM events").fetchone()[0]
        waiting = conn.execute("SELECT COUNT(*) FROM events WHERE platform_confirmed = 0").fetchone()[0]
        return int(total), int(waiting)

    def purge_confirmed_before(self, timestamp: str) -> int:
        conn = self._connection()
        cursor = conn.execute(
            "DELETE FROM events WHERE platform_confirmed = 1 AND timestamp < ?",
            (timestamp,),
        )
        conn.commit()
        return int(cursor.rowcount)

    def close(self) -> None:
        if self._conn is None:
            return
        self._conn.close()
        self._conn = None

    def _select(self, where: str, limit: int = 64, args: tuple = ()) -> list[StoredEvent]:
        conn = self._connection()
        rows = conn.execute(f"SELECT rowid, event_id, payload FROM events WHERE {where} ORDER BY rowid LIMIT ?", (*args, limit)).fetchall()
        return [StoredEvent(row["event_id"], bytes(row["payload"]), row["rowid"]) for row in rows]

    def _mark(self, column: str, event_id: str) -> None:
        if column not in ("broker_confirmed", "platform_confirmed"):
            raise ValueError(f"unknown confirmation column: {column}")
        conn = self._connection()
        conn.execute(f"UPDATE events SET {column} = 1 WHERE event_id = ?", (event_id,))
        conn.commit()

    def _connection(self) -> sqlite3.Connection:
        if self._conn is None:
            raise sqlite3.ProgrammingError("event store is closed")
        return self._conn
