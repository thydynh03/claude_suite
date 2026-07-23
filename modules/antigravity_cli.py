"""
antigravity_cli.py — Bridge cho Antigravity CLI (Google AI Gemini 3.6 Flash / Pro)
Hỗ trợ gọi trực tiếp Antigravity CLI hoặc chuyển đổi giữa Claude CLI & Antigravity CLI.
"""

import subprocess
import json
import threading
import time
import uuid
import os
import shutil
from dataclasses import dataclass, field
from typing import Optional, Callable

# Tìm đường dẫn Antigravity CLI / Executable
def _find_antigravity():
    path = shutil.which("agy") or shutil.which("antigravity")
    if path:
        return path
    candidates = [
        r"C:\Users\ASUS\AppData\Local\Programs\Antigravity\Antigravity.exe",
        r"C:\Users\ASUS\AppData\Local\Programs\Antigravity IDE\bin\antigravity-ide.cmd",
        "agy",
        "antigravity"
    ]
    for c in candidates:
        if os.path.isfile(c):
            return c
    return "agy"

ANTIGRAVITY_EXE = _find_antigravity()

# Antigravity Models (Gemini & Claude Thinking via Antigravity)
ANTIGRAVITY_MODELS = {
    "gemini-3.6-flash-high":   "Gemini 3.6 Flash (High)",
    "gemini-3.6-flash-medium": "Gemini 3.6 Flash (Medium)",
    "gemini-3.6-flash-low":    "Gemini 3.6 Flash (Low)",
    "gemini-3.5-flash-high":   "Gemini 3.5 Flash (High)",
    "gemini-3.5-flash-medium": "Gemini 3.5 Flash (Medium)",
    "gemini-3.5-flash-low":    "Gemini 3.5 Flash (Low)",
    "gemini-3.1-pro-high":     "Gemini 3.1 Pro (High)",
    "gemini-3.1-pro-low":      "Gemini 3.1 Pro (Low)",
    "claude-sonnet-4.6-thinking": "Claude Sonnet 4.6 (Thinking)",
    "claude-opus-4.6-thinking":   "Claude Opus 4.6 (Thinking)",
    "default": "gemini-3.6-flash-high"
}


@dataclass
class AntigravityRunResult:
    success:    bool
    output:     str
    raw_json:   Optional[dict] = None
    session_id: Optional[str] = None
    duration_s: float = 0.0
    error:      str = ""
    tokens_used: int = 0


class AntigravityCLI:
    """
    Python wrapper cho Antigravity CLI (Gemini 3.6 Flash/Pro).
    """

    def __init__(self, timeout: int = 300):
        self.timeout = timeout

    def run_once(
        self,
        prompt: str,
        model: str = "gemini-3-6-flash",
        system: str = "",
        on_log: Optional[Callable] = None,
    ) -> AntigravityRunResult:
        full_prompt = f"[System Context: {system}]\n\n{prompt}" if system else prompt
        
        # Truyền model flag
        cmd = [ANTIGRAVITY_EXE, "-p", full_prompt, "--model", model]
        return self._execute(cmd, on_log)

    def _execute(self, cmd: list, on_log: Optional[Callable]) -> AntigravityRunResult:
        if on_log:
            on_log(f"🧠 Antigravity CLI: {' '.join(cmd[:3])}...", "THINKING")

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
                stdin=subprocess.DEVNULL,
                **kwargs
            )

            stdout_lines = []
            stderr_lines = []

            def _read_stdout():
                for line in iter(proc.stdout.readline, ''):
                    if not line:
                        break
                    stdout_lines.append(line)
                    clean = line.strip()
                    if clean and on_log:
                        on_log(f"🚀 [Antigravity] {clean}", "THINKING")

            t_out = threading.Thread(target=_read_stdout, daemon=True)
            t_out.start()
            output_text = "".join(stdout_lines).strip()
            err_text    = "".join(stderr_lines).strip()
            duration    = round(time.time() - t0, 2)

            # Parse real token usage or compute BPE ratio
            import re
            tok_match = re.search(r"(?:total_tokens|tokens_used|usage)\s*[:=]\s*(\d+)", output_text + err_text, re.I)
            if tok_match:
                tokens_count = int(tok_match.group(1))
            else:
                tokens_count = max(1, (len(full_prompt) + len(output_text)) // 4)

            if proc.returncode == 0:
                if on_log:
                    on_log(f"✅ Antigravity CLI hoàn thành ({duration}s, {tokens_count:,} tokens)", "SUCCESS")
                return AntigravityRunResult(
                    success=True, output=output_text, duration_s=duration, tokens_used=tokens_count
                )
            else:
                if on_log:
                    on_log(f"❌ Antigravity CLI lỗi (exit {proc.returncode}): {err_text[:200]}", "ERROR")
                return AntigravityRunResult(
                    success=False, output=output_text, error=err_text, duration_s=duration, tokens_used=tokens_count
                )

        except subprocess.TimeoutExpired:
            proc.kill()
            if on_log:
                on_log(f"⏰ Antigravity CLI timeout sau {self.timeout}s", "ERROR")
            return AntigravityRunResult(
                success=False, output="", error=f"Timeout sau {self.timeout}s"
            )
        except Exception as ex:
            if on_log:
                on_log(f"❌ Antigravity CLI ngoại lệ: {ex}", "ERROR")
            return AntigravityRunResult(
                success=False, output="", error=str(ex)
            )
