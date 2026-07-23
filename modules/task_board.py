"""
task_board.py — Task management với SQLite, Kanban columns, subtask decompose
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

STATUSES = ["backlog", "queued", "running", "done", "failed"]
PRIORITIES = ["low", "normal", "high", "urgent"]

STATUS_ICONS = {
    "backlog": "📋", "queued": "⏳",
    "running": "⚡", "done": "✅", "failed": "❌"
}
PRIORITY_COLORS = {
    "low": "#8b949e", "normal": "#79c0ff",
    "high": "#e3b341",  "urgent": "#f85149"
}


from modules.models import Task


class TaskBoard:
    """
    Task manager với SQLite — hỗ trợ Kanban, dependencies, subtasks.
    """

    def __init__(self, db_path: str = DB_PATH):
        self.db_path = db_path
        self._init_db()

    def _init_db(self):
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("""
                CREATE TABLE IF NOT EXISTS tasks (
                    task_id     TEXT PRIMARY KEY,
                    title       TEXT NOT NULL,
                    description TEXT DEFAULT '',
                    prompt      TEXT DEFAULT '',
                    priority    TEXT DEFAULT 'normal',
                    status      TEXT DEFAULT 'backlog',
                    assigned_to TEXT,
                    depends_on  TEXT DEFAULT '[]',
                    retry_count INTEGER DEFAULT 0,
                    max_retries INTEGER DEFAULT 3,
                    result      TEXT DEFAULT '',
                    session_id  TEXT,
                    parent_id   TEXT,
                    created_at  TEXT,
                    started_at  TEXT,
                    finished_at TEXT
                )
            """)
            conn.commit()

    # ── CRUD ─────────────────────────────────────────────────────────────

    def add(self, task: Task) -> Task:
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("""
                INSERT INTO tasks VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
            """, (task.task_id, task.title, task.description, task.prompt,
                  task.priority, task.status, task.assigned_to,
                  task.depends_on, task.retry_count, task.max_retries,
                  task.result, task.session_id, task.parent_id,
                  task.created_at, task.started_at, task.finished_at))
            conn.commit()
        return task

    def get(self, task_id: str) -> Optional[Task]:
        with sqlite3.connect(self.db_path) as conn:
            conn.row_factory = sqlite3.Row
            row = conn.execute(
                "SELECT * FROM tasks WHERE task_id=?", (task_id,)
            ).fetchone()
        return Task(**dict(row)) if row else None

    def list_by_status(self, status: str) -> List[Task]:
        with sqlite3.connect(self.db_path) as conn:
            conn.row_factory = sqlite3.Row
            rows = conn.execute("""
                SELECT * FROM tasks WHERE status=?
                ORDER BY
                  CASE priority
                    WHEN 'urgent' THEN 0 WHEN 'high' THEN 1
                    WHEN 'normal' THEN 2 ELSE 3
                  END, created_at
            """, (status,)).fetchall()
        return [Task(**dict(r)) for r in rows]

    def list_all(self) -> List[Task]:
        with sqlite3.connect(self.db_path) as conn:
            conn.row_factory = sqlite3.Row
            rows = conn.execute(
                "SELECT * FROM tasks ORDER BY created_at DESC"
            ).fetchall()
        return [Task(**dict(r)) for r in rows]

    def list_subtasks(self, parent_id: str) -> List[Task]:
        with sqlite3.connect(self.db_path) as conn:
            conn.row_factory = sqlite3.Row
            rows = conn.execute(
                "SELECT * FROM tasks WHERE parent_id=? ORDER BY created_at",
                (parent_id,)
            ).fetchall()
        return [Task(**dict(r)) for r in rows]

    def update_status(self, task_id: str, status: str,
                      result: str = "", session_id: str = None):
        now = datetime.datetime.now().isoformat()
        with sqlite3.connect(self.db_path) as conn:
            if status == "running":
                conn.execute("""
                    UPDATE tasks SET status=?, started_at=?,
                      session_id=COALESCE(?, session_id)
                    WHERE task_id=?
                """, (status, now, session_id, task_id))
            elif status in ("done", "failed"):
                conn.execute("""
                    UPDATE tasks SET status=?, finished_at=?,
                      result=?, session_id=COALESCE(?, session_id)
                    WHERE task_id=?
                """, (status, now, result, session_id, task_id))
            else:
                conn.execute(
                    "UPDATE tasks SET status=? WHERE task_id=?",
                    (status, task_id))
            conn.commit()

    def increment_retry(self, task_id: str) -> int:
        with sqlite3.connect(self.db_path) as conn:
            conn.execute(
                "UPDATE tasks SET retry_count=retry_count+1 WHERE task_id=?",
                (task_id,))
            conn.commit()
            row = conn.execute(
                "SELECT retry_count FROM tasks WHERE task_id=?",
                (task_id,)
            ).fetchone()
        return row[0] if row else 0

    def assign(self, task_id: str, agent_id: str):
        with sqlite3.connect(self.db_path) as conn:
            conn.execute(
                "UPDATE tasks SET assigned_to=?, status='queued' WHERE task_id=?",
                (agent_id, task_id))
            conn.commit()

    def delete(self, task_id: str):
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("DELETE FROM tasks WHERE task_id=?", (task_id,))
            conn.commit()

    def delete_all(self):
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("DELETE FROM tasks")
            conn.commit()

    # ── Dependencies ──────────────────────────────────────────────────────

    def dependencies_met(self, task: Task) -> bool:
        """Trả về True nếu tất cả task phụ thuộc đã Done."""
        for dep_id in task.depends_on_list:
            dep = self.get(dep_id)
            if not dep or dep.status != "done":
                return False
        return True

    def next_dispatchable(self) -> List[Task]:
        """Lấy danh sách task backlog có thể dispatch ngay."""
        candidates = self.list_by_status("backlog")
        return [t for t in candidates if self.dependencies_met(t)]

    # ── Stats ─────────────────────────────────────────────────────────────

    def counts(self) -> dict:
        with sqlite3.connect(self.db_path) as conn:
            rows = conn.execute(
                "SELECT status, COUNT(*) FROM tasks GROUP BY status"
            ).fetchall()
        return {r[0]: r[1] for r in rows}
