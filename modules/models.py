"""
models.py — Common Dataclasses & Core Type Definitions for Claude Suite.
"""

import uuid
from dataclasses import dataclass, field
from typing import Optional, List

MODEL_TOKEN_LIMITS = {
    "gemini-3.6-flash-high": 1000000,
    "gemini-3.6-flash-low": 1000000,
    "gemini-3.5-flash-high": 1000000,
    "gemini-3.5-flash-low": 1000000,
    "gemini-3.1-pro-high": 2000000,
    "gemini-3.1-pro-low": 2000000,
    "claude-opus-4-8": 200000,
    "claude-sonnet-4-5": 200000,
    "claude-haiku-4-5": 200000,
    "claude-sonnet-4.6-thinking": 200000,
    "claude-opus-4.6-thinking": 200000
}


@dataclass
class Agent:
    agent_id:   str   = field(default_factory=lambda: str(uuid.uuid4())[:8])
    name:       str   = "New Agent"
    model:      str   = "claude-opus-4-8"
    system:     str   = ""
    icon:       str   = "🤖"
    session_id: Optional[str] = None
    status:     str   = "idle"      # idle | running | done | error
    tasks_done: int   = 0
    last_task:  str   = ""
    last_error: str   = ""
    notes:      str   = ""
    tokens_used: int  = 0
    token_limit: int  = 200000

    @property
    def status_dot(self) -> str:
        return {"idle": "🟡", "running": "🟢", "done": "✅", "error": "🔴"}.get(self.status, "⚪")

    @property
    def model_short(self) -> str:
        m = self.model
        if "gemini-3.6" in m: return "Gemini 3.6 Flash"
        if "gemini-3.5" in m: return "Gemini 3.5 Flash"
        if "gemini-3.1" in m: return "Gemini 3.1 Pro"
        if "sonnet-4.6" in m: return "Claude Sonnet 4.6"
        if "opus-4.6" in m:   return "Claude Opus 4.6"
        if "antigravity" in m or "agy" in m: return "Antigravity"
        if "opus" in m:   return "Opus 4.8"
        if "sonnet" in m: return "Sonnet 4.5"
        if "haiku" in m:  return "Haiku 4.5"
        return m.split("-")[1] if "-" in m else m

    @property
    def provider(self) -> str:
        """Trả về provider dựa trên model name"""
        m = self.model.lower()
        if any(k in m for k in ["gemini", "thinking", "antigravity", "agy"]):
            return "anti_cli"
        return "claude_cli"

    @property
    def token_limit_max(self) -> int:
        return MODEL_TOKEN_LIMITS.get(self.model, self.token_limit or 200000)

    @property
    def token_remaining(self) -> int:
        return max(0, self.token_limit_max - self.tokens_used)

    @property
    def token_usage_percent(self) -> float:
        max_t = self.token_limit_max
        if max_t <= 0: return 0.0
        return min(100.0, (self.tokens_used / max_t) * 100.0)

    @property
    def token_display(self) -> str:
        used_k = self.tokens_used / 1000.0
        max_k = self.token_limit_max / 1000.0
        return f"{used_k:.1f}k / {max_k:.0f}k ({self.token_usage_percent:.1f}%)"

    def fallback_to_antigravity(self) -> dict:
        prev_model = self.model
        self.model = "antigravity-flash-3.0"
        self.notes = f"⚠️ Smart Fallback: Switched from {prev_model} to Antigravity CLI"
        return {"previous_model": prev_model, "new_model": self.model}


import datetime
import json

@dataclass
class Task:
    task_id:       str  = field(default_factory=lambda: str(uuid.uuid4())[:8])
    title:         str  = "New Task"
    description:   str  = ""
    prompt:        str  = ""          # Prompt gửi cho Claude
    priority:      str  = "normal"
    status:        str  = "backlog"   # backlog | queued | running | done | failed
    assigned_to:   Optional[str] = None # agent_id
    depends_on:    str  = "[]"        # JSON list of task_ids or List
    retry_count:   int  = 0
    max_retries:   int  = 3
    result:        str  = ""
    session_id:    Optional[str] = None
    parent_id:     Optional[str] = None  # For subtasks
    created_at:    str  = field(default_factory=lambda: datetime.datetime.now().isoformat())
    started_at:    Optional[str] = None
    finished_at:   Optional[str] = None

    @property
    def depends_on_list(self) -> List[str]:
        if isinstance(self.depends_on, list):
            return self.depends_on
        try:
            return json.loads(self.depends_on)
        except Exception:
            return []

    @property
    def tags(self) -> List[str]:
        """Tự động nội suy tags từ title hoặc lấy trực tiếp tag [xxx]"""
        import re
        t = (self.title + " " + self.description + " " + self.prompt).lower()
        
        found = []
        # Các tag tường minh [tag]
        matches = re.findall(r"\[([a-z0-9_]+)\]", t)
        if matches:
            found.extend(matches)
            
        # Nội suy thông minh hơn với nhiều từ khóa mở rộng
        keyword_map = {
            "plan": ["plan", "lên kế hoạch", "brainstorm", "architect", "thiết kế", "kiến trúc", "schema", "database", "wbs"],
            "code": ["code", "dev", "lập trình", "implement", "viết", "xây dựng", "tạo", "build", "tích hợp", "fix bug"],
            "test": ["test", "kiểm thử", "qa", "unit test", "e2e", "integration test", "chạy thử"],
            "review": ["review", "đánh giá", "kiểm tra", "audit", "security", "bảo mật", "refactor", "tối ưu"],
            "research": ["research", "nghiên cứu", "tìm hiểu", "đọc", "phân tích", "so sánh", "khảo sát"],
            "doc": ["doc", "tài liệu", "readme", "hướng dẫn", "ghi chú", "báo cáo"],
            "deploy": ["deploy", "triển khai", "release", "publish", "build production", "docker", "ci/cd"]
        }
        
        for tag, keywords in keyword_map.items():
            if any(w in t for w in keywords):
                found.append(tag)
            
        # De-duplicate
        return list(dict.fromkeys(found))

    @property
    def status_icon(self) -> str:
        return {"backlog": "📋", "queued": "⏳", "running": "⚡", "done": "✅", "failed": "❌"}.get(self.status, "❓")

    @property
    def priority_color(self) -> str:
        return {"low": "#8b949e", "normal": "#79c0ff", "high": "#e3b341", "urgent": "#f85149"}.get(self.priority, "#8b949e")

    @property
    def duration(self) -> Optional[str]:
        if self.started_at and self.finished_at:
            try:
                s = datetime.datetime.fromisoformat(self.started_at)
                f = datetime.datetime.fromisoformat(self.finished_at)
                sec = (f - s).total_seconds()
                return f"{sec:.1f}s"
            except Exception:
                pass
        return None


@dataclass
class MemoryItem:
    memory_id:   str = field(default_factory=lambda: str(uuid.uuid4())[:8])
    agent_id:    str = ""
    task_id:     Optional[str] = None
    session_id:  Optional[str] = None
    role:        str = "user"    # user | assistant | system
    content:     str = ""
    timestamp:   str = ""
    tokens_used: int = 0


@dataclass
class PipelineStep:
    step_id:     str = ""
    role_name:   str = ""
    agent_id:    Optional[str] = None
    prompt_template: str = ""
    status:      str = "pending" # pending | running | completed | failed
    output:      str = ""


@dataclass
class PlanTask:
    title:       str = ""
    description: str = ""
    prompt:      str = ""
    priority:    str = "normal"
    depends_on:  List[str] = field(default_factory=list)
    model_hint:  str = "claude-opus-4-8"

    @property
    def tags(self) -> List[str]:
        import re
        t = (self.title + " " + self.description + " " + self.prompt).lower()
        
        found = []
        matches = re.findall(r"\[([a-z0-9_]+)\]", t)
        if matches: found.extend(matches)
            
        keyword_map = {
            "plan": ["plan", "lên kế hoạch", "brainstorm", "architect", "thiết kế", "kiến trúc", "schema", "database", "wbs"],
            "code": ["code", "dev", "lập trình", "implement", "viết", "xây dựng", "tạo", "build", "tích hợp", "fix bug"],
            "test": ["test", "kiểm thử", "qa", "unit test", "e2e", "integration test", "chạy thử"],
            "review": ["review", "đánh giá", "kiểm tra", "audit", "security", "bảo mật", "refactor", "tối ưu"],
            "research": ["research", "nghiên cứu", "tìm hiểu", "đọc", "phân tích", "so sánh", "khảo sát"],
            "doc": ["doc", "tài liệu", "readme", "hướng dẫn", "ghi chú", "báo cáo"],
            "deploy": ["deploy", "triển khai", "release", "publish", "build production", "docker", "ci/cd"]
        }
        
        for tag, keywords in keyword_map.items():
            if any(w in t for w in keywords): found.append(tag)
            
        return list(dict.fromkeys(found))

    @property
    def provider(self) -> str:
        m = self.model_hint.lower()
        if any(k in m for k in ["gemini", "thinking", "antigravity", "agy"]):
            return "anti_cli"
        return "claude_cli"
