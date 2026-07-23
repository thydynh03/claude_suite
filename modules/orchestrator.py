"""
orchestrator.py — Điều phối tasks → agents, monitor, retry
"""

import threading
import time
import datetime
from typing import Callable, Optional

from modules.claude_cli import ClaudeCLI, RunResult
from modules.agent_registry import AgentRegistry, Agent
from modules.task_board import TaskBoard, Task
from modules.memory_store import MemoryStore, MemoryItem


class Orchestrator:
    """
    Điều phối tự động:
    - Lấy task dispatchable từ backlog
    - Tìm agent idle phù hợp
    - Spawn claude CLI process
    - Monitor kết quả
    - Retry nếu fail
    """

    def __init__(self,
                 registry: AgentRegistry,
                 board: TaskBoard,
                 cli: ClaudeCLI,
                 on_log: Optional[Callable] = None,
                 get_context_fn: Optional[Callable[[], str]] = None,
                 get_workspace_fn: Optional[Callable[[], str]] = None,
                 on_task_event: Optional[Callable] = None):
        self.registry = registry
        self.board    = board
        self.cli      = cli
        self.memory   = MemoryStore()
        self.on_log   = on_log or (lambda m, lvl="INFO": print(f"[{lvl}] {m}"))
        self.get_context_fn = get_context_fn
        self.get_workspace_fn = get_workspace_fn
        self.on_task_event = on_task_event

        self._running  = False
        self._thread   = None
        self._lock     = threading.Lock()
        self._active_threads: dict = {}   # task_id → Thread

    def _auto_snapshot_git(self):
        import subprocess
        import os
        
        ws = self.get_workspace_fn() if self.get_workspace_fn else None
        if not ws or not os.path.isdir(ws):
            return
            
        try:
            # Initialize git if needed
            if not os.path.exists(os.path.join(ws, ".git")):
                subprocess.run(["git", "init"], cwd=ws, capture_output=True, check=True)
                
            # Add all changes and commit
            subprocess.run(["git", "add", "."], cwd=ws, capture_output=True, check=True)
            # We don't check=True on commit because there might be nothing to commit
            subprocess.run(["git", "commit", "-m", "Auto-snapshot before AI execution"], cwd=ws, capture_output=True)
            self.log(f"Đã tạo snapshot git tự động tại {ws}", "INFO")
        except Exception as e:
            self.log(f"Auto-snapshot git failed: {e}", "WARN")

    # ── Control ───────────────────────────────────────────────────────────

    def start(self):
        if self._running:
            return
        self._running = True
        self._thread = threading.Thread(target=self._loop, daemon=True)
        self._thread.start()
        self.log("Orchestrator khoi dong.", "SUCCESS")

    def stop(self):
        self._running = False
        self.log("Orchestrator dung.", "WARN")

    def log(self, msg: str, level: str = "INFO"):
        self.on_log(msg, level)

    # ── Main loop ─────────────────────────────────────────────────────────

    def _loop(self):
        while self._running:
            try:
                self._tick()
            except Exception as e:
                self.log(f"Loop error: {e}", "ERROR")
            time.sleep(2)

    def _tick(self):
        """Mỗi tick: tìm task dispatchable + agent idle → spawn."""
        tasks = self.board.next_dispatchable()
        if not tasks:
            return

        for task in tasks:
            if task.task_id in self._active_threads:
                continue  # đang chạy rồi

            agent = self.registry.get_or_create_for_task(task.title, task.prompt)
            if not agent:
                self.log(f"Không thể khởi tạo Agent cho task [{task.task_id}].", "WARN")
                break

            self._dispatch(task, agent)

    # ── Dispatch ──────────────────────────────────────────────────────────

    def _dispatch(self, task: Task, agent: Agent):
        self.log(f"Dispatch [{task.task_id}] '{task.title}' → Agent '{agent.name}'", "SEND")
        self.board.assign(task.task_id, agent.agent_id)
        self.registry.update_status(agent.agent_id, "running",
                                    last_task=task.title)
                                    
        if self.on_task_event:
            self.on_task_event(agent.name, "started", task.title)

        prompt = task.prompt or task.description or task.title

        t = threading.Thread(
            target=self._run_task,
            args=(task, agent, prompt),
            daemon=True
        )
        self._active_threads[task.task_id] = t
        t.start()

    def _run_task(self, task: Task, agent: Agent, prompt: str):
        self.board.update_status(task.task_id, "running")
        self.log(f"[{agent.name}] Dang xu ly: '{task.title}'...", "TIER1")

        # Approval Checkpoint (Human-in-the-loop)
        if getattr(task, "require_approval", False) or "architect" in agent.name.lower() or "ba" in agent.name.lower():
            import tkinter.messagebox as messagebox
            self.log(f"[{agent.name}] Đang chờ duyệt kế hoạch từ người dùng...", "WARN")
            # We pause here by showing a blocking messagebox.
            # In a real async UI this might be handled via a callback, but this simple modal blocks this daemon thread until user answers.
            approved = messagebox.askyesno(
                "Approval Checkpoint", 
                f"Agent '{agent.name}' chuẩn bị thực thi hoặc vừa hoàn thành một bước quan trọng (Lên kế hoạch / BA).\nBạn có muốn cho phép tiếp tục không?"
            )
            if not approved:
                self.log(f"[{agent.name}] Bị từ chối bởi người dùng.", "ERROR")
                self.board.update_status(task.task_id, "failed")
                return

        # Auto-snapshot git
        self._auto_snapshot_git()

        # Automatically prepend Global Workspace Context (Folder + Attached Files)
        ws_ctx = self.get_context_fn() if self.get_context_fn else ""
        full_prompt = f"{ws_ctx}\n\n{prompt}" if ws_ctx else prompt

        def _on_cli_log(m: str, lvl: str = "INFO"):
            self.log(f"  [{agent.name}] {m}", lvl)
            try:
                ag_curr = self.registry.get(agent.agent_id)
                if ag_curr:
                    ag_curr.status = "running"
                    ag_curr.last_task = task.title
                    ag_curr.notes = m[:120]
                    self.registry.update(ag_curr)
            except Exception:
                pass

        result = self.cli.run_agent(
            agent, full_prompt,
            on_log=_on_cli_log
        )

        self.memory.add(MemoryItem(
            agent_id=agent.agent_id, task_id=task.task_id,
            session_id=result.session_id, role="user", content=task.prompt or task.description
        ))

        if getattr(result, "tokens_used", 0) > 0:
            self.registry.add_tokens(agent.agent_id, result.tokens_used)

        # Check for Token Quota Exhaustion -> Smart Fallback to Antigravity CLI
        fresh_agent = self.registry.get(agent.agent_id) or agent
        if not result.success and self._is_quota_exhausted(fresh_agent, result.error):
            fallback_info = fresh_agent.fallback_to_antigravity()
            self.registry.update(fresh_agent)
            self.log(f"⚠️ [SMART FALLBACK] Hết Token/Quota Claude CLI! Tự động chuyển Agent '{fresh_agent.name}' sang Antigravity CLI ({fresh_agent.model})", "WARN")
            
            # Retry immediately via Antigravity CLI
            result = self.cli.run_agent(
                fresh_agent, prompt,
                on_log=lambda m, lvl="INFO": self.log(f"  [{fresh_agent.name}] (Fallback) {m}", lvl)
            )
            if getattr(result, "tokens_used", 0) > 0:
                self.registry.add_tokens(fresh_agent.agent_id, result.tokens_used)

        if result.success:
            self.memory.add(MemoryItem(
                agent_id=agent.agent_id, task_id=task.task_id,
                session_id=result.session_id, role="assistant", content=result.output
            ))
            self.board.update_status(
                task.task_id, "done",
                result=result.output[:500],
                session_id=result.session_id
            )
            self.registry.update_status(
                agent.agent_id, "idle",
                tasks_done_delta=1,
                session_id=result.session_id
            )
            self.log(f"[{agent.name}] DONE '{task.title}' ({result.duration_s:.1f}s)", "SUCCESS")
        else:
            retry = self.board.increment_retry(task.task_id)
            if retry < task.max_retries:
                self.log(f"[{agent.name}] Fail, retry {retry}/{task.max_retries}...", "WARN")
                self.board.update_status(task.task_id, "backlog")
            else:
                self.log(f"[{agent.name}] FAILED '{task.title}' sau {retry} retries", "ERROR")
                self.board.update_status(task.task_id, "failed",
                                         result=result.error)
            self.registry.update_status(agent.agent_id, "idle",
                                        last_error=result.error)

        with self._lock:
            self._active_threads.pop(task.task_id, None)
            
        if self.on_task_event:
            self.on_task_event(agent.name, "completed", None)

    # ── Manual dispatch ───────────────────────────────────────────────────

    def run_now(self, agent: Agent, prompt: str,
                on_done: Optional[Callable] = None):
        """Chạy ngay 1 prompt với agent chỉ định (không qua queue)."""
        self.log(f"Manual run: Agent '{agent.name}'", "SEND")
        self.registry.update_status(agent.agent_id, "running")

        def _worker():
            self.memory.add(MemoryItem(
                agent_id=agent.agent_id, role="user", content=prompt
            ))
            result = self.cli.run_agent(agent, prompt,
                on_log=lambda m, lvl="INFO": self.log(f"  [{agent.name}] {m}", lvl))
            if result.success:
                self.memory.add(MemoryItem(
                    agent_id=agent.agent_id, session_id=result.session_id,
                    role="assistant", content=result.output
                ))
            status = "idle" if result.success else "error"
            self.registry.update_status(
                agent.agent_id, status,
                tasks_done_delta=1 if result.success else 0,
                last_error=result.error,
                session_id=result.session_id
            )
            msg = f"'{agent.name}' xong: {result.output[:60]}" if result.success \
                  else f"'{agent.name}' loi: {result.error[:60]}"
            self.log(msg, "SUCCESS" if result.success else "ERROR")
            if on_done:
                on_done(result)

        threading.Thread(target=_worker, daemon=True).start()

    # ── Plan execution ────────────────────────────────────────────────────

    def execute_plan(self, tasks: list):
        """Thêm danh sách tasks vào board và bắt đầu orchestrate."""
        for t in tasks:
            self.board.add(t)
        self.log(f"Plan: {len(tasks)} tasks da them vao queue.", "INFO")
        if not self._running:
            self.start()

    def _is_quota_exhausted(self, agent: Agent, result_error: str) -> bool:
        if agent.token_remaining <= 0:
            return True
        # Only fallback if model is native Claude CLI model and error indicates quota limit
        m = agent.model.lower()
        if "thinking" in m or "gemini" in m or "antigravity" in m:
            return False  # Already on Antigravity CLI
        
        err_lower = (result_error or "").lower()
        quota_keywords = [
            "token limit", "quota", "rate limit", "credit balance",
            "context_length_exceeded", "429", "out of tokens", "token_remaining"
        ]
        return any(k in err_lower for k in quota_keywords)
