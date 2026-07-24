"""
claude_cli.py — Bridge giữa Python và Claude Code CLI
Wrap subprocess calls tới `claude` command.
"""

import subprocess
import json
import threading
import time
import uuid
import os
from dataclasses import dataclass, field
from typing import Optional, Callable
from modules.antigravity_cli import AntigravityCLI

# Tìm đường dẫn claude executable
def _find_claude():
    import shutil
    path = shutil.which("claude")
    if path:
        return path
    # Windows fallbacks
    candidates = [
        os.path.expanduser("~\\AppData\\Local\\AnthropicClaude\\claude.exe"),
        r"C:\Program Files\Claude\claude.exe",
        "claude",
    ]
    for c in candidates:
        if os.path.isfile(c):
            return c
    return "claude"  # rely on PATH

CLAUDE_EXE = _find_claude()

# Model names (OAuth + Pro subscription)
MODELS = {
    "opus":    "claude-opus-4-8",    # Opus 4.8 (latest)
    "sonnet":  "claude-sonnet-4-5",  # Sonnet 4.5
    "haiku":   "claude-haiku-4-5",   # Haiku 4.5
    "default": "claude-opus-4-8",
}


@dataclass
class AgentSession:
    """Đại diện cho 1 agent với session Claude Code."""
    agent_id:    str
    name:        str
    model:       str        = "claude-opus-4-8"
    system:      str        = ""
    session_id:  Optional[str] = None   # Claude Code session ID (để resume)
    status:      str        = "idle"    # idle | running | done | error
    last_output: str        = ""
    last_error:  str        = ""
    tasks_done:  int        = 0


@dataclass
class RunResult:
    success:    bool
    output:     str
    raw_json:   Optional[dict] = None
    session_id: Optional[str]  = None
    error:      str            = ""
    duration_s: float          = 0.0
    tokens_used: int           = 0


class ClaudeCLI:
    """
    Wrapper để gọi `claude` CLI và nhận kết quả.

    Cách dùng:
        cli = ClaudeCLI()
        result = cli.run_once("Summarize this file", model="claude-haiku-4-5")
        print(result.output)
    """

    def __init__(self, timeout: int = 300):
        self.timeout = timeout
        self._sessions: dict[str, AgentSession] = {}

    # ── One-shot call ─────────────────────────────────────────────────────

    def run_once(
        self,
        prompt: str,
        model: str = "claude-opus-4-8",
        system: str = "",
        output_format: str = "json",
        extra_args: list = None,
        on_log: Optional[Callable] = None,
        cwd: Optional[str] = None,
    ) -> RunResult:
        if any(k in model.lower() for k in ["gemini", "thinking", "antigravity", "agy"]):
            ag_cli = AntigravityCLI(timeout=self.timeout)
            res = ag_cli.run_once(prompt=prompt, model=model, system=system, on_log=on_log, cwd=cwd)
            return RunResult(success=res.success, output=res.output, duration_s=res.duration_s, error=res.error)

        full_prompt = f"[System Context: {system}]\n\n{prompt}" if system else prompt
        cmd = [CLAUDE_EXE, "-p", full_prompt,
               "--model", model,
               "--output-format", output_format]
        if extra_args:
            cmd += extra_args

        return self._execute(cmd, on_log, cwd=cwd)

    # ── Resume session ────────────────────────────────────────────────────

    def resume(
        self,
        session_id: str,
        prompt: str,
        model: str = "claude-opus-4-8",
        output_format: str = "json",
        on_log: Optional[Callable] = None,
        cwd: Optional[str] = None,
    ) -> RunResult:
        """
        Tiếp tục conversation cũ bằng session_id.
        Dùng để maintain context qua nhiều lần gọi.
        """
        cmd = [CLAUDE_EXE, "--resume", session_id,
               "-p", prompt,
               "--model", model,
               "--output-format", output_format]
        return self._execute(cmd, on_log, cwd=cwd)

    # ── Agent helpers ─────────────────────────────────────────────────────

    def create_agent(self, name: str, model: str = "claude-opus-4-8",
                     system: str = "") -> AgentSession:
        agent = AgentSession(
            agent_id=str(uuid.uuid4())[:8],
            name=name, model=model, system=system
        )
        self._sessions[agent.agent_id] = agent
        return agent

    def run_agent(
        self,
        agent: AgentSession,
        prompt: str,
        on_log: Optional[Callable] = None,
        cwd: Optional[str] = None,
    ) -> RunResult:
        """
        Gọi agent: nếu có session_id thì resume, không thì tạo mới.
        Tự động cập nhật agent.session_id sau khi xong.
        """
        agent.status = "running"

        if agent.session_id:
            result = self.resume(agent.session_id, prompt,
                                 model=agent.model, on_log=on_log, cwd=cwd)
        else:
            result = self.run_once(prompt, model=agent.model,
                                   system=agent.system, on_log=on_log, cwd=cwd)

        if result.success:
            agent.status = "done"
            agent.tasks_done += 1
            agent.last_output = result.output
            if result.session_id:
                agent.session_id = result.session_id  # save for next call
        else:
            agent.status = "error"
            agent.last_error = result.error

        return result

    def run_agent_async(
        self,
        agent: AgentSession,
        prompt: str,
        on_done: Optional[Callable] = None,
        on_log: Optional[Callable] = None,
    ) -> threading.Thread:
        """
        Chạy agent trong thread riêng (non-blocking).
        on_done(result) được gọi khi xong.
        """
        def _worker():
            result = self.run_agent(agent, prompt, on_log=on_log)
            if on_done:
                on_done(result)

        t = threading.Thread(target=_worker, daemon=True)
        t.start()
        return t

    # ── Internal execute ──────────────────────────────────────────────────

    def _execute(self, cmd: list, on_log: Optional[Callable], cwd: Optional[str] = None) -> RunResult:
        if on_log:
            on_log(f"CMD: {' '.join(cmd[:4])}...", "INFO")

        t0 = time.time()
        try:
            kwargs = {}
            if os.name == "nt":
                kwargs["creationflags"] = getattr(subprocess, "CREATE_NO_WINDOW", 0x08000000)

            proc = subprocess.Popen(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                encoding="utf-8",
                errors="replace",
                stdin=subprocess.DEVNULL,  # avoid stdin warning
                cwd=cwd,
                **kwargs
            )
            if on_log:
                on_log("🧠 AI Thinking & Processing request...", "THINKING")

            stdout_lines = []
            stderr_lines = []

            def _read_stdout():
                for line in iter(proc.stdout.readline, ''):
                    if not line:
                        break
                    stdout_lines.append(line)
                    clean = line.strip()
                    if clean and on_log:
                        try:
                            on_log(clean, "THINKING")
                        except Exception:
                            try:
                                on_log(clean.encode('ascii', errors='ignore').decode('ascii'), "THINKING")
                            except Exception:
                                pass

            def _read_stderr():
                for line in iter(proc.stderr.readline, ''):
                    if not line:
                        break
                    stderr_lines.append(line)
                    clean = line.strip()
                    if clean and on_log:
                        try:
                            on_log(clean, "WARN")
                        except Exception:
                            pass

            t_out = threading.Thread(target=_read_stdout, daemon=True)
            t_err = threading.Thread(target=_read_stderr, daemon=True)
            t_out.start()
            t_err.start()

            try:
                proc.wait(timeout=self.timeout)
            except subprocess.TimeoutExpired:
                proc.kill()
                if on_log:
                    on_log(f"Timeout sau {self.timeout}s", "ERROR")
                return RunResult(success=False, output="", error=f"Timeout sau {self.timeout}s", duration_s=time.time()-t0)

            t_out.join(timeout=2)
            t_err.join(timeout=2)

            stdout = "".join(stdout_lines)
            stderr = "".join(stderr_lines)
            duration = time.time() - t0

            if proc.returncode != 0:
                err = stderr.strip() or f"Exit code {proc.returncode}"
                if "is_error" in stdout or "api_error" in stdout:
                    err += f"\n[STDOUT]: {stdout.strip()}"
                if on_log:
                    on_log(f"Loi CLI: {err[:120]}", "ERROR")
                return RunResult(success=False, output=stdout,
                                 error=err, duration_s=duration)

            # Parse JSON output & token usage
            raw = None
            output_text = stdout.strip()
            session_id = None
            tokens_count = 0

            import re
            tok_match = re.search(r"(?:total_tokens|tokens_used|usage)\s*[:=]\s*(\d+)", stdout + stderr, re.I)
            if tok_match:
                tokens_count = int(tok_match.group(1))

            if output_text:
                try:
                    raw = json.loads(output_text)
                    # Claude CLI JSON có dạng: {"result": "...", "session_id": "...", "usage": {"total_tokens": ...}}
                    output_text = raw.get("result", raw.get("content", output_text))
                    session_id  = raw.get("session_id")
                    if isinstance(raw, dict):
                        u = raw.get("usage", {})
                        if isinstance(u, dict) and "total_tokens" in u:
                            tokens_count = u["total_tokens"]
                except json.JSONDecodeError:
                    pass  # Không phải JSON, dùng raw text

            if tokens_count <= 0:
                tokens_count = max(1, (len(cmd[2] if len(cmd)>2 else "") + len(str(output_text))) // 4)

            if on_log:
                preview = str(output_text)[:80].replace("\n", " ")
                on_log(f"OK ({duration:.1f}s, {tokens_count:,} tokens): {preview}...", "SUCCESS")

            return RunResult(
                success=True, output=str(output_text),
                raw_json=raw, session_id=session_id,
                duration_s=duration, tokens_used=tokens_count
            )

        except FileNotFoundError:
            msg = f"Khong tim thay `claude` CLI. PATH: {CLAUDE_EXE}"
            if on_log:
                on_log(msg, "ERROR")
            return RunResult(success=False, output="", error=msg)
        except Exception as e:
            if on_log:
                on_log(f"Exception: {e}", "ERROR")
            return RunResult(success=False, output="", error=str(e))


# ── Quick test ────────────────────────────────────────────────────────────────

if __name__ == "__main__":
    cli = ClaudeCLI(timeout=60)
    print("Testing Claude CLI...")
    result = cli.run_once(
        "Reply with exactly: CLAUDE_CLI_OK",
        model="claude-opus-4-8",
        on_log=lambda m, lvl: print(f"[{lvl}] {m}")
    )
    print(f"Success: {result.success}")
    print(f"Output:  {result.output}")
    print(f"Session: {result.session_id}")
    print(f"Time:    {result.duration_s:.2f}s")
