#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Claude Agent Manager v1.0
Desktop app quan ly Claude agents, tasks, pipelines
"""

import sys, io, os
if sys.stdout is not None:
    sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8', errors='replace')
if sys.stderr is not None:
    sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding='utf-8', errors='replace')

# Ensure modules/ is on path
sys.path.insert(0, os.path.dirname(__file__))

import tkinter as tk
from tkinter import ttk, messagebox, scrolledtext, simpledialog
import threading
import datetime
import time

from modules.agent_registry import AgentRegistry, Agent, BUILTIN_TEMPLATES
from modules.task_board import TaskBoard, Task, STATUSES, STATUS_ICONS, PRIORITY_COLORS
from modules.claude_cli import ClaudeCLI, MODELS
from modules.orchestrator import Orchestrator
from modules.plan_builder import PlanBuilder

# ── Theme ─────────────────────────────────────────────────────────────────────
THEMES = {
    "dark": {
        "bg":         "#0d1117",
        "surface":    "#161b22",
        "surface2":   "#21262d",
        "border":     "#30363d",
        "accent":     "#da3633",
        "accent2":    "#79c0ff",
        "text":       "#e6edf3",
        "text_muted": "#8b949e",
        "success":    "#3fb950",
        "warning":    "#e3b341",
        "error":      "#f85149",
        "input_bg":   "#010409",
        "tab_sel":    "#1f6feb",
    },
    "light": {
        "bg":         "#f6f8fa",
        "surface":    "#ffffff",
        "surface2":   "#eaeef2",
        "border":     "#d0d7de",
        "accent":     "#cf222e",
        "accent2":    "#0969da",
        "text":       "#1f2328",
        "text_muted": "#57606a",
        "success":    "#1a7f37",
        "warning":    "#9a6700",
        "error":      "#cf222e",
        "input_bg":   "#ffffff",
        "tab_sel":    "#0969da",
    }
}
current_theme = "dark"
def T(): return THEMES[current_theme]

# ── Log colour tags ────────────────────────────────────────────────────────────
LOG_TAGS = {
    "SUCCESS": {"dark": "#3fb950", "light": "#1a7f37"},
    "ERROR":   {"dark": "#f85149", "light": "#cf222e"},
    "WARN":    {"dark": "#e3b341", "light": "#9a6700"},
    "TIER1":   {"dark": "#d2a8ff", "light": "#8250df"},
    "TIER2":   {"dark": "#ffa657", "light": "#bc4c00"},
    "SEND":    {"dark": "#79c0ff", "light": "#0969da"},
    "INFO":    {"dark": "#8b949e", "light": "#57606a"},
    "SEP":     {"dark": "#30363d", "light": "#d0d7de"},
    "TIME":    {"dark": "#484f58", "light": "#6e7781"},
}


# ═══════════════════════════════════════════════════════════════════════════════
class AgentManagerApp(tk.Tk):
    def __init__(self):
        super().__init__()
        self.title("Claude Agent Manager")
        self.geometry("960x720")
        self.resizable(True, True)
        self.minsize(780, 560)

        # Core services
        self.cli        = ClaudeCLI(timeout=300)
        self.registry   = AgentRegistry()
        self.board      = TaskBoard()
        self.plan_bld   = PlanBuilder(cli=self.cli)
        self.orchestr   = Orchestrator(self.registry, self.board, self.cli,
                                       on_log=self._global_log)
        self.log_count  = 0
        self._refresh_job = None

        self._build_ui()
        self._apply_theme()
        self._start_refresh()

    # ── Build UI ───────────────────────────────────────────────────────────

    def _build_ui(self):
        # ── Header ──
        self.header = tk.Frame(self, height=52)
        self.header.pack(fill="x")
        self.header.pack_propagate(False)

        self.lbl_title = tk.Label(self.header,
            text="🤖  Claude Agent Manager",
            font=("Segoe UI", 13, "bold"))
        self.lbl_title.pack(side="left", padx=18, pady=12)

        # Status pill (orchestrator state)
        self.lbl_orch = tk.Label(self.header, text="● Stopped",
            font=("Segoe UI", 9))
        self.lbl_orch.pack(side="left", padx=(0, 12), pady=12)

        self.btn_theme = tk.Button(self.header, text="☀ Light",
            font=("Segoe UI", 9), relief="flat", bd=0,
            padx=10, pady=4, cursor="hand2", command=self._toggle_theme)
        self.btn_theme.pack(side="right", padx=14, pady=12)

        self.btn_orch = tk.Button(self.header, text="▶ Start Orchestrator",
            font=("Segoe UI", 9, "bold"), relief="flat", bd=0,
            padx=12, pady=5, cursor="hand2", command=self._toggle_orchestrator)
        self.btn_orch.pack(side="right", padx=(0, 8), pady=10)

        self._divider = tk.Frame(self, height=1)
        self._divider.pack(fill="x")

        # ── Notebook tabs ──
        style = ttk.Style(self)
        style.theme_use("clam")
        self._configure_style(style)

        self.notebook = ttk.Notebook(self)
        self.notebook.pack(fill="both", expand=True)

        self.tab_agents  = tk.Frame(self.notebook)
        self.tab_tasks   = tk.Frame(self.notebook)
        self.tab_plan    = tk.Frame(self.notebook)
        self.tab_run     = tk.Frame(self.notebook)
        self.tab_log     = tk.Frame(self.notebook)

        self.notebook.add(self.tab_agents, text="🤖  Agents")
        self.notebook.add(self.tab_tasks,  text="📋  Tasks")
        self.notebook.add(self.tab_plan,   text="🗺  Plan Builder")
        self.notebook.add(self.tab_run,    text="⚡  Quick Run")
        self.notebook.add(self.tab_log,    text="📊  Live Log")

        self._build_agents_tab()
        self._build_tasks_tab()
        self._build_plan_tab()
        self._build_run_tab()
        self._build_log_tab()

    # ══════════════════════════════════════════════════════════════════════
    # TAB 1 — AGENTS
    # ══════════════════════════════════════════════════════════════════════

    def _build_agents_tab(self):
        t = self.tab_agents
        # Toolbar
        tb = tk.Frame(t)
        tb.pack(fill="x", padx=12, pady=(10, 4))
        tk.Label(tb, text="AGENTS", font=("Segoe UI", 9, "bold")).pack(side="left")

        tk.Button(tb, text="+ New Agent", font=("Segoe UI", 9),
            relief="flat", bd=0, padx=10, pady=4, cursor="hand2",
            command=self._new_agent).pack(side="right")
        tk.Button(tb, text="🔄 Refresh", font=("Segoe UI", 9),
            relief="flat", bd=0, padx=10, pady=4, cursor="hand2",
            command=self._refresh_agents).pack(side="right", padx=4)

        # Agent list (canvas + scrollbar for card grid)
        self.agents_frame = tk.Frame(t)
        self.agents_frame.pack(fill="both", expand=True, padx=12, pady=4)

        self.agents_canvas = tk.Canvas(self.agents_frame, highlightthickness=0)
        self.agents_vsb    = tk.Scrollbar(self.agents_frame, orient="vertical",
                                          command=self.agents_canvas.yview)
        self.agents_canvas.configure(yscrollcommand=self.agents_vsb.set)
        self.agents_vsb.pack(side="right", fill="y")
        self.agents_canvas.pack(fill="both", expand=True)

        self.agents_inner = tk.Frame(self.agents_canvas)
        self.agents_canvas_win = self.agents_canvas.create_window(
            (0, 0), window=self.agents_inner, anchor="nw")
        self.agents_inner.bind("<Configure>", lambda e: (
            self.agents_canvas.configure(
                scrollregion=self.agents_canvas.bbox("all")),
            self.agents_canvas.itemconfig(
                self.agents_canvas_win,
                width=self.agents_canvas.winfo_width())
        ))
        self.agents_canvas.bind("<Configure>", lambda e:
            self.agents_canvas.itemconfig(
                self.agents_canvas_win, width=e.width))

        self._refresh_agents()

    def _refresh_agents(self):
        for w in self.agents_inner.winfo_children():
            w.destroy()

        agents = self.registry.list_all()
        if not agents:
            tk.Label(self.agents_inner,
                text="Chua co agent nao. Nhan '+ New Agent' de tao.",
                font=("Segoe UI", 10), fg=T()["text_muted"],
                bg=T()["bg"]).pack(pady=40)
            return

        for i, agent in enumerate(agents):
            self._agent_card(self.agents_inner, agent, i)

    def _agent_card(self, parent, agent: Agent, idx: int):
        t = T()
        card = tk.Frame(parent, relief="flat", bd=0)
        card.pack(fill="x", pady=3)

        inner = tk.Frame(card, relief="flat", bd=1)
        inner.pack(fill="x")

        # Status dot + name
        left = tk.Frame(inner)
        left.pack(side="left", fill="y", padx=12, pady=10)

        dot_color = {"idle": t["warning"], "running": t["success"],
                     "done": t["accent2"], "error": t["error"]}.get(agent.status, t["text_muted"])
        tk.Label(left, text="●", font=("Segoe UI", 20),
            fg=dot_color, bg=t["surface"]).pack(side="left", padx=(0, 10))

        info = tk.Frame(left, bg=t["surface"])
        info.pack(side="left")
        tk.Label(info, text=f"{agent.icon} {agent.name}",
            font=("Segoe UI", 11, "bold"),
            fg=t["text"], bg=t["surface"]).pack(anchor="w")
        tk.Label(info, text=f"{agent.model_short}  ·  {agent.status.upper()}  ·  {agent.tasks_done} tasks done",
            font=("Segoe UI", 8), fg=t["text_muted"], bg=t["surface"]).pack(anchor="w")
        if agent.last_task:
            tk.Label(info, text=f"Last: {agent.last_task[:55]}",
                font=("Segoe UI", 8), fg=t["text_muted"], bg=t["surface"]).pack(anchor="w")

        # Buttons right
        right = tk.Frame(inner, bg=t["surface"])
        right.pack(side="right", padx=10, pady=10)

        tk.Button(right, text="▶ Run", font=("Segoe UI", 8),
            relief="flat", bd=0, padx=8, pady=3, cursor="hand2",
            command=lambda a=agent: self._run_agent_dialog(a)).pack(side="left", padx=2)
        tk.Button(right, text="✏ Edit", font=("Segoe UI", 8),
            relief="flat", bd=0, padx=8, pady=3, cursor="hand2",
            command=lambda a=agent: self._edit_agent(a)).pack(side="left", padx=2)
        tk.Button(right, text="🔄 Reset", font=("Segoe UI", 8),
            relief="flat", bd=0, padx=8, pady=3, cursor="hand2",
            command=lambda a=agent: self._reset_agent_session(a)).pack(side="left", padx=2)
        tk.Button(right, text="🗑", font=("Segoe UI", 8),
            relief="flat", bd=0, padx=6, pady=3, cursor="hand2",
            fg=T()["error"],
            command=lambda a=agent: self._delete_agent(a)).pack(side="left", padx=2)

        # Style card
        for w in [card, inner, left, info, right]:
            try: w.configure(bg=t["surface"])
            except: pass

    # ══════════════════════════════════════════════════════════════════════
    # TAB 2 — TASKS (Kanban)
    # ══════════════════════════════════════════════════════════════════════

    def _build_tasks_tab(self):
        t = self.tab_tasks
        tb = tk.Frame(t)
        tb.pack(fill="x", padx=12, pady=(10, 4))
        tk.Label(tb, text="TASK BOARD", font=("Segoe UI", 9, "bold")).pack(side="left")
        tk.Button(tb, text="+ Add Task", font=("Segoe UI", 9),
            relief="flat", bd=0, padx=10, pady=4, cursor="hand2",
            command=self._new_task_dialog).pack(side="right")
        tk.Button(tb, text="🗑 Clear Done", font=("Segoe UI", 9),
            relief="flat", bd=0, padx=10, pady=4, cursor="hand2",
            command=self._clear_done_tasks).pack(side="right", padx=4)

        # Kanban columns
        self.kanban_frame = tk.Frame(t)
        self.kanban_frame.pack(fill="both", expand=True, padx=8, pady=4)

        self.kanban_cols  = {}
        self.kanban_lists = {}
        col_cfg = [
            ("backlog",  "📋 BACKLOG",   0),
            ("queued",   "⏳ QUEUED",    1),
            ("running",  "⚡ RUNNING",   2),
            ("done",     "✅ DONE",      3),
            ("failed",   "❌ FAILED",    4),
        ]
        for status, label, col in col_cfg:
            self.kanban_frame.columnconfigure(col, weight=1, uniform="col")
            f = tk.Frame(self.kanban_frame)
            f.grid(row=0, column=col, sticky="nsew", padx=4, pady=4)
            self.kanban_frame.rowconfigure(0, weight=1)

            hdr = tk.Frame(f)
            hdr.pack(fill="x", pady=(0, 4))
            self.kanban_cols[status] = tk.Label(hdr, text=label,
                font=("Segoe UI", 8, "bold"))
            self.kanban_cols[status].pack(side="left")

            lbox_frame = tk.Frame(f)
            lbox_frame.pack(fill="both", expand=True)
            vsb = tk.Scrollbar(lbox_frame)
            vsb.pack(side="right", fill="y")
            lb = tk.Listbox(lbox_frame, relief="flat", bd=0,
                font=("Segoe UI", 9), selectmode="single",
                yscrollcommand=vsb.set, activestyle="none")
            lb.pack(fill="both", expand=True)
            vsb.config(command=lb.yview)
            lb.bind("<Double-Button-1>",
                    lambda e, s=status: self._task_detail(s))
            self.kanban_lists[status] = lb

        self._refresh_tasks()

    def _refresh_tasks(self):
        counts = self.board.counts()
        for status in STATUSES:
            lb = self.kanban_lists.get(status)
            if not lb: continue
            lb.delete(0, "end")
            tasks = self.board.list_by_status(status)
            count = counts.get(status, 0)
            if status in self.kanban_cols:
                labels = {"backlog":"📋 BACKLOG","queued":"⏳ QUEUED",
                          "running":"⚡ RUNNING","done":"✅ DONE","failed":"❌ FAILED"}
                self.kanban_cols[status].configure(
                    text=f"{labels.get(status,status)}  ({count})")
            for task in tasks:
                lb.insert("end", f"{task.status_icon} [{task.priority[:1].upper()}] {task.title}")

    # ══════════════════════════════════════════════════════════════════════
    # TAB 3 — PLAN BUILDER
    # ══════════════════════════════════════════════════════════════════════

    def _build_plan_tab(self):
        t = self.tab_plan
        # Left: input
        paned = tk.PanedWindow(t, orient="horizontal", sashwidth=6)
        paned.pack(fill="both", expand=True, padx=8, pady=8)

        left = tk.Frame(paned)
        paned.add(left, minsize=280)

        tk.Label(left, text="MÔ TẢ PROJECT",
            font=("Segoe UI", 9, "bold")).pack(anchor="w", padx=8, pady=(8, 4))
        self.plan_input = scrolledtext.ScrolledText(left, height=8,
            font=("Segoe UI", 10), relief="flat", wrap="word", padx=8, pady=6)
        self.plan_input.pack(fill="both", expand=True, padx=8)
        self.plan_input.insert("1.0",
            "Example: Build a REST API with authentication, database, and tests")

        btn_row = tk.Frame(left)
        btn_row.pack(fill="x", padx=8, pady=8)
        self.btn_decompose = tk.Button(btn_row, text="🗺 AI Decompose",
            font=("Segoe UI", 10, "bold"), relief="flat", bd=0,
            padx=14, pady=8, cursor="hand2", command=self._run_decompose)
        self.btn_decompose.pack(side="left")
        tk.Button(btn_row, text="📝 Simple Split", font=("Segoe UI", 9),
            relief="flat", bd=0, padx=10, pady=8, cursor="hand2",
            command=self._simple_split).pack(side="left", padx=6)

        self.plan_status = tk.Label(left, text="", font=("Segoe UI", 8),
            wraplength=250, justify="left")
        self.plan_status.pack(anchor="w", padx=8)

        # Right: generated tasks
        right = tk.Frame(paned)
        paned.add(right, minsize=380)

        hdr = tk.Frame(right)
        hdr.pack(fill="x", padx=8, pady=(8, 4))
        tk.Label(hdr, text="TASKS ĐƯỢC TẠO",
            font=("Segoe UI", 9, "bold")).pack(side="left")
        self.btn_execute_plan = tk.Button(hdr, text="▶ Execute Plan",
            font=("Segoe UI", 9, "bold"), relief="flat", bd=0,
            padx=12, pady=4, cursor="hand2", command=self._execute_plan,
            state="disabled")
        self.btn_execute_plan.pack(side="right")
        tk.Button(hdr, text="🗑 Clear", font=("Segoe UI", 9),
            relief="flat", bd=0, padx=8, pady=4, cursor="hand2",
            command=self._clear_plan).pack(side="right", padx=4)

        plan_cont = tk.Frame(right)
        plan_cont.pack(fill="both", expand=True, padx=8, pady=(0, 8))
        vsb = tk.Scrollbar(plan_cont)
        vsb.pack(side="right", fill="y")
        self.plan_list = tk.Text(plan_cont, font=("Consolas", 9),
            relief="flat", bd=0, state="disabled", wrap="word",
            padx=8, pady=6, yscrollcommand=vsb.set)
        self.plan_list.pack(fill="both", expand=True)
        vsb.config(command=self.plan_list.yview)

        self._pending_plan = []

    # ══════════════════════════════════════════════════════════════════════
    # TAB 4 — QUICK RUN
    # ══════════════════════════════════════════════════════════════════════

    def _build_run_tab(self):
        t = self.tab_run
        paned = tk.PanedWindow(t, orient="vertical", sashwidth=6)
        paned.pack(fill="both", expand=True, padx=8, pady=8)

        top = tk.Frame(paned)
        paned.add(top, minsize=200)

        # Agent selector + prompt
        row = tk.Frame(top)
        row.pack(fill="x", padx=8, pady=(8, 4))
        tk.Label(row, text="Agent:", font=("Segoe UI", 10)).pack(side="left")
        self.run_agent_var = tk.StringVar()
        self.run_agent_cb  = ttk.Combobox(row, textvariable=self.run_agent_var,
            font=("Segoe UI", 10), state="readonly", width=28)
        self.run_agent_cb.pack(side="left", padx=8)
        self._refresh_agent_combobox()

        tk.Label(row, text="Model:", font=("Segoe UI", 10)).pack(side="left", padx=(12, 4))
        self.run_model_var = tk.StringVar(value="claude-opus-4-8")
        model_cb = ttk.Combobox(row, textvariable=self.run_model_var,
            font=("Segoe UI", 9), width=20,
            values=["claude-opus-4-8", "claude-sonnet-4-5", "claude-haiku-4-5"])
        model_cb.pack(side="left")

        tk.Label(top, text="PROMPT", font=("Segoe UI", 9, "bold")).pack(
            anchor="w", padx=10, pady=(8, 2))
        self.run_prompt = scrolledtext.ScrolledText(top, height=6,
            font=("Segoe UI", 10), relief="flat", wrap="word", padx=8, pady=6)
        self.run_prompt.pack(fill="both", expand=True, padx=8)

        btn_row = tk.Frame(top)
        btn_row.pack(fill="x", padx=8, pady=8)
        self.btn_run = tk.Button(btn_row, text="▶  Run Now",
            font=("Segoe UI", 11, "bold"), relief="flat", bd=0,
            padx=18, pady=9, cursor="hand2", command=self._quick_run)
        self.btn_run.pack(side="left")
        tk.Button(btn_row, text="🔍 Find Claude Windows",
            font=("Segoe UI", 9), relief="flat", bd=0,
            padx=10, pady=9, cursor="hand2",
            command=self._find_claude_windows).pack(side="left", padx=8)

        # Result area
        bottom = tk.Frame(paned)
        paned.add(bottom, minsize=120)
        tk.Label(bottom, text="RESULT", font=("Segoe UI", 9, "bold")).pack(
            anchor="w", padx=10, pady=(8, 2))
        self.run_result = scrolledtext.ScrolledText(bottom,
            font=("Consolas", 9), relief="flat", bd=0,
            state="disabled", wrap="word", padx=8, pady=6)
        self.run_result.pack(fill="both", expand=True, padx=8, pady=(0, 8))

    # ══════════════════════════════════════════════════════════════════════
    # TAB 5 — LIVE LOG
    # ══════════════════════════════════════════════════════════════════════

    def _build_log_tab(self):
        t = self.tab_log
        hdr = tk.Frame(t)
        hdr.pack(fill="x", padx=12, pady=(8, 4))

        self.lbl_log_title = tk.Label(hdr, text="LOG HOAT DONG",
            font=("Segoe UI", 9, "bold"))
        self.lbl_log_title.pack(side="left")

        self.lbl_log_cnt = tk.Label(hdr, text="0 dong", font=("Segoe UI", 8))
        self.lbl_log_cnt.pack(side="left", padx=(8, 0))

        tk.Button(hdr, text="Copy", font=("Segoe UI", 8),
            relief="flat", bd=0, padx=8, pady=3, cursor="hand2",
            command=self._copy_log).pack(side="right", padx=(4, 0))
        tk.Button(hdr, text="Xoa", font=("Segoe UI", 8),
            relief="flat", bd=0, padx=8, pady=3, cursor="hand2",
            command=self._clear_log).pack(side="right")

        # Filter row
        frow = tk.Frame(t)
        frow.pack(fill="x", padx=12, pady=(0, 4))
        self.log_filter_var = tk.StringVar(value="ALL")
        for lvl in ["ALL", "SUCCESS", "ERROR", "WARN", "INFO", "TIER1", "TIER2"]:
            tk.Radiobutton(frow, text=lvl, variable=self.log_filter_var,
                value=lvl, font=("Segoe UI", 8), relief="flat",
                command=self._apply_log_filter).pack(side="left", padx=2)

        cont = tk.Frame(t)
        cont.pack(fill="both", expand=True, padx=12, pady=(0, 8))
        vsb = tk.Scrollbar(cont)
        vsb.pack(side="right", fill="y")
        self.log_text = tk.Text(cont, font=("Consolas", 9),
            relief="flat", bd=0, state="disabled", wrap="word",
            padx=10, pady=8, yscrollcommand=vsb.set)
        self.log_text.pack(fill="both", expand=True)
        vsb.config(command=self.log_text.yview)

        # Configure colour tags
        self._setup_log_tags()

    def _setup_log_tags(self):
        th = current_theme
        for tag, colors in LOG_TAGS.items():
            self.log_text.tag_configure(tag, foreground=colors[th])
        self.log_text.tag_configure("BOLD", font=("Consolas", 9, "bold"))

    # ── Global log ─────────────────────────────────────────────────────────

    BADGES = {"SUCCESS":" OK  ","ERROR":" ERR ","WARN":" WRN ",
              "TIER1":" T1  ","TIER2":" T2  ","SEND":" SND ",
              "INFO":" --- ","SEP":""}

    def _global_log(self, msg: str, level: str = "INFO"):
        if not hasattr(self, "log_text"):
            return
        ts = datetime.datetime.now().strftime("%H:%M:%S")
        self.log_count += 1

        def _insert():
            self.log_text.configure(state="normal")
            if level == "SEP":
                self.log_text.insert("end", msg + "\n", "SEP")
            else:
                badge = self.BADGES.get(level, " --- ")
                self.log_text.insert("end", f"[{ts}] ", "TIME")
                self.log_text.insert("end", badge, (level, "BOLD"))
                self.log_text.insert("end", f"  {msg}\n", level)
            self.log_text.see("end")
            self.log_text.configure(state="disabled")
            self.lbl_log_cnt.configure(text=f"{self.log_count} dong")

        self.after(0, _insert)

    def _apply_log_filter(self):
        pass  # TODO: implement filter by tag visibility

    def _clear_log(self):
        self.log_text.configure(state="normal")
        self.log_text.delete("1.0", "end")
        self.log_text.configure(state="disabled")
        self.log_count = 0
        self.lbl_log_cnt.configure(text="0 dong")

    def _copy_log(self):
        content = self.log_text.get("1.0", "end").strip()
        self.clipboard_clear()
        self.clipboard_append(content)
        self._global_log("Da copy log!", "SUCCESS")

    # ── Agent actions ──────────────────────────────────────────────────────

    def _new_agent(self):
        self._agent_edit_window(None)

    def _edit_agent(self, agent: Agent):
        self._agent_edit_window(agent)

    def _agent_edit_window(self, agent: Agent = None):
        win = tk.Toplevel(self)
        win.title("New Agent" if agent is None else f"Edit: {agent.name}")
        win.geometry("520x480")
        win.configure(bg=T()["bg"])
        win.grab_set()

        def lbl(parent, text):
            tk.Label(parent, text=text, font=("Segoe UI", 9, "bold"),
                bg=T()["bg"], fg=T()["text_muted"]).pack(anchor="w", pady=(8, 2))

        pad = tk.Frame(win, bg=T()["bg"])
        pad.pack(fill="both", expand=True, padx=20, pady=10)

        lbl(pad, "NAME")
        name_var = tk.StringVar(value=agent.name if agent else "My Agent")
        tk.Entry(pad, textvariable=name_var, font=("Segoe UI", 11),
            relief="flat", bg=T()["input_bg"], fg=T()["text"],
            insertbackground=T()["text"]).pack(fill="x")

        lbl(pad, "MODEL")
        model_var = tk.StringVar(value=agent.model if agent else "claude-opus-4-8")
        ttk.Combobox(pad, textvariable=model_var,
            values=["claude-opus-4-8", "claude-sonnet-4-5", "claude-haiku-4-5"],
            font=("Segoe UI", 10), state="readonly").pack(fill="x")

        lbl(pad, "ICON")
        icon_var = tk.StringVar(value=agent.icon if agent else "🤖")
        icon_row = tk.Frame(pad, bg=T()["bg"])
        icon_row.pack(fill="x")
        tk.Entry(icon_row, textvariable=icon_var, font=("Segoe UI", 14),
            width=4, relief="flat", bg=T()["input_bg"],
            fg=T()["text"]).pack(side="left")
        for ic in ["🤖","🔍","💻","🧪","📝","🗺","⚡","🛡"]:
            tk.Button(icon_row, text=ic, font=("Segoe UI", 11),
                relief="flat", bd=0, padx=2, cursor="hand2",
                bg=T()["surface2"],
                command=lambda i=ic: icon_var.set(i)).pack(side="left", padx=1)

        lbl(pad, "SYSTEM PROMPT")
        sys_text = scrolledtext.ScrolledText(pad, height=6,
            font=("Segoe UI", 9), relief="flat",
            bg=T()["input_bg"], fg=T()["text"],
            insertbackground=T()["text"], wrap="word", padx=6, pady=4)
        sys_text.pack(fill="both", expand=True)
        if agent:
            sys_text.insert("1.0", agent.system)

        # Templates
        lbl(pad, "LOAD TEMPLATE")
        trow = tk.Frame(pad, bg=T()["bg"])
        trow.pack(fill="x", pady=(0, 8))
        for tmpl in BUILTIN_TEMPLATES:
            tk.Button(trow, text=tmpl["name"], font=("Segoe UI", 8),
                relief="flat", bd=0, padx=6, pady=3, cursor="hand2",
                bg=T()["surface2"], fg=T()["text"],
                command=lambda t=tmpl: (
                    name_var.set(t["name"]),
                    icon_var.set(t["icon"]),
                    model_var.set(t["model"]),
                    sys_text.delete("1.0", "end"),
                    sys_text.insert("1.0", t["system"])
                )).pack(side="left", padx=2)

        def _save():
            name = name_var.get().strip()
            if not name:
                messagebox.showwarning("Thieu", "Nhap ten agent!"); return
            if agent:
                agent.name   = name
                agent.model  = model_var.get()
                agent.icon   = icon_var.get()
                agent.system = sys_text.get("1.0", "end").strip()
                self.registry.update(agent)
                self._global_log(f"Da cap nhat agent: {name}", "SUCCESS")
            else:
                new = Agent(name=name, model=model_var.get(),
                            icon=icon_var.get(),
                            system=sys_text.get("1.0","end").strip())
                self.registry.create(new)
                self._global_log(f"Da tao agent moi: {name}", "SUCCESS")
            self._refresh_agents()
            self._refresh_agent_combobox()
            win.destroy()

        btn_row = tk.Frame(pad, bg=T()["bg"])
        btn_row.pack(fill="x", pady=4)
        tk.Button(btn_row, text="💾 Save", font=("Segoe UI", 10, "bold"),
            relief="flat", bd=0, padx=16, pady=8, cursor="hand2",
            bg=T()["tab_sel"], fg="#fff", command=_save).pack(side="left")
        tk.Button(btn_row, text="Cancel", font=("Segoe UI", 10),
            relief="flat", bd=0, padx=12, pady=8, cursor="hand2",
            bg=T()["surface2"], fg=T()["text"],
            command=win.destroy).pack(side="left", padx=8)

    def _delete_agent(self, agent: Agent):
        if messagebox.askyesno("Xac nhan", f"Xoa agent '{agent.name}'?"):
            self.registry.delete(agent.agent_id)
            self._refresh_agents()
            self._refresh_agent_combobox()
            self._global_log(f"Da xoa agent: {agent.name}", "WARN")

    def _reset_agent_session(self, agent: Agent):
        self.registry.reset_session(agent.agent_id)
        self.registry.update_status(agent.agent_id, "idle")
        self._refresh_agents()
        self._global_log(f"Da reset session agent: {agent.name}", "INFO")

    def _run_agent_dialog(self, agent: Agent):
        prompt = simpledialog.askstring(
            "Run Agent", f"Prompt cho '{agent.name}':",
            parent=self)
        if not prompt: return
        self._global_log(f"Manual run agent '{agent.name}'...", "SEND")
        self.orchestr.run_now(agent, prompt,
            on_done=lambda r: self._refresh_agents())

    def _refresh_agent_combobox(self):
        agents = self.registry.list_all()
        vals = [f"{a.icon} {a.name} ({a.model_short})" for a in agents]
        if hasattr(self, "run_agent_cb"):
            self.run_agent_cb["values"] = vals
            if vals:
                self.run_agent_cb.current(0)
        self._agents_list = agents

    # ── Task actions ───────────────────────────────────────────────────────

    def _new_task_dialog(self):
        win = tk.Toplevel(self)
        win.title("New Task")
        win.geometry("480x380")
        win.configure(bg=T()["bg"])
        win.grab_set()

        pad = tk.Frame(win, bg=T()["bg"])
        pad.pack(fill="both", expand=True, padx=20, pady=10)

        def lbl(text):
            tk.Label(pad, text=text, font=("Segoe UI", 9, "bold"),
                bg=T()["bg"], fg=T()["text_muted"]).pack(anchor="w", pady=(6,2))

        lbl("TITLE")
        title_var = tk.StringVar()
        tk.Entry(pad, textvariable=title_var, font=("Segoe UI", 11),
            relief="flat", bg=T()["input_bg"], fg=T()["text"],
            insertbackground=T()["text"]).pack(fill="x")

        lbl("PROMPT / DESCRIPTION")
        prompt_txt = scrolledtext.ScrolledText(pad, height=6,
            font=("Segoe UI", 9), relief="flat",
            bg=T()["input_bg"], fg=T()["text"],
            insertbackground=T()["text"], wrap="word")
        prompt_txt.pack(fill="both", expand=True)

        row = tk.Frame(pad, bg=T()["bg"])
        row.pack(fill="x", pady=8)

        lbl2 = tk.Label(row, text="Priority:", font=("Segoe UI", 9),
            bg=T()["bg"], fg=T()["text"])
        lbl2.pack(side="left")
        pri_var = tk.StringVar(value="normal")
        ttk.Combobox(row, textvariable=pri_var, width=10,
            values=["low","normal","high","urgent"],
            state="readonly").pack(side="left", padx=6)

        def _save():
            t = title_var.get().strip()
            p = prompt_txt.get("1.0", "end").strip()
            if not t:
                messagebox.showwarning("Thieu", "Nhap tieu de!"); return
            task = Task(title=t, description=p, prompt=p,
                        priority=pri_var.get())
            self.board.add(task)
            self._refresh_tasks()
            self._global_log(f"Da them task: '{t}'", "SUCCESS")
            win.destroy()

        tk.Button(pad, text="💾 Add Task", font=("Segoe UI", 10, "bold"),
            relief="flat", bd=0, padx=16, pady=8, cursor="hand2",
            bg=T()["tab_sel"], fg="#fff", command=_save).pack(side="left")

    def _task_detail(self, status: str):
        lb = self.kanban_lists[status]
        sel = lb.curselection()
        if not sel: return
        tasks = self.board.list_by_status(status)
        if sel[0] >= len(tasks): return
        task = tasks[sel[0]]
        info = (f"ID: {task.task_id}\nTitle: {task.title}\n"
                f"Status: {task.status}  Priority: {task.priority}\n"
                f"Assigned: {task.assigned_to or 'None'}\n"
                f"Retries: {task.retry_count}/{task.max_retries}\n"
                f"Duration: {task.duration or 'N/A'}\n\n"
                f"Result:\n{task.result or '(chua co)'}")
        messagebox.showinfo(f"Task: {task.title}", info)

    def _clear_done_tasks(self):
        done = self.board.list_by_status("done")
        for t in done:
            self.board.delete(t.task_id)
        self._refresh_tasks()
        self._global_log(f"Da xoa {len(done)} done tasks.", "INFO")

    # ── Plan Builder actions ───────────────────────────────────────────────

    def _run_decompose(self):
        project = self.plan_input.get("1.0", "end").strip()
        if not project:
            messagebox.showwarning("Thieu", "Nhap mo ta project!"); return
        self.btn_decompose.configure(state="disabled", text="⏳ Dang xu ly...")
        self.plan_status.configure(text="Dang goi Claude AI...",
            fg=T()["warning"])
        self._pending_plan = []

        def _worker():
            tasks = self.plan_bld.build_from_description(
                project, on_log=self._global_log)
            self.after(0, lambda: self._show_plan(tasks))

        threading.Thread(target=_worker, daemon=True).start()

    def _simple_split(self):
        project = self.plan_input.get("1.0", "end").strip()
        if not project:
            messagebox.showwarning("Thieu", "Nhap mo ta project!"); return
        tasks = self.plan_bld._simple_decompose(project)
        self._show_plan(tasks)

    def _show_plan(self, plan_tasks):
        self.btn_decompose.configure(state="normal", text="🗺 AI Decompose")
        self._pending_plan = plan_tasks
        self.plan_list.configure(state="normal")
        self.plan_list.delete("1.0", "end")
        for i, pt in enumerate(plan_tasks):
            dep_str = f" [dep: {','.join(pt.depends_on)}]" if pt.depends_on else ""
            self.plan_list.insert("end",
                f"[{i+1}] {pt.title}{dep_str}\n"
                f"     Model: {pt.model_hint}  Priority: {pt.priority}\n"
                f"     Prompt: {pt.prompt[:80]}...\n\n")
        self.plan_list.configure(state="disabled")
        self.btn_execute_plan.configure(state="normal")
        self.plan_status.configure(
            text=f"Da tao {len(plan_tasks)} tasks. Nhan 'Execute Plan' de chay.",
            fg=T()["success"])

    def _clear_plan(self):
        self.plan_list.configure(state="normal")
        self.plan_list.delete("1.0", "end")
        self.plan_list.configure(state="disabled")
        self._pending_plan = []
        self.btn_execute_plan.configure(state="disabled")
        self.plan_status.configure(text="")

    def _execute_plan(self):
        if not self._pending_plan:
            return
        task_objects = self.plan_bld.plan_to_tasks(self._pending_plan)
        self.orchestr.execute_plan(task_objects)
        self._refresh_tasks()
        self.notebook.select(self.tab_tasks)
        self._global_log(f"Execute plan: {len(task_objects)} tasks da them vao board.", "SUCCESS")
        self.btn_execute_plan.configure(state="disabled")

    # ── Quick Run ──────────────────────────────────────────────────────────

    def _quick_run(self):
        prompt = self.run_prompt.get("1.0", "end").strip()
        if not prompt:
            messagebox.showwarning("Thieu", "Nhap prompt!"); return

        agents = getattr(self, "_agents_list", [])
        sel_idx = self.run_agent_cb.current()
        agent = agents[sel_idx] if 0 <= sel_idx < len(agents) else None

        model = self.run_model_var.get()
        self.btn_run.configure(state="disabled", text="⏳ Running...")
        self.run_result.configure(state="normal")
        self.run_result.delete("1.0", "end")
        self.run_result.configure(state="disabled")
        self._global_log(f"Quick run: model={model}", "SEND")

        def _worker():
            if agent:
                # Save original model, use selected model temporarily
                orig = agent.model
                agent.model = model
                result = self.cli.run_agent(agent, prompt,
                    on_log=self._global_log)
                agent.model = orig
                self.registry.update(agent)
            else:
                result = self.cli.run_once(prompt, model=model,
                    on_log=self._global_log)

            def _done():
                self.btn_run.configure(state="normal", text="▶  Run Now")
                self.run_result.configure(state="normal")
                if result.success:
                    self.run_result.insert("1.0", result.output)
                    self._global_log(f"Done ({result.duration_s:.1f}s)", "SUCCESS")
                else:
                    self.run_result.insert("1.0", f"ERROR: {result.error}")
                    self._global_log(f"Failed: {result.error[:80]}", "ERROR")
                self.run_result.configure(state="disabled")
                self._refresh_agents()

            self.after(0, _done)

        threading.Thread(target=_worker, daemon=True).start()

    def _find_claude_windows(self):
        try:
            import win32gui
            found = []
            def cb(hwnd, _):
                if win32gui.IsWindowVisible(hwnd):
                    t = win32gui.GetWindowText(hwnd).lower()
                    if "claude" in t and "scheduler" not in t:
                        found.append(f"[{hwnd}] {win32gui.GetWindowText(hwnd)}")
            win32gui.EnumWindows(cb, None)
            if found:
                for f in found: self._global_log(f"Window: {f}", "SUCCESS")
            else:
                self._global_log("Khong tim thay cua so Claude.", "WARN")
        except Exception as e:
            self._global_log(f"Loi: {e}", "ERROR")

    # ── Orchestrator control ───────────────────────────────────────────────

    def _toggle_orchestrator(self):
        if self.orchestr._running:
            self.orchestr.stop()
            self.btn_orch.configure(text="▶ Start Orchestrator")
            self.lbl_orch.configure(text="● Stopped", fg=T()["text_muted"])
        else:
            self.orchestr.start()
            self.btn_orch.configure(text="⏹ Stop Orchestrator")
            self.lbl_orch.configure(text="● Running", fg=T()["success"])

    # ── Auto-refresh ───────────────────────────────────────────────────────

    def _start_refresh(self):
        def _tick():
            try:
                self._refresh_tasks()
                self._refresh_agents()
            except Exception:
                pass
            self._refresh_job = self.after(3000, _tick)
        self._refresh_job = self.after(3000, _tick)

    # ── Theme ──────────────────────────────────────────────────────────────

    def _configure_style(self, style):
        t = T()
        style.configure("TNotebook",
            background=t["bg"], borderwidth=0, tabmargins=[0, 0, 0, 0])
        style.configure("TNotebook.Tab",
            background=t["surface2"], foreground=t["text_muted"],
            padding=[14, 6], font=("Segoe UI", 9), borderwidth=0)
        style.map("TNotebook.Tab",
            background=[("selected", t["bg"]), ("active", t["surface"])],
            foreground=[("selected", t["text"]), ("active", t["text"])])

    def _toggle_theme(self):
        global current_theme
        current_theme = "light" if current_theme == "dark" else "dark"
        self.btn_theme.configure(
            text="🌙 Dark" if current_theme == "light" else "☀ Light")
        self._apply_theme()

    def _apply_theme(self):
        t = T()
        self.configure(bg=t["bg"])
        self.header.configure(bg=t["surface"])
        self._divider.configure(bg=t["border"])
        self.lbl_title.configure(bg=t["surface"], fg=t["text"])
        self.btn_theme.configure(bg=t["surface2"], fg=t["text_muted"],
            activebackground=t["border"])
        self.btn_orch.configure(bg=t["tab_sel"], fg="#fff",
            activebackground=t["accent"])
        self.lbl_orch.configure(bg=t["surface"])

        style = ttk.Style(self)
        self._configure_style(style)

        for tab in [self.tab_agents, self.tab_tasks, self.tab_plan,
                    self.tab_run, self.tab_log]:
            tab.configure(bg=t["bg"])

        # Paned windows
        for pw in [self.paned if hasattr(self, "paned") else None]:
            if pw: pw.configure(bg=t["border"])

        # Log tags
        self._setup_log_tags()
        self.log_text.configure(bg=t["surface"], fg=t["text"])

        # Kanban
        for col_lbl in self.kanban_cols.values():
            col_lbl.configure(bg=t["bg"], fg=t["text_muted"])
        for lb in self.kanban_lists.values():
            lb.configure(bg=t["surface"], fg=t["text"],
                selectbackground=t["tab_sel"], selectforeground="#fff")

        self.agents_canvas.configure(bg=t["bg"])
        self.agents_inner.configure(bg=t["bg"])
        self._refresh_agents()

        self.btn_run.configure(bg=t["tab_sel"], fg="#fff")
        self.plan_list.configure(bg=t["surface"], fg=t["text"])
        self.plan_input.configure(bg=t["input_bg"], fg=t["text"],
            insertbackground=t["text"])
        self.run_prompt.configure(bg=t["input_bg"], fg=t["text"],
            insertbackground=t["text"])
        self.run_result.configure(bg=t["surface"], fg=t["text"])

        self.lbl_log_title.configure(bg=t["bg"], fg=t["text_muted"])
        self.lbl_log_cnt.configure(bg=t["bg"], fg=t["text_muted"])
        self.kanban_frame.configure(bg=t["bg"])


if __name__ == "__main__":
    app = AgentManagerApp()
    app.mainloop()
