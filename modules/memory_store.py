"""
memory_store.py — SQLite-backed Memory & Conversation History cho Claude Agents
Lưu trữ toàn bộ lịch sử trao đổi, prompt, kết quả và session_id vào SQLite (agent_manager.db).
"""

import sqlite3
import json
import uuid
import datetime
import os
from dataclasses import dataclass, field
from typing import Optional, List

def _get_db_path():
    base = r"e:\exe" if os.path.exists(r"e:\exe") else os.path.expanduser("~\\.claude_suite")
    os.makedirs(base, exist_ok=True)
    return os.path.join(base, "agent_manager.db")

DB_PATH = _get_db_path()


from modules.models import MemoryItem


class MemoryStore:
    """
    Quản lý bộ nhớ lâu dài (Long-term Memory) của Agents trong SQLite.
    """

    def __init__(self, db_path: str = DB_PATH):
        self.db_path = db_path
        self._init_db()

    def _init_db(self):
        with sqlite3.connect(self.db_path, check_same_thread=False) as conn:
            conn.execute("PRAGMA journal_mode=WAL;")
            conn.execute("""
                CREATE TABLE IF NOT EXISTS agent_memory (
                    memory_id   TEXT PRIMARY KEY,
                    agent_id    TEXT NOT NULL,
                    task_id     TEXT,
                    session_id  TEXT,
                    role        TEXT DEFAULT 'user',
                    content     TEXT NOT NULL,
                    timestamp   TEXT,
                    tokens_used INTEGER DEFAULT 0
                )
            """)
            conn.commit()

    def add(self, item: MemoryItem) -> MemoryItem:
        with sqlite3.connect(self.db_path, check_same_thread=False) as conn:
            conn.execute("""
                INSERT INTO agent_memory VALUES (?,?,?,?,?,?,?,?)
            """, (item.memory_id, item.agent_id, item.task_id, item.session_id,
                  item.role, item.content, item.timestamp, item.tokens_used))
            conn.commit()
        return item

    def get_agent_memory(self, agent_id: str, limit: int = 50) -> List[MemoryItem]:
        with sqlite3.connect(self.db_path, check_same_thread=False) as conn:
            conn.row_factory = sqlite3.Row
            rows = conn.execute("""
                SELECT * FROM agent_memory WHERE agent_id = ? ORDER BY timestamp DESC LIMIT ?
            """, (agent_id, limit)).fetchall()
            return [MemoryItem(**dict(r)) for r in rows]

    def get_task_memory(self, task_id: str) -> List[MemoryItem]:
        with sqlite3.connect(self.db_path, check_same_thread=False) as conn:
            conn.row_factory = sqlite3.Row
            rows = conn.execute("""
                SELECT * FROM agent_memory WHERE task_id = ? ORDER BY timestamp ASC
            """, (task_id,)).fetchall()
            return [MemoryItem(**dict(r)) for r in rows]

    def clear_agent_memory(self, agent_id: str):
        with sqlite3.connect(self.db_path, check_same_thread=False) as conn:
            conn.execute("DELETE FROM agent_memory WHERE agent_id = ?", (agent_id,))
            conn.commit()
