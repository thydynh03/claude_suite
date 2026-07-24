"""
agent_registry.py — Quản lý danh sách Agents + SQLite persistence
"""

import sqlite3
import json
import uuid
import os
from dataclasses import dataclass, asdict, field
from typing import Optional, List
from modules.models import Agent, MODEL_TOKEN_LIMITS

def _get_db_path():
    base = r"e:\exe" if os.path.exists(r"e:\exe") else os.path.expanduser("~\\.claude_suite")
    os.makedirs(base, exist_ok=True)
    return os.path.join(base, "agent_manager.db")

DB_PATH = _get_db_path()

BUILTIN_TEMPLATES = [
    {
        "name": "CEO & Executive Director",
        "model": "gemini-3.1-pro-high",
        "icon": "👑",
        "system": """[ROLE]: You are the Chief Executive Officer (CEO) & Board Director.
[CONTEXT]: Directing enterprise digital transformation, product vision, market strategy, and strategic resource allocation.
[ACTION]:
1. Define high-level product strategy, market positioning, and core value proposition.
2. Outline key business milestones, ROI goals, and compliance/governance principles.
3. Provide executive leadership direction for the PM, BA, and Technology Leadership teams.
[NOTE / DIRECTIVE]: Focus on strategic impact, business scalability, risk mitigation, and executive clarity.
[OUTPUT FORMAT]: Executive Strategic Specification Document in Markdown format."""
    },
    {
        "name": "Lead Business Analyst (BA)",
        "model": "claude-opus-4-8",
        "icon": "📋",
        "system": """[ROLE]: You are the Lead Business Analyst (BA) & Domain Expert.
[CONTEXT]: Translating executive business goals into formal Software Requirements Specifications (SRS), Epics, and User Stories.
[ACTION]:
1. Analyze business requirements, domain rules, and user workflows.
2. Draft Epics and granular User Stories with strict Acceptance Criteria using Given-When-Then format.
3. Map edge cases, input validation matrices, and functional data requirements.
[NOTE / DIRECTIVE]: Ensure zero functional ambiguity between business stakeholders and engineering teams.
[OUTPUT FORMAT]: Formal SRS Document with Epics, User Stories, and Acceptance Criteria."""
    },
    {
        "name": "Technical Project Manager (PM)",
        "model": "claude-sonnet-4-5",
        "icon": "🎯",
        "system": """[ROLE]: You are a Senior Technical Project Manager (PM) & Agile Scrum Master.
[CONTEXT]: Orchestrating software execution, Work Breakdown Structure (WBS), sprint planning, and risk management.
[ACTION]:
1. Decompose product requirements into a sequential Work Breakdown Structure (WBS).
2. Assign task priorities (URGENT, HIGH, NORMAL, LOW), dependencies, and estimated effort.
3. Establish Sprint milestones, delivery schedules, and risk mitigation strategies.
[NOTE / DIRECTIVE]: Enforce clear task dependency graphs and prevent project bottlenecks.
[OUTPUT FORMAT]: Agile Sprint Plan, WBS Matrix, and Risk Management Table."""
    },
    {
        "name": "Chief Architect & Tech Lead",
        "model": "claude-opus-4.6-thinking",
        "icon": "🗺️",
        "system": """[ROLE]: You are the Chief Solution Architect & Engineering Tech Lead.
[CONTEXT]: Designing enterprise cloud software architecture, microservices decomposition, database schemas, and API specs.
[ACTION]:
1. Design high-level system architecture and component interactions (C4 Model).
2. Define relational/NoSQL database schemas, indexing strategies, and entity relationships.
3. Specify REST/gRPC API contracts, OpenAPI 3.0 schemas, and security standards.
[NOTE / DIRECTIVE]: Enforce SOLID principles, high availability, horizontal scalability, and clean architecture.
[OUTPUT FORMAT]: Comprehensive Architecture Blueprint Document with Mermaid Diagrams and Schema Definitions."""
    },
    {
        "name": "Senior Fullstack Developer",
        "model": "gemini-3.6-flash-high",
        "icon": "💻",
        "system": """[ROLE]: You are a Senior Principal Fullstack Software Engineer.
[CONTEXT]: Implementing production-ready, type-safe, high-performance software components and APIs.
[ACTION]:
1. Write clean, modular, production-grade source code adhering to design patterns and type hints.
2. Handle error boundaries, exception handling, data sanitization, and logging.
3. Optimize algorithms for CPU/Memory performance and async database operations.
[NOTE / DIRECTIVE]: NEVER use pseudo-code, stub comments, or '// TODO' placeholders. Return 100% complete, runnable code files.
[OUTPUT FORMAT]: Complete, production-ready code files with docstrings and type annotations."""
    },
    {
        "name": "Senior Code Reviewer & Security Auditor",
        "model": "claude-sonnet-4.6-thinking",
        "icon": "🛡️",
        "system": """[ROLE]: You are a Principal Security Auditor & Code Quality Specialist.
[CONTEXT]: Inspecting software codebases for OWASP Top 10 vulnerabilities, performance bottlenecks, and code smells.
[ACTION]:
1. Conduct SAST audits for SQLi, XSS, CSRF, RCE, IDOR, and authentication bypasses.
2. Profile memory usage, thread safety, and execution complexity.
3. Provide concrete code refactoring diffs to patch vulnerabilities and improve quality.
[NOTE / DIRECTIVE]: Classify vulnerabilities by CVSS v3.1 severity ratings (CRITICAL, HIGH, MEDIUM, LOW).
[OUTPUT FORMAT]: Security Audit Report with CVSS scores, line-by-line findings, and drop-in code refactoring diffs."""
    },
    {
        "name": "Lead QA & Test Automation Specialist",
        "model": "gemini-3.6-flash-low",
        "icon": "🧪",
        "system": """[ROLE]: You are the Lead Quality Assurance (QA) & Test Automation Specialist.
[CONTEXT]: Designing comprehensive automated test suites for unit, integration, and E2E system validation.
[ACTION]:
1. Formulate test matrices covering happy paths, negative boundary conditions, and stress tests.
2. Write automated PyTest/Jest test scripts with isolated fixtures, stubs, and mocks.
3. Verify 100% assertion coverage for API endpoints, data models, and business logic.
[NOTE / DIRECTIVE]: Ensure test scripts are runnable out-of-the-box with clear descriptive test names.
[OUTPUT FORMAT]: Complete, runnable PyTest/Jest test suite code files."""
    }
]







class AgentRegistry:
    """
    CRUD cho agents, backed bởi SQLite.
    """

    def __init__(self, db_path: str = DB_PATH):
        self.db_path = db_path
        self._init_db()

    def _init_db(self):
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("""
                CREATE TABLE IF NOT EXISTS agents (
                    agent_id   TEXT PRIMARY KEY,
                    name       TEXT NOT NULL,
                    model      TEXT NOT NULL DEFAULT 'claude-opus-4-8',
                    system     TEXT DEFAULT '',
                    icon       TEXT DEFAULT '🤖',
                    session_id TEXT,
                    status     TEXT DEFAULT 'idle',
                    tasks_done INTEGER DEFAULT 0,
                    last_task  TEXT DEFAULT '',
                    last_error TEXT DEFAULT '',
                    notes      TEXT DEFAULT '',
                    tokens_used INTEGER DEFAULT 0,
                    token_limit INTEGER DEFAULT 200000
                )
            """)
            # Migration check for tokens_used & token_limit
            try:
                conn.execute("ALTER TABLE agents ADD COLUMN tokens_used INTEGER DEFAULT 0")
            except Exception:
                pass
            try:
                conn.execute("ALTER TABLE agents ADD COLUMN token_limit INTEGER DEFAULT 200000")
            except Exception:
                pass
            conn.commit()
        # Ban đầu để trống (blanket default empty). Agents sẽ tự động tạo khi có task yêu cầu.

    # ── CRUD ─────────────────────────────────────────────────────────────

    def create(self, agent: Agent) -> Agent:
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("""
                INSERT INTO agents
                  (agent_id,name,model,system,icon,session_id,status,tasks_done,last_task,last_error,notes,tokens_used,token_limit)
                VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
            """, (agent.agent_id, agent.name, agent.model, agent.system,
                  agent.icon, agent.session_id, agent.status,
                  agent.tasks_done, agent.last_task, agent.last_error, agent.notes,
                  agent.tokens_used, agent.token_limit))
            conn.commit()
        return agent

    def get(self, agent_id: str) -> Optional[Agent]:
        with sqlite3.connect(self.db_path) as conn:
            conn.row_factory = sqlite3.Row
            row = conn.execute(
                "SELECT * FROM agents WHERE agent_id=?", (agent_id,)
            ).fetchone()
        return Agent(**dict(row)) if row else None

    def list_all(self) -> List[Agent]:
        with sqlite3.connect(self.db_path) as conn:
            conn.row_factory = sqlite3.Row
            rows = conn.execute("SELECT * FROM agents ORDER BY name").fetchall()
        return [Agent(**dict(r)) for r in rows]

    def update(self, agent: Agent):
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("""
                UPDATE agents SET
                  name=?, model=?, system=?, icon=?, session_id=?,
                  status=?, tasks_done=?, last_task=?, last_error=?, notes=?,
                  tokens_used=?, token_limit=?
                WHERE agent_id=?
            """, (agent.name, agent.model, agent.system, agent.icon,
                  agent.session_id, agent.status, agent.tasks_done,
                  agent.last_task, agent.last_error, agent.notes,
                  agent.tokens_used, agent.token_limit,
                  agent.agent_id))
            conn.commit()

    def add_tokens(self, agent_id: str, count: int):
        ag = self.get(agent_id)
        if ag:
            ag.tokens_used += count
            self.update(ag)

    def reset_tokens(self, agent_id: str):
        ag = self.get(agent_id)
        if ag:
            ag.tokens_used = 0
            self.update(ag)

    def update_status(self, agent_id: str, status: str,
                      last_task: str = "", last_error: str = "",
                      session_id: str = None, tasks_done_delta: int = 0):
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("""
                UPDATE agents SET status=?,
                  last_task=COALESCE(NULLIF(?,  ''), last_task),
                  last_error=COALESCE(NULLIF(?, ''), last_error),
                  session_id=COALESCE(?, session_id),
                  tasks_done=tasks_done + ?
                WHERE agent_id=?
            """, (status, last_task, last_error, session_id,
                  tasks_done_delta, agent_id))
            conn.commit()

    def reset_session(self, agent_id: str):
        """Xóa session → lần sau sẽ bắt đầu fresh."""
        with sqlite3.connect(self.db_path) as conn:
            conn.execute(
                "UPDATE agents SET session_id=NULL WHERE agent_id=?",
                (agent_id,))
            conn.commit()

    def delete(self, agent_id: str):
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("DELETE FROM agents WHERE agent_id=?", (agent_id,))
            conn.commit()

    def reset_to_defaults(self):
        """Xóa TẤT CẢ agents và seed lại 7 Enterprise Corporate Roles mặc định."""
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("DELETE FROM agents")
            conn.commit()
        for t in BUILTIN_TEMPLATES:
            self.create(Agent(
                name=t["name"], model=t["model"],
                system=t["system"], icon=t["icon"]
            ))

    def delete_all(self):
        """Xóa toàn bộ agents trong database."""
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("DELETE FROM agents")
            conn.commit()

    def list_idle(self) -> List[Agent]:
        return [a for a in self.list_all() if a.status == "idle"]

    def get_by_model_pref(self, preferred_models: list) -> Optional[Agent]:
        """Tìm agent idle phù hợp nhất."""
        idle = self.list_idle()
        for model in preferred_models:
            for a in idle:
                if model in a.model:
                    return a
        return idle[0] if idle else None

    def get_or_create_for_task(self, task=None, title: str = "", prompt: str = "") -> Agent:
        """
        Tự động tìm hoặc khởi tạo Agent phù hợp dựa trên yêu cầu công việc và tag.
        """
        text_lower = ""
        tags = []
        if task:
            title = getattr(task, 'title', title)
            prompt = getattr(task, 'prompt', prompt)
            tags = getattr(task, 'tags', [])
        
        text_lower = (title + " " + prompt).lower()

        # Keyword mapping to builtin roles
        keyword_role_map = [
            (["ceo", "vision", "strategy", "tiêu chuẩn 5 phần", "mô hình kinh doanh"], "CEO & Executive Director"),
            (["ba", "srs", "user story", "requirements", "phân tích yêu cầu", "nghiệp vụ"], "Lead Business Analyst (BA)"),
            (["pm", "wbs", "sprint", "project manager", "quản lý dự án", "tiến độ"], "Technical Project Manager (PM)"),
            (["architect", "architecture", "kiến trúc", "tech lead", "hệ thống", "c4", "schema"], "Chief Architect & Tech Lead"),
            (["security", "audit", "owasp", "bảo mật", "lỗ hổng", "sast"], "Senior Code Reviewer & Security Auditor"),
            (["test", "pytest", "unit test", "qa", "kiem thu", "automation"], "Lead QA & Test Automation Specialist"),
            (["dev", "feature", "code", "implement", "chức năng", "lập trình"], "Senior Fullstack Developer")
        ]

        target_template_name = "Senior Fullstack Developer"
        
        # 1. Ánh xạ từ Tag (Ưu tiên cao nhất)
        if "plan" in tags or "architect" in tags:
            target_template_name = "Chief Architect & Tech Lead"
        elif "code" in tags:
            target_template_name = "Senior Fullstack Developer"
        elif "test" in tags:
            target_template_name = "Lead QA & Test Automation Specialist"
        elif "review" in tags:
            target_template_name = "Senior Code Reviewer & Security Auditor"
        elif "research" in tags:
            target_template_name = "Lead Business Analyst (BA)"
        else:
            # 2. Ánh xạ từ Keywords
            for keywords, role_name in keyword_role_map:
                if any(kw in text_lower for kw in keywords):
                    target_template_name = role_name
                    break

        # Check if an agent matching target_template_name is already in DB
        all_agents = self.list_all()
        for a in all_agents:
            if target_template_name.lower() in a.name.lower():
                return a

        # Nếu chưa có Agent phù hợp với role này trong DB, tự động sinh Agent mới dựa trên template
        matched_tmpl = None
        for t in BUILTIN_TEMPLATES:
            if t["name"] == target_template_name:
                matched_tmpl = t
                break
        if not matched_tmpl:
            matched_tmpl = BUILTIN_TEMPLATES[3] # Senior Fullstack Developer default

        new_agent = self.create(Agent(
            name=matched_tmpl["name"],
            model=matched_tmpl["model"],
            system=matched_tmpl["system"],
            icon=matched_tmpl["icon"]
        ))
        return new_agent

