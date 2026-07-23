#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Claude Suite v2.0 — Ultra-Modern Desktop App (CustomTkinter UI/UX)
Hợp nhất Agent Manager, Task Kanban, AI Plan Builder, Win32 Scheduler & CLI Runner
"""

import sys, io, os
if sys.stdout is not None and hasattr(sys.stdout, "buffer") and not sys.stdout.closed:
    try:
        sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8', errors='replace')
    except Exception:
        pass

if sys.stderr is not None and hasattr(sys.stderr, "buffer") and not sys.stderr.closed:
    try:
        sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding='utf-8', errors='replace')
    except Exception:
        pass

sys.path.insert(0, os.path.dirname(__file__))

import customtkinter as ctk
import tkinter as tk
from tkinter import messagebox, scrolledtext, simpledialog, filedialog
import threading
import datetime
import time
import math

# Set CustomTkinter default options
ctk.set_appearance_mode("dark")
ctk.set_default_color_theme("blue")

current_theme = "dark"

from modules.agent_registry import AgentRegistry, Agent, BUILTIN_TEMPLATES
from modules.task_board     import TaskBoard, Task, STATUSES, STATUS_ICONS, PRIORITY_COLORS
from modules.claude_cli     import ClaudeCLI, MODELS
from modules.orchestrator   import Orchestrator
from modules.plan_builder   import PlanBuilder
from modules.scheduler_tab  import SchedulerTabFrame
from modules.memory_store   import MemoryStore, MemoryItem
from modules.context_manager import ContextManager
from modules.pipeline_engine import AgentPipeline, PipelineStep
from modules.exporter        import ReportExporter
from modules.webhook_server import WebhookServer
from modules.tray_widget    import FloatingQuickWidget
from modules.file_tree_explorer import FileTreeExplorerModal
from modules.workspace_config import WorkspaceConfigStore

# Import New 3-Tab Structure
from ui.tabs.cockpit_tab import CockpitTabFrame
from ui.tabs.task_board_tab import TaskBoardTabFrame
from ui.tabs.studio_settings_tab import StudioSettingsTabFrame
from ui.tabs.virtual_office_tab import VirtualOfficeTabFrame

# Log Tag Colors (Dark / Light)
LOG_TAGS = {
    "THINKING": {"dark": "#c084fc", "light": "#7e22ce"},
    "SUCCESS":  {"dark": "#4ade80", "light": "#16a34a"},
    "ERROR":    {"dark": "#f87171", "light": "#dc2626"},
    "WARN":     {"dark": "#facc15", "light": "#ca8a04"},
    "TIER1":    {"dark": "#c084fc", "light": "#9333ea"},
    "TIER2":    {"dark": "#fb923c", "light": "#ea580c"},
    "SEND":     {"dark": "#38bdf8", "light": "#0284c7"},
    "INFO":     {"dark": "#94a3b8", "light": "#64748b"},
    "SEP":      {"dark": "#334155", "light": "#cbd5e1"},
    "TIME":     {"dark": "#64748b", "light": "#94a3b8"},
}


class ClaudeSuiteApp(ctk.CTk):
    def __init__(self):
        super().__init__()

        self.title("Claude Suite — Agent Control Center")
        self.geometry("1060x780")
        self.minsize(880, 640)

        # Core services
        self.cli          = ClaudeCLI(timeout=300)
        self.registry     = AgentRegistry()
        self.board        = TaskBoard()
        self.memory       = MemoryStore()
        self.ctx_mgr      = ContextManager()
        self.plan_bld     = PlanBuilder(cli=self.cli, on_engine_fallback=self._prompt_engine_fallback)
        self.exporter     = ReportExporter()
        self.webhook_srv  = WebhookServer(port=9090, on_payload=self._on_webhook_received)
        self._quick_context_paths = []
        self.ws_config = WorkspaceConfigStore()
        self._global_workspace_folder = self.ws_config.last_workspace_folder or (r"e:\exe" if os.path.exists(r"e:\exe") else "")
        if self._global_workspace_folder:
            self.ws_config.add_workspace(self._global_workspace_folder)
        self._global_workspace_files  = []
        self._agent_page = 1
        self._agent_page_size = 5
        self._kanban_col_limits = {s: 6 for s in STATUSES}

        self.pipeline_eng = AgentPipeline(
            self.registry, self.cli, self.memory, 
            on_log=self._global_log, 
            get_context_fn=self._get_active_workspace_context, 
            on_agent_communicate=self._trigger_office_communication,
            on_engine_fallback=self._prompt_engine_fallback
        )
        self.orchestr   = Orchestrator(
            self.registry, self.board, self.cli,
            on_log=self._global_log,
            get_context_fn=self._get_active_workspace_context,
            get_workspace_fn=lambda: self._global_workspace_folder,
            on_task_event=self._trigger_office_task,
            on_engine_fallback=self._prompt_engine_fallback
        )
        self.log_count  = 0
        self.current_theme = "dark"

        self._build_ui()
        self._start_refresh()
        self.after(200, self._bring_to_front)

    def _get_active_workspace_context(self) -> str:
        """Trả về khối Markdown Context từ Folder Dự Án + Files Đính Kèm."""
        attached_paths = []
        if self._global_workspace_folder and os.path.exists(self._global_workspace_folder):
            attached_paths.append(self._global_workspace_folder)
        attached_paths.extend(self._global_workspace_files)
        return self.ctx_mgr.build_context_prompt(attached_paths) if attached_paths else ""

    def _bring_to_front(self):
        try:
            self.lift()
            self.attributes("-topmost", True)
            self.after(500, lambda: self.attributes("-topmost", False))
            self.focus_force()
        except Exception:
            pass

    def _run_safe_thread(self, target, on_error_ui_reset=None):
        def _wrapper():
            try:
                target()
            except Exception as e:
                import traceback
                err_msg = traceback.format_exc()
                self._global_log(f"💥 Bị lỗi ngầm: {str(e)}\n{err_msg}", "ERROR")
                if on_error_ui_reset:
                    self.after(0, on_error_ui_reset)
        threading.Thread(target=_wrapper, daemon=True).start()

    def show_toast(self, message: str, mtype: str = "info"):
        toast = ctk.CTkFrame(self, corner_radius=8, fg_color="#10b981" if mtype == "success" else "#f59e0b" if mtype == "warning" else "#3b82f6")
        lbl = ctk.CTkLabel(toast, text=message, font=ctk.CTkFont(size=12, weight="bold"), text_color="white")
        lbl.pack(padx=20, pady=10)
        
        # Animate slide up
        toast.place(relx=0.5, rely=1.0, anchor="s", y=50)
        
        def slide_up(y):
            if y > -20:
                toast.place(relx=0.5, rely=1.0, anchor="s", y=y)
                self.after(20, slide_up, y - 5)
            else:
                self.after(3000, slide_down, -20)
                
        def slide_down(y):
            if y < 50:
                toast.place(relx=0.5, rely=1.0, anchor="s", y=y)
                self.after(20, slide_down, y + 5)
            else:
                toast.destroy()
                
        slide_up(50)

    # ── Build UI ───────────────────────────────────────────────────────────

    def _build_ui(self):
        # ── Top Bar / Header ──
        self.header = ctk.CTkFrame(self, height=60, corner_radius=0, fg_color=("gray95", "#0f172a"))
        self.header.pack(fill="x")
        self.header.pack_propagate(False)

        # Logo & App Title
        logo_frame = ctk.CTkFrame(self.header, fg_color="transparent")
        logo_frame.pack(side="left", padx=20, pady=10)

        lbl_logo = ctk.CTkLabel(
            logo_frame, text="✨", font=ctk.CTkFont(size=22)
        )
        lbl_logo.pack(side="left", padx=(0, 8))

        lbl_title = ctk.CTkLabel(
            logo_frame, text="Claude Suite",
            font=ctk.CTkFont(family="Segoe UI", size=18, weight="bold"),
            text_color=("#0f172a", "#f8fafc")
        )
        lbl_title.pack(side="left", padx=(0, 12))

        # Top Bar Minimalist IDE Menu Items (File | View | Window)
        self.btn_menu_file = ctk.CTkButton(
            logo_frame, text="File", font=ctk.CTkFont(family="Segoe UI", size=13),
            width=48, height=28, fg_color="transparent", text_color=("gray20", "#e2e8f0"),
            hover_color=("gray85", "#1e293b"), corner_radius=4, command=self._show_file_menu
        )
        self.btn_menu_file.pack(side="left", padx=2)

        self.btn_menu_view = ctk.CTkButton(
            logo_frame, text="View", font=ctk.CTkFont(family="Segoe UI", size=13),
            width=48, height=28, fg_color="transparent", text_color=("gray20", "#e2e8f0"),
            hover_color=("gray85", "#1e293b"), corner_radius=4, command=self._show_view_menu
        )
        self.btn_menu_view.pack(side="left", padx=2)

        self.btn_menu_window = ctk.CTkButton(
            logo_frame, text="Window", font=ctk.CTkFont(family="Segoe UI", size=13),
            width=64, height=28, fg_color="transparent", text_color=("gray20", "#e2e8f0"),
            hover_color=("gray85", "#1e293b"), corner_radius=4, command=self._show_window_menu
        )
        self.btn_menu_window.pack(side="left", padx=2)

        # Workspace Folder Badge Pill in Header
        folder_label = os.path.basename(self._global_workspace_folder) or self._global_workspace_folder if self._global_workspace_folder else "Chưa chọn"
        self.btn_ws_folder_pill = ctk.CTkButton(
            logo_frame, text=f"📁 {folder_label}",
            font=ctk.CTkFont(family="Segoe UI", size=11, weight="bold"),
            height=26, fg_color=("gray85", "#1e293b"), text_color=("#22c55e" if self._global_workspace_folder else "gray50"),
            hover_color=("gray75", "#334155"), corner_radius=13, command=self._open_file_tree_explorer
        )
        self.btn_ws_folder_pill.pack(side="left", padx=(12, 4))

        # Workspace Files Context Counter Badge
        self.lbl_ws_summary = ctk.CTkLabel(
            logo_frame, text="🟢 0 Files Context",
            font=ctk.CTkFont(size=11), text_color=("#38bdf8" if current_theme == "dark" else "#0284c7")
        )
        self.lbl_ws_summary.pack(side="left", padx=(4, 0))

        # Orchestrator Status Pill
        self.status_pill = ctk.CTkFrame(self.header, corner_radius=20, fg_color=("gray85", "#1e293b"))
        self.status_pill.pack(side="left", padx=16, pady=14)

        self.lbl_orch_dot = ctk.CTkLabel(
            self.status_pill, text="●", font=ctk.CTkFont(size=14), text_color="#64748b"
        )
        self.lbl_orch_dot.pack(side="left", padx=(10, 4))

        self.lbl_orch_txt = ctk.CTkLabel(
            self.status_pill, text="Orchestrator: Inactive",
            font=ctk.CTkFont(size=11, weight="bold"), text_color=("gray30", "gray70")
        )
        self.lbl_orch_txt.pack(side="left", padx=(0, 10))

        # AI Thinking Status Pill
        self.thinking_pill = ctk.CTkFrame(self.header, corner_radius=20, fg_color=("gray85", "#1e293b"))
        self.thinking_pill.pack(side="left", padx=(0, 16), pady=14)

        self.lbl_think_icon = ctk.CTkLabel(
            self.thinking_pill, text="🧠", font=ctk.CTkFont(size=14)
        )
        self.lbl_think_icon.pack(side="left", padx=(10, 4))

        self.lbl_think_txt = ctk.CTkLabel(
            self.thinking_pill, text="AI Ready",
            font=ctk.CTkFont(size=11, weight="bold"), text_color=("gray30", "gray70")
        )
        self.lbl_think_txt.pack(side="left", padx=(0, 10))

        # Top Right Actions
        right_header = ctk.CTkFrame(self.header, fg_color="transparent")
        right_header.pack(side="right", padx=20, pady=10)

        self.btn_theme = ctk.CTkButton(
            right_header, text="☀  Light", width=80, height=36,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("gray85", "#1e293b"), text_color=("gray10", "gray90"),
            hover_color=("gray75", "#334155"), corner_radius=8,
            command=self._toggle_theme
        )
        self.btn_theme.pack(side="right", padx=(6, 0))

        self.btn_prompt_arch = ctk.CTkButton(
            right_header, text="🪄  Prompt Architect", height=36, width=140,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("#7c3aed", "#8b5cf6"), hover_color=("#6d28d9", "#7c3aed"),
            corner_radius=8, command=self._open_prompt_architect_modal
        )
        self.btn_prompt_arch.pack(side="right", padx=(6, 0))

        self.btn_floating = ctk.CTkButton(
            right_header, text="⚡  Floating Prompt", height=36, width=130,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("gray85", "#1e293b"), text_color=("gray10", "gray90"),
            hover_color=("gray75", "#334155"), corner_radius=8,
            command=self._open_floating_prompt
        )
        self.btn_floating.pack(side="right", padx=(6, 0))

        self.btn_webhook = ctk.CTkButton(
            right_header, text="🌐  Webhook: Off", height=36, width=120,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("gray85", "#1e293b"), text_color=("gray10", "gray90"),
            hover_color=("gray75", "#334155"), corner_radius=8,
            command=self._toggle_webhook
        )
        self.btn_webhook.pack(side="right", padx=(6, 0))

        self.btn_orch = ctk.CTkButton(
            right_header, text="▶  Start Orchestrator", height=36,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("#0284c7", "#0284c7"), hover_color=("#0369a1", "#0369a1"),
            corner_radius=8, command=self._toggle_orchestrator
        )
        self.btn_orch.pack(side="right")

        self._update_global_workspace_ui()

        # ── Main Tab Navigation (CTkTabview) ──
        self.tabview = ctk.CTkTabview(
            self, corner_radius=12,
            segmented_button_fg_color=("gray85", "#1e293b"),
            segmented_button_selected_color=("#0284c7", "#0284c7"),
            segmented_button_selected_hover_color=("#0369a1", "#0369a1"),
            segmented_button_unselected_hover_color=("gray75", "#334155")
        )
        self.tabview.pack(fill="both", expand=True, padx=16, pady=(4, 16))

        # Create New 3-Workspace Tabs
        self.tab_cockpit = self.tabview.add("🚀 AI Cockpit")
        self.tab_board = self.tabview.add("📋 Task Board & Planning")
        self.tab_studio = self.tabview.add("⚙️ Studio & Settings")
        self.tab_office = self.tabview.add("🏢 Virtual Office")

        # Khởi tạo các module Tab Frame mới và đưa vào tab view
        self.cockpit_frame = CockpitTabFrame(self.tab_cockpit, app=self)
        self.cockpit_frame.pack(fill="both", expand=True)

        self.board_frame = TaskBoardTabFrame(self.tab_board, self.board, self.plan_bld)
        self.board_frame.pack(fill="both", expand=True)

        self.studio_frame = StudioSettingsTabFrame(self.tab_studio, self.registry, self.cli)
        self.studio_frame.pack(fill="both", expand=True)

        self.office_frame = VirtualOfficeTabFrame(self.tab_office, app=self)
        self.office_frame.pack(fill="both", expand=True)

        # Render các UI component cũ vào các tab con mới tương ứng
        self._build_tasks_tab(parent=self.board_frame.tab_kanban)
        self._build_plan_tab(parent=self.board_frame.tab_plan)

        self._build_agents_tab(parent=self.studio_frame.tab_agents)
        self._build_pipeline_tab(parent=self.studio_frame.tab_pipeline)
        self._build_scheduler_tab(parent=self.studio_frame.tab_scheduler)
        self._build_run_tab(parent=self.studio_frame.tab_cli)
        self._build_log_tab(parent=self.studio_frame.tab_log)

    # ══════════════════════════════════════════════════════════════════════
    # TAB 1 — AGENTS
    # ══════════════════════════════════════════════════════════════════════

    def _build_agents_tab(self, parent):
        t = parent

        tb = ctk.CTkFrame(t, fg_color="transparent")
        tb.pack(fill="x", padx=4, pady=(4, 8))

        ctk.CTkLabel(
            tb, text="CLAUDE AGENTS REGISTRY",
            font=ctk.CTkFont(size=13, weight="bold"),
            text_color=("gray30", "gray70")
        ).pack(side="left", padx=4)

        # Pagination Bar for Agents
        self.agent_page_frame = ctk.CTkFrame(tb, fg_color="transparent")
        self.agent_page_frame.pack(side="left", padx=(16, 0))

        self.btn_agent_prev = ctk.CTkButton(
            self.agent_page_frame, text="◀", width=28, height=28,
            font=ctk.CTkFont(size=11, weight="bold"),
            fg_color=("gray80", "#334155"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "#475569"), corner_radius=6,
            command=self._prev_agent_page
        )
        self.btn_agent_prev.pack(side="left", padx=2)

        self.lbl_agent_page_info = ctk.CTkLabel(
            self.agent_page_frame, text="Trang 1 / 1 (0 Agents)",
            font=ctk.CTkFont(size=11, weight="bold"), text_color=("gray30", "gray70")
        )
        self.lbl_agent_page_info.pack(side="left", padx=6)

        self.btn_agent_next = ctk.CTkButton(
            self.agent_page_frame, text="▶", width=28, height=28,
            font=ctk.CTkFont(size=11, weight="bold"),
            fg_color=("gray80", "#334155"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "#475569"), corner_radius=6,
            command=self._next_agent_page
        )
        self.btn_agent_next.pack(side="left", padx=2)

        ctk.CTkButton(
            tb, text="+ New Agent", width=110, height=32,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("#0284c7", "#0284c7"), hover_color=("#0369a1", "#0369a1"),
            corner_radius=8, command=self._new_agent
        ).pack(side="right", padx=(6, 0))

        ctk.CTkButton(
            tb, text="🔄 Refresh", width=90, height=32,
            font=ctk.CTkFont(size=12),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=8,
            command=lambda: self._refresh_agents(force=True)
        ).pack(side="right")

        ctk.CTkButton(
            tb, text="🧹 Reset 7 Roles", width=120, height=32,
            font=ctk.CTkFont(size=11),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=8,
            command=self._reset_agents_to_defaults
        ).pack(side="right", padx=(0, 4))

        ctk.CTkButton(
            tb, text="🗑 Blanket Empty", width=120, height=32,
            font=ctk.CTkFont(size=11),
            fg_color=("#dc2626", "#dc2626"), hover_color=("#b91c1c", "#b91c1c"),
            text_color="white", corner_radius=8,
            command=self._clear_all_agents
        ).pack(side="right", padx=(0, 4))

        # Scrollable Agent Card Grid
        self.agents_scroll = ctk.CTkScrollableFrame(t, fg_color="transparent")
        self.agents_scroll.pack(fill="both", expand=True, padx=4, pady=4)

        self._refresh_agents()

    def _prev_agent_page(self):
        if self._agent_page > 1:
            self._agent_page -= 1
            self._refresh_agents(force=True)

    def _next_agent_page(self):
        all_agents = self.registry.list_all()
        total_pages = max(1, math.ceil(len(all_agents) / self._agent_page_size))
        if self._agent_page < total_pages:
            self._agent_page += 1
            self._refresh_agents(force=True)

    def _refresh_agents(self, force=False):
        all_agents = self.registry.list_all()
        total_agents = len(all_agents)
        total_pages = max(1, math.ceil(total_agents / self._agent_page_size))
        if self._agent_page > total_pages:
            self._agent_page = total_pages

        start_idx = (self._agent_page - 1) * self._agent_page_size
        end_idx = start_idx + self._agent_page_size
        agents_page = all_agents[start_idx:end_idx]

        if hasattr(self, "lbl_agent_page_info"):
            self.lbl_agent_page_info.configure(
                text=f"Trang {self._agent_page} / {total_pages} (Tổng {total_agents} Agents)"
            )

        current_ids = [a.agent_id for a in agents_page]

        if not hasattr(self, "_agent_card_map"):
            self._agent_card_map = {}

        existing_ids = list(self._agent_card_map.keys())

        if force or set(current_ids) != set(existing_ids):
            # Rebuild all visible page cards
            for w in self.agents_scroll.winfo_children():
                w.destroy()
            self._agent_card_map = {}

            if not agents_page:
                ctk.CTkLabel(
                    self.agents_scroll,
                    text="⚡ Danh sách Agents đang trống (Blanket Default).\n\nAgents sẽ TỰ ĐỘNG TẠO dựa trên yêu cầu khi bạn tạo Tasks hoặc bấm các nút Preset.\nBạn cũng có thể bấm '+ New Agent' hoặc '🧹 Reset 7 Roles' bất kỳ lúc nào.",
                    font=ctk.CTkFont(size=13), text_color=("gray50", "gray50"),
                    justify="center"
                ).pack(pady=60)
                return

            for idx, agent in enumerate(agents_page):
                self._agent_card(self.agents_scroll, agent, idx)
        else:
            # In-place update existing page cards without destroying widgets
            status_colors = {
                "idle": "#f59e0b", "running": "#22c55e",
                "done": "#38bdf8", "error": "#ef4444"
            }
            for agent in agents_page:
                ref = self._agent_card_map.get(agent.agent_id)
                if not ref:
                    continue
                dot_color = status_colors.get(agent.status, "#64748b")
                ref["dot"].configure(text_color=dot_color)

                sub_txt = f"Trạng thái: {agent.status.upper()}  ·  Đã hoàn thành: {agent.tasks_done} tasks"
                ref["sub"].configure(text=sub_txt)

                if agent.last_task and "last" in ref:
                    ref["last"].configure(text=f"Tác vụ gần nhất: {agent.last_task[:60]}")

    def _agent_card(self, parent, agent: Agent, idx: int):
        card = ctk.CTkFrame(parent, corner_radius=12, fg_color=("gray90", "#1e293b"))
        card.pack(fill="x", pady=5)

        inner = ctk.CTkFrame(card, fg_color="transparent")
        inner.pack(fill="x", padx=16, pady=12)

        # Left Info
        left = ctk.CTkFrame(inner, fg_color="transparent")
        left.pack(side="left", fill="y")

        status_colors = {
            "idle": "#f59e0b", "running": "#22c55e",
            "done": "#38bdf8", "error": "#ef4444"
        }
        dot_color = status_colors.get(agent.status, "#64748b")

        lbl_dot = ctk.CTkLabel(left, text="●", font=ctk.CTkFont(size=18), text_color=dot_color)
        lbl_dot.pack(side="left", padx=(0, 10))

        info = ctk.CTkFrame(left, fg_color="transparent")
        info.pack(side="left")

        # Title row with icon
        title_row = ctk.CTkFrame(info, fg_color="transparent")
        title_row.pack(anchor="w")

        ctk.CTkLabel(
            title_row, text=f"{agent.icon} {agent.name}",
            font=ctk.CTkFont(size=14, weight="bold"),
            text_color=("gray10", "gray95")
        ).pack(side="left", padx=(0, 10))

        # Model badge
        model_bg = {"Opus": "#4338ca", "Sonnet": "#0284c7", "Haiku": "#0d9488"}.get(agent.model_short, "#475569")
        model_badge = ctk.CTkFrame(title_row, corner_radius=10, fg_color=model_bg)
        model_badge.pack(side="left")

        ctk.CTkLabel(
            model_badge, text=agent.model_short, font=ctk.CTkFont(size=10, weight="bold"),
            text_color="#ffffff"
        ).pack(padx=8, pady=2)

        # Sub details & Token Quota Display
        sub_txt = f"Trạng thái: {agent.status.upper()}  ·  Đã làm: {agent.tasks_done} tasks  ·  📊 Tokens: {agent.token_display}"
        lbl_sub = ctk.CTkLabel(
            info, text=sub_txt, font=ctk.CTkFont(size=11), text_color=("gray40", "gray60")
        )
        lbl_sub.pack(anchor="w", pady=(2, 0))

        # Token Usage Progress Bar
        tok_progress = ctk.CTkProgressBar(info, height=6, corner_radius=3)
        tok_progress.pack(fill="x", pady=(3, 2))
        pct = agent.token_usage_percent / 100.0
        tok_progress.set(pct)
        # Color coding: Green if remaining > 30%, Amber if <= 30%, Red if <= 10%
        rem_pct = 100.0 - agent.token_usage_percent
        prog_color = "#22c55e" if rem_pct > 30 else "#f59e0b" if rem_pct > 10 else "#ef4444"
        tok_progress.configure(progress_color=prog_color)

        lbl_last = None
        act_text = f"⚡ Live: {agent.notes[:65]}" if (agent.status == "running" and agent.notes) else f"Tác vụ gần nhất: {agent.last_task[:60]}" if agent.last_task else ""
        if act_text:
            lbl_last = ctk.CTkLabel(
                info, text=act_text,
                font=ctk.CTkFont(size=11, weight="bold" if agent.status == "running" else "normal"),
                text_color=("#22c55e" if agent.status == "running" else "gray50")
            )
            lbl_last.pack(anchor="w")

        # Save widget references for smooth updates
        self._agent_card_map[agent.agent_id] = {
            "dot": lbl_dot,
            "sub": lbl_sub,
            "last": lbl_last
        }

        # Right Action Buttons
        right = ctk.CTkFrame(inner, fg_color="transparent")
        right.pack(side="right")

        ctk.CTkButton(
            right, text="▶ Run", width=64, height=30,
            font=ctk.CTkFont(size=11, weight="bold"),
            fg_color=("#16a34a", "#22c55e"), hover_color=("#15803d", "#16a34a"),
            corner_radius=6, command=lambda a=agent: self._run_agent_dialog(a)
        ).pack(side="left", padx=3)

        ctk.CTkButton(
            right, text="🔍 Detail", width=64, height=30,
            font=ctk.CTkFont(size=11),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=6,
            command=lambda a=agent: self._show_agent_detail_modal(a)
        ).pack(side="left", padx=3)

        ctk.CTkButton(
            right, text="📜 Memory", width=74, height=30,
            font=ctk.CTkFont(size=11),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=6,
            command=lambda a=agent: self._view_agent_memory_modal(a)
        ).pack(side="left", padx=3)

        ctk.CTkButton(
            right, text="✏ Edit", width=64, height=30,
            font=ctk.CTkFont(size=11),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=6,
            command=lambda a=agent: self._edit_agent(a)
        ).pack(side="left", padx=3)

        ctk.CTkButton(
            right, text="🔄 Reset", width=64, height=30,
            font=ctk.CTkFont(size=11),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=6,
            command=lambda a=agent: self._reset_agent_session(a)
        ).pack(side="left", padx=3)

        ctk.CTkButton(
            right, text="🗑", width=36, height=30,
            font=ctk.CTkFont(size=12),
            fg_color=("#fee2e2", "#450a0a"), text_color="#ef4444",
            hover_color=("#fca5a5", "#7f1d1d"), corner_radius=6,
            command=lambda a=agent: self._delete_agent(a)
        ).pack(side="left", padx=3)

    # ══════════════════════════════════════════════════════════════════════
    # TAB 2 — TASKS KANBAN BOARD
    # ══════════════════════════════════════════════════════════════════════

    def _build_tasks_tab(self, parent):
        t = parent

        tb = ctk.CTkFrame(t, fg_color="transparent")
        tb.pack(fill="x", padx=4, pady=(4, 8))

        ctk.CTkLabel(
            tb, text="TASK KANBAN BOARD",
            font=ctk.CTkFont(size=13, weight="bold"),
            text_color=("gray30", "gray70")
        ).pack(side="left", padx=4)

        ctk.CTkButton(
            tb, text="📤 Export Report", width=110, height=32,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("#0284c7", "#0284c7"), hover_color=("#0369a1", "#0369a1"),
            corner_radius=8, command=self._export_kanban_report
        ).pack(side="right", padx=(6, 0))

        ctk.CTkButton(
            tb, text="+ Add Task", width=100, height=32,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("#0284c7", "#0284c7"), hover_color=("#0369a1", "#0369a1"),
            corner_radius=8, command=self._new_task_dialog
        ).pack(side="right", padx=(6, 0))

        ctk.CTkButton(
            tb, text="🔥 Clear All Tasks", width=120, height=32,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("#dc2626", "#ef4444"), text_color="#ffffff",
            hover_color=("#b91c1c", "#dc2626"), corner_radius=8,
            command=self._clear_all_tasks
        ).pack(side="right", padx=(6, 0))

        ctk.CTkButton(
            tb, text="🗑 Clear Done", width=100, height=32,
            font=ctk.CTkFont(size=12),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=8,
            command=self._clear_done_tasks
        ).pack(side="right")

        # Kanban columns frame
        self.kanban_frame = ctk.CTkFrame(t, fg_color="transparent")
        self.kanban_frame.pack(fill="both", expand=True, padx=2, pady=4)

        self.kanban_cols  = {}
        self.kanban_scrolls = {}
        self._task_card_map = {}
        self._dragging_task = None

        col_cfg = [
            ("backlog",  "📋 BACKLOG",  0),
            ("queued",   "⏳ QUEUED",   1),
            ("running",  "⚡ RUNNING",  2),
            ("done",     "✅ DONE",     3),
            ("failed",   "❌ FAILED",   4),
        ]

        for status, label, col in col_cfg:
            self.kanban_frame.columnconfigure(col, weight=1, uniform="kanban_col")
            self.kanban_frame.rowconfigure(0, weight=1)

            col_card = ctk.CTkFrame(self.kanban_frame, corner_radius=12, fg_color=("gray90", "#1e293b"))
            col_card.grid(row=0, column=col, sticky="nsew", padx=3, pady=2)

            hdr = ctk.CTkFrame(col_card, fg_color="transparent")
            hdr.pack(fill="x", padx=10, pady=(10, 6))

            lbl_col = ctk.CTkLabel(
                hdr, text=label, font=ctk.CTkFont(size=11, weight="bold"),
                text_color=("gray30", "gray70")
            )
            lbl_col.pack(side="left")
            self.kanban_cols[status] = lbl_col

            # Scrollable column for Jira cards
            scroll_col = ctk.CTkScrollableFrame(col_card, fg_color="transparent")
            scroll_col.pack(fill="both", expand=True, padx=4, pady=(0, 6))
            self.kanban_scrolls[status] = scroll_col

            # Inline Quick Create at bottom of Backlog
            if status == "backlog":
                quick_row = ctk.CTkFrame(col_card, fg_color="transparent")
                quick_row.pack(fill="x", padx=8, pady=(0, 8))

                self.quick_task_entry = ctk.CTkEntry(
                    quick_row, placeholder_text="+ Create Issue (Press Enter)...",
                    font=ctk.CTkFont(size=11), height=30
                )
                self.quick_task_entry.pack(fill="x")
                self.quick_task_entry.bind("<Return>", lambda e: self._quick_add_task())

        self._refresh_tasks()

    def _quick_add_task(self):
        title = self.quick_task_entry.get().strip()
        if not title:
            return
        task = Task(title=title, description=title, prompt=title, priority="normal")
        self.board.add(task)
        self.quick_task_entry.delete(0, "end")
        self._refresh_tasks()
        self._global_log(f"Jira Quick Create: '{title}' added to Backlog", "SUCCESS")

    def _refresh_tasks(self):
        # Skip UI refresh while user is dragging a card
        if getattr(self, "_dragging_task", None) is not None:
            return

        counts = self.board.counts()
        labels = {"backlog":"📋 BACKLOG","queued":"⏳ QUEUED",
                  "running":"⚡ RUNNING","done":"✅ DONE","failed":"❌ FAILED"}

        # Update column header counts in-place
        for status in STATUSES:
            count = counts.get(status, 0)
            if status in self.kanban_cols:
                self.kanban_cols[status].configure(
                    text=f"{labels.get(status, status)} ({count})"
                )

        all_tasks = self.board.list_all()
        
        # --- Empty State UI ---
        if not hasattr(self, "_kanban_empty_lbl"):
            self._kanban_empty_lbl = ctk.CTkLabel(
                self.kanban_frame, 
                text="📭 Chưa có nhiệm vụ nào.\nHãy sang tab 'AI Planning' để lập kế hoạch hoặc bấm '+ Add Task' nhé!", 
                font=ctk.CTkFont(size=14, weight="bold"), 
                text_color=("gray50", "gray40")
            )
        
        if not all_tasks:
            self._kanban_empty_lbl.place(relx=0.5, rely=0.5, anchor="center")
        else:
            self._kanban_empty_lbl.place_forget()
        # ----------------------

        # Group tasks by status & apply lazy loading limits per column
        tasks_by_status = {s: [] for s in STATUSES}
        for t in all_tasks:
            if t.status in tasks_by_status:
                tasks_by_status[t.status].append(t)

        visible_tasks = []
        for status in STATUSES:
            limit = self._kanban_col_limits.get(status, 6)
            col_tasks = tasks_by_status[status]
            visible_tasks.extend(col_tasks[:limit])

            # Manage Load More Button at bottom of column
            scroll = self.kanban_scrolls.get(status)
            if scroll:
                if not hasattr(self, "_load_more_btns"):
                    self._load_more_btns = {}
                
                total_in_col = len(col_tasks)
                if total_in_col > limit:
                    rem = total_in_col - limit
                    btn_text = f"➕ Hiển thị thêm 10 Tasks (Còn {rem})"
                    if status not in self._load_more_btns or not self._load_more_btns[status].winfo_exists():
                        btn = ctk.CTkButton(
                            scroll, text=btn_text, height=28,
                            font=ctk.CTkFont(size=10, weight="bold"),
                            fg_color=("gray85", "#1e293b"), text_color=("#0284c7", "#38bdf8"),
                            hover_color=("gray75", "#334155"), corner_radius=6,
                            command=lambda s=status: self._load_more_kanban_tasks(s)
                        )
                        self._load_more_btns[status] = btn
                    else:
                        btn = self._load_more_btns[status]
                        btn.configure(text=btn_text)
                    btn.pack_forget()
                    btn.pack(in_=scroll, fill="x", pady=6, padx=4)
                else:
                    if status in self._load_more_btns and self._load_more_btns[status].winfo_exists():
                        self._load_more_btns[status].pack_forget()

        current_map = {t.task_id: t for t in visible_tasks}

        if not hasattr(self, "_jira_card_map"):
            self._jira_card_map = {}

        existing_ids = list(self._jira_card_map.keys())

        # 1. Destroy widgets for tasks no longer visible
        for tid in existing_ids:
            if tid not in current_map:
                ref = self._jira_card_map.pop(tid, None)
                if ref and ref.get("card"):
                    try: ref["card"].destroy()
                    except Exception: pass

        # 2. Render or Move visible cards in-place without full destruction
        agents_map = {a.agent_id: a for a in self.registry.list_all()}

        for task in visible_tasks:
            ref = self._jira_card_map.get(task.task_id)
            scroll = self.kanban_scrolls.get(task.status)
            if not scroll:
                continue

            if not ref or not ref.get("card") or not ref["card"].winfo_exists():
                # Brand new card widget
                self._render_jira_card(scroll, task, agents_map)
            elif ref.get("status") != task.status:
                # Task moved column -> re-pack in new scroll frame in-place (NO destruction!)
                card = ref["card"]
                card.pack_forget()
                card.pack(in_=scroll, fill="x", pady=4, padx=2)
                ref["status"] = task.status
                # Update footer agent txt if changed
                assigned_agent = agents_map.get(task.assigned_to)
                agent_txt = f"{assigned_agent.icon} {assigned_agent.name[:12]}" if assigned_agent else "👤 Unassigned"
                if "lbl_agent" in ref and ref["lbl_agent"].winfo_exists():
                    ref["lbl_agent"].configure(text=agent_txt)

    def _load_more_kanban_tasks(self, status: str):
        current_limit = self._kanban_col_limits.get(status, 6)
        self._kanban_col_limits[status] = current_limit + 10
        self._refresh_tasks()

    def _render_jira_card(self, parent, task: Task, agents_map: dict):
        card = ctk.CTkFrame(parent, corner_radius=10, fg_color=("gray95", "#0f172a"), border_width=1, border_color=("gray80", "#334155"))
        card.pack(fill="x", pady=4, padx=2)

        # Priority badge colors
        pri_bg = {
            "urgent": "#ef4444", "high": "#eab308",
            "normal": "#0284c7", "low": "#64748b"
        }.get(task.priority, "#64748b")

        # Top Header Row: Task Key + Priority Badge
        top_row = ctk.CTkFrame(card, fg_color="transparent")
        top_row.pack(fill="x", padx=10, pady=(8, 4))

        key_badge = ctk.CTkFrame(top_row, corner_radius=6, fg_color=("#e2e8f0", "#1e293b"))
        key_badge.pack(side="left")
        ctk.CTkLabel(
            key_badge, text=f"TSK-{task.task_id[:4].upper()}",
            font=ctk.CTkFont(size=9, weight="bold"),
            text_color=("#0369a1", "#38bdf8")
        ).pack(side="left", padx=(6, 2), pady=1)

        # Quick Detail Inspector Button
        ctk.CTkButton(
            key_badge, text="🔍", width=18, height=18,
            font=ctk.CTkFont(size=8),
            fg_color="transparent", hover_color=("gray75", "#334155"),
            command=lambda t=task: self._show_task_detail_modal(t)
        ).pack(side="left", padx=(0, 4))

        pri_badge = ctk.CTkFrame(top_row, corner_radius=6, fg_color=pri_bg)
        pri_badge.pack(side="right")
        ctk.CTkLabel(
            pri_badge, text=task.priority.upper(),
            font=ctk.CTkFont(size=8, weight="bold"),
            text_color="#ffffff"
        ).pack(padx=6, pady=1)

        # Title
        lbl_title = ctk.CTkLabel(
            card, text=task.title,
            font=ctk.CTkFont(size=12, weight="bold"),
            text_color=("gray10", "gray95"),
            wraplength=170, justify="left"
        )
        lbl_title.pack(anchor="w", padx=10, pady=(2, 6))

        # Bottom Footer Row: Assigned Agent & Move Buttons
        footer = ctk.CTkFrame(card, fg_color="transparent")
        footer.pack(fill="x", padx=10, pady=(0, 8))

        assigned_agent = agents_map.get(task.assigned_to)
        agent_txt = f"{assigned_agent.icon} {assigned_agent.name[:12]}" if assigned_agent else "👤 Unassigned"

        ctk.CTkLabel(
            footer, text=agent_txt,
            font=ctk.CTkFont(size=10),
            text_color=("gray40", "gray60")
        ).pack(side="left")

        lbl_agent_widget = footer.winfo_children()[-1]

        # Save widget reference for in-place updates without flickering
        self._jira_card_map[task.task_id] = {
            "card": card,
            "status": task.status,
            "lbl_agent": lbl_agent_widget
        }

        # Quick Status Move Arrows (Jira style move buttons)
        move_frame = ctk.CTkFrame(footer, fg_color="transparent")
        move_frame.pack(side="right")

        curr_idx = STATUSES.index(task.status) if task.status in STATUSES else 0

        if curr_idx > 0:
            prev_status = STATUSES[curr_idx - 1]
            ctk.CTkButton(
                move_frame, text="◀", width=22, height=20,
                font=ctk.CTkFont(size=9),
                fg_color=("gray85", "#1e293b"), text_color=("gray20", "gray80"),
                hover_color=("gray75", "#334155"), corner_radius=4,
                command=lambda s=prev_status, tid=task.task_id: self._move_task_status(tid, s)
            ).pack(side="left", padx=1)

        if curr_idx < len(STATUSES) - 1:
            next_status = STATUSES[curr_idx + 1]
            ctk.CTkButton(
                move_frame, text="▶", width=22, height=20,
                font=ctk.CTkFont(size=9),
                fg_color=("gray85", "#1e293b"), text_color=("gray20", "gray80"),
                hover_color=("gray75", "#334155"), corner_radius=4,
                command=lambda s=next_status, tid=task.task_id: self._move_task_status(tid, s)
            ).pack(side="left", padx=1)

        # Bind Click & Drag Events
        for widget in [card, top_row, key_badge, pri_badge, lbl_title, footer]:
            widget.bind("<Double-Button-1>", lambda e, tid=task.task_id: self._task_detail_by_id(tid))
            widget.bind("<ButtonPress-1>", lambda e, tid=task.task_id, w=card: self._on_drag_start(e, tid, w))
            widget.bind("<B1-Motion>", self._on_drag_motion)
            widget.bind("<ButtonRelease-1>", self._on_drag_release)

    def _move_task_status(self, task_id: str, new_status: str):
        self.board.update_status(task_id, new_status)
        self._last_task_hash = None
        self._refresh_tasks()
        self._global_log(f"Moved task [{task_id}] -> {new_status.upper()}", "SUCCESS")

    def _on_drag_start(self, event, task_id: str, widget):
        self._dragging_task = task_id
        self._drag_start_x = event.x_root
        self._drag_start_y = event.y_root
        widget.configure(border_color="#0284c7", border_width=2)

    def _on_drag_motion(self, event):
        if not self._dragging_task:
            return
        # Calculate cursor relative position over kanban board columns
        x_rel = event.x_root - self.kanban_frame.winfo_rootx()
        total_width = self.kanban_frame.winfo_width() or 1
        col_idx = int((x_rel / total_width) * len(STATUSES))
        col_idx = max(0, min(len(STATUSES) - 1, col_idx))

        target_status = STATUSES[col_idx]
        for s in STATUSES:
            if s == target_status:
                self.kanban_cols[s].configure(text_color="#0284c7")
            else:
                self.kanban_cols[s].configure(text_color=("gray30", "gray70"))

    def _on_drag_release(self, event):
        if not self._dragging_task:
            return
        x_rel = event.x_root - self.kanban_frame.winfo_rootx()
        total_width = self.kanban_frame.winfo_width() or 1
        col_idx = int((x_rel / total_width) * len(STATUSES))
        col_idx = max(0, min(len(STATUSES) - 1, col_idx))

        target_status = STATUSES[col_idx]
        self._move_task_status(self._dragging_task, target_status)

        # Reset column label colors
        for s in STATUSES:
            self.kanban_cols[s].configure(text_color=("gray30", "gray70"))
        self._dragging_task = None

    def _task_detail_by_id(self, task_id: str):
        task = self.board.get(task_id)
        if not task:
            return
        info = (f"ID: {task.task_id}\nTitle: {task.title}\n"
                f"Status: {task.status.upper()}  Priority: {task.priority.upper()}\n"
                f"Assigned: {task.assigned_to or 'Unassigned'}\n"
                f"Retries: {task.retry_count}/{task.max_retries}\n"
                f"Duration: {task.duration or 'N/A'}\n\n"
                f"Prompt:\n{task.prompt or task.description}\n\n"
                f"Result:\n{task.result or '(Chưa có kết quả)'}")
        messagebox.showinfo(f"Jira Issue: TSK-{task.task_id[:4].upper()}", info)

    # ══════════════════════════════════════════════════════════════════════
    # TAB 3 — AI PLAN BUILDER
    # ══════════════════════════════════════════════════════════════════════

    def _build_plan_tab(self, parent):
        t = parent

        grid = ctk.CTkFrame(t, fg_color="transparent")
        grid.pack(fill="both", expand=True, padx=4, pady=4)
        grid.columnconfigure(0, weight=1)
        grid.columnconfigure(1, weight=1)
        grid.rowconfigure(0, weight=1)

        # Left Column: Input Description
        left_card = ctk.CTkFrame(grid, corner_radius=12, fg_color=("gray90", "#1e293b"))
        left_card.grid(row=0, column=0, sticky="nsew", padx=(0, 6), pady=4)

        ctk.CTkLabel(
            left_card, text="📝  MÔ TẢ DỰ ÁN / MỤC TIÊU",
            font=ctk.CTkFont(size=12, weight="bold"),
            text_color=("gray30", "gray70")
        ).pack(anchor="w", padx=16, pady=(14, 6))

        self.plan_input = ctk.CTkTextbox(
            left_card, font=ctk.CTkFont(size=12), corner_radius=8
        )
        self.plan_input.pack(fill="both", expand=True, padx=16, pady=(0, 10))
        self.plan_input.insert("1.0", "Xây dựng hệ thống Quản lý Task với REST API, Dashboard UI và Unit Tests")

        btn_row = ctk.CTkFrame(left_card, fg_color="transparent")
        btn_row.pack(fill="x", padx=16, pady=(0, 14))

        self.btn_decompose = ctk.CTkButton(
            btn_row, text="🗺  AI Decompose (Opus 4.8)", height=38,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("#0284c7", "#0284c7"), hover_color=("#0369a1", "#0369a1"),
            corner_radius=8, command=self._run_decompose
        )
        self.btn_decompose.pack(side="left", expand=True, fill="x", padx=(0, 6))

        ctk.CTkButton(
            btn_row, text="⚡ Simple Split", height=38, width=100,
            font=ctk.CTkFont(size=12),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=8,
            command=self._simple_split
        ).pack(side="left")

        self.plan_status = ctk.CTkLabel(
            left_card, text="", font=ctk.CTkFont(size=11), text_color="gray50"
        )
        self.plan_status.pack(anchor="w", padx=16, pady=(0, 10))

        # Right Column: Generated Task Tree
        right_card = ctk.CTkFrame(grid, corner_radius=12, fg_color=("gray90", "#1e293b"))
        right_card.grid(row=0, column=1, sticky="nsew", padx=(6, 0), pady=4)

        hdr_right = ctk.CTkFrame(right_card, fg_color="transparent")
        hdr_right.pack(fill="x", padx=16, pady=(14, 6))

        ctk.CTkLabel(
            hdr_right, text="🌲  TASKS ĐƯỢC TẠO",
            font=ctk.CTkFont(size=12, weight="bold"),
            text_color=("gray30", "gray70")
        ).pack(side="left")

        self.btn_execute_plan = ctk.CTkButton(
            hdr_right, text="▶  Execute Plan", height=30, width=110,
            font=ctk.CTkFont(size=11, weight="bold"),
            fg_color=("#16a34a", "#22c55e"), hover_color=("#15803d", "#16a34a"),
            corner_radius=6, command=self._execute_plan, state="disabled"
        )
        self.btn_execute_plan.pack(side="right")

        ctk.CTkButton(
            hdr_right, text="🗑 Clear", height=30, width=64,
            font=ctk.CTkFont(size=11),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=6,
            command=self._clear_plan
        ).pack(side="right", padx=(0, 6))

        self.plan_list = ctk.CTkTextbox(
            right_card, font=ctk.CTkFont(family="Consolas", size=11),
            corner_radius=8
        )
        self.plan_list.pack(fill="both", expand=True, padx=16, pady=(0, 14))

        self._pending_plan = []

    # ══════════════════════════════════════════════════════════════════════
    # TAB 4 — WIN32 SCHEDULER
    # ══════════════════════════════════════════════════════════════════════

    def _build_scheduler_tab(self, parent):
        t = parent
        # Render SchedulerTabFrame inside tab
        frame = SchedulerTabFrame(t, log_fn=self._global_log)
        frame.pack(fill="both", expand=True)

    # ══════════════════════════════════════════════════════════════════════
    # TAB 5 — QUICK CLI RUNNER
    # ══════════════════════════════════════════════════════════════════════

    def _build_run_tab(self, parent):
        t = parent

        card_top = ctk.CTkFrame(t, corner_radius=12, fg_color=("gray90", "#1e293b"))
        card_top.pack(fill="x", padx=4, pady=(4, 8))

        row_cfg = ctk.CTkFrame(card_top, fg_color="transparent")
        row_cfg.pack(fill="x", padx=16, pady=(14, 8))

        ctk.CTkLabel(row_cfg, text="Agent:", font=ctk.CTkFont(size=12, weight="bold")).pack(side="left", padx=(0, 6))
        self.run_agent_var = ctk.StringVar()
        self.run_agent_cb  = ctk.CTkOptionMenu(
            row_cfg, variable=self.run_agent_var, width=220, font=ctk.CTkFont(size=12)
        )
        self.run_agent_cb.pack(side="left", padx=(0, 20))
        self._refresh_agent_combobox()

        ctk.CTkLabel(row_cfg, text="Model:", font=ctk.CTkFont(size=12, weight="bold")).pack(side="left", padx=(0, 6))
        self.run_model_var = ctk.StringVar(value="claude-opus-4-8")
        model_cb = ctk.CTkOptionMenu(
            row_cfg, variable=self.run_model_var, width=250, font=ctk.CTkFont(size=12),
            values=[
                "claude-opus-4-8", "claude-sonnet-4-5", "claude-haiku-4-5",
                "gemini-3.6-flash-high", "gemini-3.6-flash-medium", "gemini-3.6-flash-low",
                "gemini-3.5-flash-high", "gemini-3.5-flash-medium", "gemini-3.5-flash-low",
                "gemini-3.1-pro-high", "gemini-3.1-pro-low",
                "claude-sonnet-4.6-thinking", "claude-opus-4.6-thinking"
            ]
        )
        model_cb.pack(side="left")

        # Context Attachment Row
        ctx_row = ctk.CTkFrame(card_top, fg_color="transparent")
        ctx_row.pack(fill="x", padx=16, pady=(0, 6))

        ctk.CTkLabel(
            ctx_row, text="📁 WORKSPACE CONTEXT:",
            font=ctk.CTkFont(size=11, weight="bold"), text_color=("gray30", "gray70")
        ).pack(side="left", padx=(0, 6))

        self.lbl_ctx_badge = ctk.CTkLabel(
            ctx_row, text="Chưa chọn file/folder",
            font=ctk.CTkFont(size=11), text_color="gray50"
        )
        self.lbl_ctx_badge.pack(side="left", padx=(0, 10))

        ctk.CTkButton(
            ctx_row, text="📁 Desktop Folder", height=28, width=120,
            font=ctk.CTkFont(size=11),
            fg_color=("gray85", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray75", "gray35"), corner_radius=6,
            command=self._pick_quick_folder
        ).pack(side="left", padx=2)

        ctk.CTkButton(
            ctx_row, text="📄 Select Files", height=28, width=100,
            font=ctk.CTkFont(size=11),
            fg_color=("gray85", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray75", "gray35"), corner_radius=6,
            command=self._pick_quick_files
        ).pack(side="left", padx=2)

        ctk.CTkButton(
            ctx_row, text="🗑 Clear", height=28, width=60,
            font=ctk.CTkFont(size=11),
            fg_color=("gray85", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray75", "gray35"), corner_radius=6,
            command=self._clear_quick_context
        ).pack(side="left", padx=2)

        ctk.CTkLabel(
            card_top, text="PROMPT NỘI DUNG",
            font=ctk.CTkFont(size=12, weight="bold"),
            text_color=("gray30", "gray70")
        ).pack(anchor="w", padx=16, pady=(6, 4))

        self.run_prompt = ctk.CTkTextbox(
            card_top, height=100, font=ctk.CTkFont(size=12), corner_radius=8
        )
        self.run_prompt.pack(fill="x", padx=16, pady=(0, 10))

        btn_row = ctk.CTkFrame(card_top, fg_color="transparent")
        btn_row.pack(fill="x", padx=16, pady=(0, 14))

        self.btn_run = ctk.CTkButton(
            btn_row, text="▶  Chạy CLI Trực Tiếp", height=38,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("#0284c7", "#0284c7"), hover_color=("#0369a1", "#0369a1"),
            corner_radius=8, command=self._quick_run
        )
        self.btn_run.pack(side="left")

        # Response Output
        card_bottom = ctk.CTkFrame(t, corner_radius=12, fg_color=("gray90", "#1e293b"))
        card_bottom.pack(fill="both", expand=True, padx=4, pady=(4, 4))

        ctk.CTkLabel(
            card_bottom, text="KẾT QUẢ TỪ CLI (OUTPUT)",
            font=ctk.CTkFont(size=12, weight="bold"),
            text_color=("gray30", "gray70")
        ).pack(anchor="w", padx=16, pady=(14, 6))

        self.run_result = ctk.CTkTextbox(
            card_bottom, font=ctk.CTkFont(family="Consolas", size=11), corner_radius=8
        )
        self.run_result.pack(fill="both", expand=True, padx=16, pady=(0, 14))

    # ══════════════════════════════════════════════════════════════════════
    # TAB 6 — LIVE LOG
    # ══════════════════════════════════════════════════════════════════════

    def _build_log_tab(self, parent):
        t = parent

        hdr = ctk.CTkFrame(t, fg_color="transparent")
        hdr.pack(fill="x", padx=4, pady=(4, 8))

        ctk.CTkLabel(
            hdr, text="LIVE SYSTEM LOG",
            font=ctk.CTkFont(size=13, weight="bold"),
            text_color=("gray30", "gray70")
        ).pack(side="left", padx=4)

        self.lbl_log_cnt = ctk.CTkLabel(
            hdr, text="0 dòng", font=ctk.CTkFont(size=11), text_color="gray50"
        )
        self.lbl_log_cnt.pack(side="left", padx=(8, 0))

        ctk.CTkButton(
            hdr, text="💾 Mở File Log", width=105, height=30,
            font=ctk.CTkFont(size=11),
            fg_color=("#0284c7", "#0284c7"), text_color="#ffffff",
            hover_color=("#0369a1", "#0369a1"), corner_radius=6,
            command=self._open_log_file
        ).pack(side="right", padx=(6, 0))

        ctk.CTkButton(
            hdr, text="Copy Log", width=80, height=30,
            font=ctk.CTkFont(size=11),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=6,
            command=self._copy_log
        ).pack(side="right", padx=(6, 0))

        ctk.CTkButton(
            hdr, text="Xóa Log", width=80, height=30,
            font=ctk.CTkFont(size=11),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=6,
            command=self._clear_log
        ).pack(side="right")

        log_card = ctk.CTkFrame(t, corner_radius=12, fg_color=("gray90", "#1e293b"))
        log_card.pack(fill="both", expand=True, padx=4, pady=(4, 4))

        # Native tk.Text inside frame for tag coloring
        container = ctk.CTkFrame(log_card, fg_color="transparent")
        container.pack(fill="both", expand=True, padx=12, pady=12)

        vsb = tk.Scrollbar(container)
        vsb.pack(side="right", fill="y")

        self.log_text = tk.Text(
            container, font=("Consolas", 9), relief="flat", bd=0,
            bg="#0f172a" if current_theme == "dark" else "#ffffff",
            fg="#f8fafc" if current_theme == "dark" else "#0f172a",
            state="disabled", wrap="word", padx=10, pady=8, yscrollcommand=vsb.set
        )
        self.log_text.pack(fill="both", expand=True)
        vsb.config(command=self.log_text.yview)

        self._setup_log_tags()

    def _setup_log_tags(self):
        th = current_theme
        for tag, colors in LOG_TAGS.items():
            self.log_text.tag_configure(tag, foreground=colors[th])
        self.log_text.tag_configure("BOLD", font=("Consolas", 9, "bold"))

    BADGES = {"THINKING":" THK ","SUCCESS":" OK  ","ERROR":" ERR ","WARN":" WRN ",
              "TIER1":" T1  ","TIER2":" T2  ","SEND":" SND ",
              "INFO":" --- ","SEP":""}

    def _global_log(self, msg: str, level: str = "INFO"):
        # Always write persistent log file to disk
        try:
            log_dir = r"e:\exe" if os.path.exists(r"e:\exe") else os.path.expanduser("~\\.claude_suite")
            os.makedirs(log_dir, exist_ok=True)
            log_file = os.path.join(log_dir, "live_system.log")
            ts_date = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
            with open(log_file, "a", encoding="utf-8") as f:
                f.write(f"[{ts_date}] [{level}] {msg}\n")
        except Exception:
            pass

        if not hasattr(self, "log_text"):
            return
        ts = datetime.datetime.now().strftime("%H:%M:%S")
        self.log_count += 1

        def _insert():
            # Update Header Thinking Pill Status
            if level == "THINKING":
                self.lbl_think_txt.configure(text="AI Thinking...", text_color="#c084fc")
            elif level in ["SUCCESS", "ERROR"]:
                self.lbl_think_txt.configure(text="AI Ready", text_color=("gray30", "gray70"))

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
            self.lbl_log_cnt.configure(text=f"{self.log_count} dòng")

        self.after(0, _insert)

    def _clear_log(self):
        self.log_text.configure(state="normal")
        self.log_text.delete("1.0", "end")
        self.log_text.configure(state="disabled")
        self.log_count = 0
        self.lbl_log_cnt.configure(text="0 dòng")

    def _copy_log(self):
        content = self.log_text.get("1.0", "end").strip()
        self.clipboard_clear()
        self.clipboard_append(content)
        self._global_log("Đã copy log vào clipboard!", "SUCCESS")

    def _open_log_file(self):
        log_dir = r"e:\exe" if os.path.exists(r"e:\exe") else os.path.expanduser("~\\.claude_suite")
        log_file = os.path.join(log_dir, "live_system.log")
        if os.path.exists(log_file):
            os.startfile(log_file)
            self._global_log(f"Đã mở file log: {log_file}", "INFO")
        else:
            messagebox.showinfo("Thông báo", f"Chưa có file log tại: {log_file}")

    # ── Actions ────────────────────────────────────────────────────────────

    def _new_agent(self): self._agent_edit_window(None)
    def _edit_agent(self, agent: Agent): self._agent_edit_window(agent)

    def _reset_agents_to_defaults(self):
        """Xóa tất cả agents và reset về 7 Enterprise Corporate Roles mặc định."""
        if not messagebox.askyesno(
            "🧹 Reset Agents",
            "Xóa TẤT CẢ agents hiện tại và khôi phục 7 Enterprise Corporate Roles mặc định?\n\n"
            "• CEO / Director of Engineering\n"
            "• Business Analyst (BA)\n"
            "• Technical Project Manager (PM)\n"
            "• Chief Architect & Tech Lead\n"
            "• Senior Fullstack Developer\n"
            "• Senior Code Reviewer & Security Auditor\n"
            "• Lead QA & Test Automation Specialist\n\n"
            "⚠️ Hành động này KHÔNG THỂ hoàn tác!",
            parent=self
        ):
            return
        self.registry.reset_to_defaults()
        self._agent_current_page = 0
        self._refresh_agents(force=True)
        self._refresh_agent_combobox()
        messagebox.showinfo("✅ Hoàn tất", "Đã reset về 7 Enterprise Corporate Roles mặc định.", parent=self)

    def _clear_all_agents(self):
        """Xóa toàn bộ Agents để đưa về trạng thái trống ban đầu (Blanket Default)."""
        if not messagebox.askyesno(
            "🗑 Xóa Hết Agents",
            "Bạn có chắc muốn xóa TẤT CẢ Agents hiện tại?\n\n"
            "Danh sách Agents sẽ trở về trạng thái TRỐNG BAN ĐẦU (Blanket Default).\n"
            "Khi có Tasks mới, hệ thống sẽ TỰ ĐỘNG TẠO Agents phù hợp theo yêu cầu.",
            parent=self
        ):
            return
        self.registry.delete_all()
        self._agent_current_page = 0
        self._refresh_agents(force=True)
        self._refresh_agent_combobox()
        messagebox.showinfo("✅ Hoàn tất", "Đã xóa toàn bộ Agents. Trạng thái hiện tại: Blanket Empty.", parent=self)

    def _show_agent_detail_modal(self, agent: Agent):
        win = ctk.CTkToplevel(self)
        win.title(f"🔍 Agent Inspector: {agent.name}")
        win.geometry("640x540")
        win.grab_set()

        pad = ctk.CTkFrame(win, fg_color="transparent")
        pad.pack(fill="both", expand=True, padx=20, pady=16)

        hdr = ctk.CTkFrame(pad, fg_color="transparent")
        hdr.pack(fill="x", pady=(0, 10))

        ctk.CTkLabel(hdr, text=f"{agent.icon}  {agent.name}", font=ctk.CTkFont(size=15, weight="bold"), text_color=("gray10", "gray95")).pack(side="left")

        st_b = ctk.CTkFrame(hdr, corner_radius=10, fg_color="#22c55e" if agent.status=="running" else "#ef4444" if agent.status=="error" else "#64748b")
        st_b.pack(side="right")
        ctk.CTkLabel(st_b, text=f"{agent.status_dot} {agent.status.upper()}", font=ctk.CTkFont(size=10, weight="bold"), text_color="#fff").pack(padx=8, pady=2)

        # Specs
        specs = ctk.CTkFrame(pad, corner_radius=8, fg_color=("gray95", "#0f172a"))
        specs.pack(fill="x", pady=(0, 10), padx=2)
        ctk.CTkLabel(specs, text=f"• Model: {agent.model}   • Tasks Done: {agent.tasks_done}   • Session ID: {agent.session_id or 'None'}", font=ctk.CTkFont(size=11), text_color=("gray30", "gray70")).pack(padx=12, pady=8, anchor="w")

        # System Prompt
        ctk.CTkLabel(pad, text="SYSTEM PROMPT (CHUYÊN MÔN):", font=ctk.CTkFont(size=11, weight="bold"), text_color=("gray30", "gray70")).pack(anchor="w", pady=(0, 2))
        sys_txt = ctk.CTkTextbox(pad, height=120, font=ctk.CTkFont(size=11), corner_radius=6)
        sys_txt.pack(fill="x", pady=(0, 10))
        sys_txt.insert("1.0", agent.system or "(Chưa thiết lập system prompt)")

        # Realtime Activity & Last Task Monitor
        ctk.CTkLabel(pad, text="⚡ HOẠT ĐỘNG REALTIME & TRẠNG THÁI AI:", font=ctk.CTkFont(size=11, weight="bold"), text_color=("#38bdf8" if current_theme=="dark" else "#0284c7")).pack(anchor="w", pady=(0, 2))
        log_txt = ctk.CTkTextbox(pad, font=ctk.CTkFont(family="Consolas", size=10), corner_radius=6)
        log_txt.pack(fill="both", expand=True, pady=(0, 12))

        def _update_live_modal():
            if not win.winfo_exists():
                return
            fresh = self.registry.get(agent.agent_id) or agent
            st_b.configure(fg_color="#22c55e" if fresh.status=="running" else "#ef4444" if fresh.status=="error" else "#64748b")
            st_text = (
                f"🟢 STATUS: {fresh.status.upper()}\n"
                f"📌 TASK ĐANG/VỪA XỬ LÝ: {fresh.last_task or 'Chưa có'}\n"
                f"⚡ HÀNH ĐỘNG REALTIME: {fresh.notes or 'Chờ tác vụ...'}\n"
                f"⚠️ LỖI GẦN NHẤT: {fresh.last_error or 'None'}\n"
            )
            log_txt.delete("1.0", "end")
            log_txt.insert("1.0", st_text)
            if fresh.status == "running":
                win.after(800, _update_live_modal)

        _update_live_modal()

        btn_r = ctk.CTkFrame(pad, fg_color="transparent")
        btn_r.pack(fill="x")
        ctk.CTkButton(btn_r, text="📜 Xem Memory SQLite", height=32, fg_color=("#0284c7", "#0284c7"), hover_color=("#0369a1", "#0369a1"), corner_radius=6, command=lambda: (win.destroy(), self._view_agent_memory_modal(agent))).pack(side="left")
        ctk.CTkButton(btn_r, text="Đóng", width=80, height=32, fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"), hover_color=("gray70", "gray35"), corner_radius=6, command=win.destroy).pack(side="right")

    def _show_task_detail_modal(self, task: Task):
        win = ctk.CTkToplevel(self)
        win.title(f"🔍 Task Detail Inspector: TSK-{task.task_id[:4].upper()}")
        win.geometry("640x540")
        win.grab_set()

        pad = ctk.CTkFrame(win, fg_color="transparent")
        pad.pack(fill="both", expand=True, padx=20, pady=16)

        # Header: Key + Status + Priority
        hdr = ctk.CTkFrame(pad, fg_color="transparent")
        hdr.pack(fill="x", pady=(0, 10))

        key_b = ctk.CTkFrame(hdr, corner_radius=6, fg_color=("#e2e8f0", "#1e293b"))
        key_b.pack(side="left")
        ctk.CTkLabel(key_b, text=f"TSK-{task.task_id[:4].upper()}", font=ctk.CTkFont(size=11, weight="bold"), text_color="#0284c7").pack(padx=8, pady=2)

        st_b = ctk.CTkFrame(hdr, corner_radius=6, fg_color=("#0284c7" if task.status=="running" else "#22c55e" if task.status=="done" else "#ef4444" if task.status=="failed" else "#64748b"))
        st_b.pack(side="left", padx=8)
        ctk.CTkLabel(st_b, text=f"{task.status_icon} {task.status.upper()}", font=ctk.CTkFont(size=10, weight="bold"), text_color="#fff").pack(padx=8, pady=2)

        pri_b = ctk.CTkFrame(hdr, corner_radius=6, fg_color="#eab308" if task.priority=="high" else "#ef4444" if task.priority=="urgent" else "#64748b")
        pri_b.pack(side="left")
        ctk.CTkLabel(pri_b, text=task.priority.upper(), font=ctk.CTkFont(size=10, weight="bold"), text_color="#fff").pack(padx=8, pady=2)

        # Title
        ctk.CTkLabel(pad, text=task.title, font=ctk.CTkFont(size=14, weight="bold"), text_color=("gray10", "gray95"), wraplength=580, justify="left").pack(anchor="w", pady=(0, 10))

        # Assigned Agent Row
        ag_row = ctk.CTkFrame(pad, fg_color="transparent")
        ag_row.pack(fill="x", pady=(0, 10))
        ctk.CTkLabel(ag_row, text="Assigned Agent:", font=ctk.CTkFont(size=11, weight="bold"), text_color=("gray30", "gray70")).pack(side="left", padx=(0, 8))

        agents = self.registry.list_all()
        ag_names = ["None"] + [f"{a.icon} {a.name}" for a in agents]
        curr_ag_idx = 0
        for i, a in enumerate(agents, 1):
            if a.agent_id == task.assigned_to:
                curr_ag_idx = i; break

        ag_var = ctk.StringVar(value=ag_names[curr_ag_idx])
        def _on_change_ag(val):
            sel_id = None
            for a in agents:
                if a.name in val: sel_id = a.agent_id; break
            self.board.update_assigned(task.task_id, sel_id)
            self._refresh_tasks()
        ctk.CTkOptionMenu(ag_row, variable=ag_var, values=ag_names, command=_on_change_ag, height=28, width=220).pack(side="left")

        # Prompt / Description
        ctk.CTkLabel(pad, text="NỘI DUNG PROMPT / MÔ TẢ:", font=ctk.CTkFont(size=11, weight="bold"), text_color=("gray30", "gray70")).pack(anchor="w", pady=(0, 2))
        p_txt = ctk.CTkTextbox(pad, height=70, font=ctk.CTkFont(size=11), corner_radius=6)
        p_txt.pack(fill="x", pady=(0, 10))
        p_txt.insert("1.0", task.prompt or task.description or "(Khởi tạo)")

        # Result / Execution Output
        ctk.CTkLabel(pad, text="📄 KẾT QUẢ THỰC THI (RESULT / LIVE LOG):", font=ctk.CTkFont(size=11, weight="bold"), text_color=("gray30", "gray70")).pack(anchor="w", pady=(0, 2))
        res_txt = ctk.CTkTextbox(pad, font=ctk.CTkFont(family="Consolas", size=10), corner_radius=6)
        res_txt.pack(fill="both", expand=True, pady=(0, 12))
        res_txt.insert("1.0", task.result or "(Chưa có kết quả thực thi)")

        # Bottom Actions
        btn_r = ctk.CTkFrame(pad, fg_color="transparent")
        btn_r.pack(fill="x")

        if task.status in ["backlog", "queued", "failed"]:
            def _run_t():
                win.destroy()
                self.board.update_status(task.task_id, "queued")
                self._refresh_tasks()
                self._global_log(f"Manual trigger task '{task.title}'", "SEND")
            ctk.CTkButton(btn_r, text="▶ Run Task", width=100, height=32, fg_color=("#16a34a", "#22c55e"), hover_color=("#15803d", "#16a34a"), corner_radius=6, command=_run_t).pack(side="left", padx=(0, 6))

        ctk.CTkButton(btn_r, text="🗑 Delete Task", width=100, height=32, fg_color=("#fee2e2", "#450a0a"), text_color="#ef4444", hover_color=("#fca5a5", "#7f1d1d"), corner_radius=6, command=lambda: (self.board.delete(task.task_id), win.destroy(), self._refresh_tasks())).pack(side="left")
        ctk.CTkButton(btn_r, text="Đóng", width=80, height=32, fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"), hover_color=("gray70", "gray35"), corner_radius=6, command=win.destroy).pack(side="right")

    def _agent_edit_window(self, agent: Agent = None):
        win = ctk.CTkToplevel(self)
        win.title("New Agent" if agent is None else f"Edit: {agent.name}")
        win.geometry("540x520")
        win.grab_set()

        pad = ctk.CTkFrame(win, fg_color="transparent")
        pad.pack(fill="both", expand=True, padx=20, pady=16)

        ctk.CTkLabel(pad, text="NAME", font=ctk.CTkFont(size=11, weight="bold")).pack(anchor="w", pady=(0, 2))
        name_var = ctk.StringVar(value=agent.name if agent else "My Agent")
        ctk.CTkEntry(pad, textvariable=name_var, font=ctk.CTkFont(size=13)).pack(fill="x", pady=(0, 10))

        ctk.CTkLabel(pad, text="MODEL", font=ctk.CTkFont(size=11, weight="bold")).pack(anchor="w", pady=(0, 2))
        model_var = ctk.StringVar(value=agent.model if agent else "claude-opus-4-8")
        ctk.CTkOptionMenu(
            pad, variable=model_var, values=[
                "claude-opus-4-8", "claude-sonnet-4-5", "claude-haiku-4-5",
                "gemini-3.6-flash-high", "gemini-3.6-flash-medium", "gemini-3.6-flash-low",
                "gemini-3.5-flash-high", "gemini-3.5-flash-medium", "gemini-3.5-flash-low",
                "gemini-3.1-pro-high", "gemini-3.1-pro-low",
                "claude-sonnet-4.6-thinking", "claude-opus-4.6-thinking"
            ]
        ).pack(fill="x", pady=(0, 10))

        ctk.CTkLabel(pad, text="ICON", font=ctk.CTkFont(size=11, weight="bold")).pack(anchor="w", pady=(0, 2))
        icon_var = ctk.StringVar(value=agent.icon if agent else "🤖")
        icon_row = ctk.CTkFrame(pad, fg_color="transparent")
        icon_row.pack(fill="x", pady=(0, 10))

        ctk.CTkEntry(icon_row, textvariable=icon_var, width=50, font=ctk.CTkFont(size=16)).pack(side="left", padx=(0, 8))
        for ic in ["🤖","🔍","💻","🧪","📝","🗺","⚡","🛡"]:
            ctk.CTkButton(
                icon_row, text=ic, width=32, height=32,
                fg_color=("gray85", "gray25"), hover_color=("gray75", "gray35"),
                corner_radius=6, command=lambda i=ic: icon_var.set(i)
            ).pack(side="left", padx=2)

        ctk.CTkLabel(pad, text="SYSTEM PROMPT", font=ctk.CTkFont(size=11, weight="bold")).pack(anchor="w", pady=(0, 2))
        sys_text = ctk.CTkTextbox(pad, height=100, font=ctk.CTkFont(size=12))
        sys_text.pack(fill="both", expand=True, pady=(0, 10))
        if agent:
            sys_text.insert("1.0", agent.system)

        def _save():
            name = name_var.get().strip()
            if not name:
                self.show_toast("Nhập tên agent!", "warning"); return
            if agent:
                agent.name   = name
                agent.model  = model_var.get()
                agent.icon   = icon_var.get()
                agent.system = sys_text.get("1.0", "end").strip()
                self.registry.update(agent)
                self._global_log(f"Đã cập nhật agent: {name}", "SUCCESS")
            else:
                new = Agent(name=name, model=model_var.get(),
                            icon=icon_var.get(),
                            system=sys_text.get("1.0","end").strip())
                self.registry.create(new)
                self._global_log(f"Đã tạo agent mới: {name}", "SUCCESS")
            self._refresh_agents(force=True)
            self._refresh_agent_combobox()
            win.destroy()

        btn_row = ctk.CTkFrame(pad, fg_color="transparent")
        btn_row.pack(fill="x", pady=(4, 0))

        ctk.CTkButton(
            btn_row, text="💾 Save Agent", height=38,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("#0284c7", "#0284c7"), hover_color=("#0369a1", "#0369a1"),
            corner_radius=8, command=_save
        ).pack(side="left", padx=(0, 8))

        ctk.CTkButton(
            btn_row, text="Cancel", height=38, width=90,
            font=ctk.CTkFont(size=12),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=8,
            command=win.destroy
        ).pack(side="left")

    def _delete_agent(self, agent: Agent):
        if messagebox.askyesno("Xác nhận", f"Xóa agent '{agent.name}'?"):
            self.registry.delete(agent.agent_id)
            self._refresh_agents(force=True)
            self._refresh_agent_combobox()
            self._global_log(f"Đã xóa agent: {agent.name}", "WARN")

    def _reset_agent_session(self, agent: Agent):
        self.registry.reset_session(agent.agent_id)
        self.registry.update_status(agent.agent_id, "idle")
        self._refresh_agents(force=True)
        self._global_log(f"Đã reset session agent: {agent.name}", "INFO")

    def _run_agent_dialog(self, agent: Agent):
        self._run_agent_modal(agent)

    def _view_agent_memory_modal(self, agent: Agent):
        win = ctk.CTkToplevel(self)
        win.title(f"📜 SQLite Memory History: {agent.name}")
        win.geometry("640x500")
        win.grab_set()

        pad = ctk.CTkFrame(win, fg_color="transparent")
        pad.pack(fill="both", expand=True, padx=20, pady=16)

        hdr = ctk.CTkFrame(pad, fg_color="transparent")
        hdr.pack(fill="x", pady=(0, 10))

        ctk.CTkLabel(
            hdr, text=f"{agent.icon}  Memory Log: {agent.name}",
            font=ctk.CTkFont(size=15, weight="bold"),
            text_color=("gray10", "gray95")
        ).pack(side="left")

        ctk.CTkButton(
            hdr, text="🗑 Clear Memory", width=110, height=28,
            font=ctk.CTkFont(size=11),
            fg_color=("#fee2e2", "#450a0a"), text_color="#ef4444",
            hover_color=("#fca5a5", "#7f1d1d"), corner_radius=6,
            command=lambda: (self.memory.clear_agent_memory(agent.agent_id), win.destroy(), messagebox.showinfo("OK", "Đã xóa bộ nhớ agent trong SQLite."))
        ).pack(side="right")

        mem_list = self.memory.get_agent_memory(agent.agent_id, limit=50)

        mem_box = ctk.CTkTextbox(pad, font=ctk.CTkFont(family="Consolas", size=11), corner_radius=8)
        mem_box.pack(fill="both", expand=True, pady=(0, 10))

        if not mem_list:
            mem_box.insert("1.0", "(Chưa có dữ liệu memory trong SQLite cho agent này.)")
        else:
            for item in reversed(mem_list):
                role_icon = "👤 User" if item.role == "user" else "🤖 Assistant"
                ts = item.timestamp[:19] if item.timestamp else ""
                mem_box.insert("end", f"[{ts}] {role_icon}:\n{item.content}\n" + "-"*50 + "\n\n")

        ctk.CTkButton(
            pad, text="Đóng", height=36, width=100,
            font=ctk.CTkFont(size=12),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=8,
            command=win.destroy
        ).pack(anchor="e")

    def _run_agent_modal(self, agent: Agent):
        win = ctk.CTkToplevel(self)
        win.title(f"▶ Run Agent: {agent.name}")
        win.geometry("560x420")
        win.grab_set()

        pad = ctk.CTkFrame(win, fg_color="transparent")
        pad.pack(fill="both", expand=True, padx=20, pady=16)

        # Header
        hdr = ctk.CTkFrame(pad, fg_color="transparent")
        hdr.pack(fill="x", pady=(0, 10))

        ctk.CTkLabel(
            hdr, text=f"{agent.icon}  Run Agent: {agent.name}",
            font=ctk.CTkFont(size=15, weight="bold"),
            text_color=("gray10", "gray95")
        ).pack(side="left")

        # Model badge
        model_badge = ctk.CTkFrame(hdr, corner_radius=10, fg_color="#4338ca")
        model_badge.pack(side="right")
        ctk.CTkLabel(
            model_badge, text=agent.model_short,
            font=ctk.CTkFont(size=10, weight="bold"), text_color="#ffffff"
        ).pack(padx=8, pady=2)

        ctk.CTkLabel(
            pad, text="PROMPT NỘI DUNG (HỆ THỐNG ĐÃ TỰ ĐỘNG NẠP KHUNG PROMPT CHUẨN ENTERPRISE)",
            font=ctk.CTkFont(size=11, weight="bold"), text_color=("gray30", "gray70")
        ).pack(anchor="w", pady=(0, 4))

        prompt_txt = ctk.CTkTextbox(pad, height=160, font=ctk.CTkFont(size=11), corner_radius=8)
        prompt_txt.pack(fill="both", expand=True, pady=(0, 10))

        # Tailored preset 5-part enterprise prompt dictionaries per Agent archetype
        agent_name = agent.name.lower()

        PROMPT_PRESETS = {
            "Strategic Vision": f"""[ROLE]: {agent.name} (Chief Executive Officer)
[CONTEXT]: Enterprise Product Strategy & Digital Transformation Roadmap.
[ACTION]: Analyze market demand, define core value propositions, and outline executive product milestones.
[NOTE / DIRECTIVE]: Focus on ROI, business scalability, and long-term competitive moat.
[OUTPUT FORMAT]: Executive Strategic Plan in Markdown format.""",

            "SRS & User Stories": f"""[ROLE]: {agent.name} (Lead Business Analyst)
[CONTEXT]: Software Requirements Specification (SRS) & Agile Backlog Creation.
[ACTION]: Draft Epics and User Stories with Given-When-Then Acceptance Criteria and input validation rules.
[NOTE / DIRECTIVE]: Ensure zero functional ambiguity between business stakeholders and developers.
[OUTPUT FORMAT]: Formal SRS Document with Epics & User Stories.""",

            "Sprint WBS Plan": f"""[ROLE]: {agent.name} (Technical Project Manager)
[CONTEXT]: Agile Sprint Planning & Work Breakdown Structure (WBS).
[ACTION]: Decompose project scope into sequential atomic task trees with explicit dependencies and priority ranks.
[NOTE / DIRECTIVE]: Mitigate project bottlenecks and allocate optimal effort estimates.
[OUTPUT FORMAT]: WBS Execution Schedule & Risk Matrix Table.""",

            "System Architecture": f"""[ROLE]: {agent.name} (Chief Solution Architect)
[CONTEXT]: High-Level Cloud Software Architecture & Microservices Design.
[ACTION]: Design C4 architecture models, relational DB schemas, and OpenAPI 3.0 REST/gRPC endpoints.
[NOTE / DIRECTIVE]: Enforce SOLID principles, high availability, and security standards.
[OUTPUT FORMAT]: System Architecture Blueprint with Mermaid Sequence Diagrams.""",

            "Implement Feature": f"""[ROLE]: {agent.name} (Senior Fullstack Developer)
[CONTEXT]: Production-Ready Software Implementation & API Endpoints.
[ACTION]: Implement type-safe, modular, production-grade source code with full error handling.
[NOTE / DIRECTIVE]: NEVER use pseudo-code or '// TODO' placeholders. Code must be 100% complete and runnable.
[OUTPUT FORMAT]: Complete Source Code Files with Docstrings.""",

            "OWASP Security Audit": f"""[ROLE]: {agent.name} (Senior Security Auditor)
[CONTEXT]: OWASP Top 10 Vulnerability Scan & SAST Security Audit.
[ACTION]: Inspect codebase for SQLi, XSS, CSRF, RCE, IDOR, and auth bypasses. Provide patch diffs.
[NOTE / DIRECTIVE]: Rank findings by CVSS v3.1 severity scores (CRITICAL, HIGH, MEDIUM).
[OUTPUT FORMAT]: Security Audit Report with CVSS Scores & Code Refactoring Diffs.""",

            "Write PyTest Suite": f"""[ROLE]: {agent.name} (Lead QA & Test Specialist)
[CONTEXT]: Automated Test Suite Design & Edge Case Validation.
[ACTION]: Write automated PyTest/Jest test scripts covering happy paths, negative boundaries, and API mocks.
[NOTE / DIRECTIVE]: Ensure 100% assertion coverage with clear test function names.
[OUTPUT FORMAT]: Runnable PyTest/Jest Test Suite Code Files."""
        }

        # Select primary preset & default pre-filled prompt based on agent role
        if "ceo" in agent_name or "director" in agent_name:
            primary_preset = "Strategic Vision"
            preset_keys = ["Strategic Vision", "SRS & User Stories", "Sprint WBS Plan", "System Architecture"]
        elif "ba" in agent_name or "analyst" in agent_name:
            primary_preset = "SRS & User Stories"
            preset_keys = ["SRS & User Stories", "Sprint WBS Plan", "System Architecture", "Implement Feature"]
        elif "pm" in agent_name or "manager" in agent_name:
            primary_preset = "Sprint WBS Plan"
            preset_keys = ["Sprint WBS Plan", "SRS & User Stories", "System Architecture", "OWASP Security Audit"]
        elif "architect" in agent_name or "tech lead" in agent_name:
            primary_preset = "System Architecture"
            preset_keys = ["System Architecture", "Implement Feature", "OWASP Security Audit", "Write PyTest Suite"]
        elif "developer" in agent_name or "engineer" in agent_name or "dev" in agent_name:
            primary_preset = "Implement Feature"
            preset_keys = ["Implement Feature", "System Architecture", "OWASP Security Audit", "Write PyTest Suite"]
        elif "security" in agent_name or "reviewer" in agent_name or "audit" in agent_name:
            primary_preset = "OWASP Security Audit"
            preset_keys = ["OWASP Security Audit", "System Architecture", "Implement Feature", "Write PyTest Suite"]
        elif "qa" in agent_name or "test" in agent_name:
            primary_preset = "Write PyTest Suite"
            preset_keys = ["Write PyTest Suite", "Implement Feature", "OWASP Security Audit", "System Architecture"]
        else:
            primary_preset = "Implement Feature"
            preset_keys = ["Implement Feature", "System Architecture", "OWASP Security Audit", "Write PyTest Suite"]

        # Auto-fill full 5-part prompt into textbox immediately upon modal load
        default_prompt = agent.system if (agent.system and "[NOTE" in agent.system) else PROMPT_PRESETS[primary_preset]
        prompt_txt.insert("1.0", default_prompt)

        # Preset prompt label
        ctk.CTkLabel(
            pad, text="💡 GỢI Ý NẠP QUICK PROMPT MẪU ENTERPRISE",
            font=ctk.CTkFont(size=10, weight="bold"), text_color=("gray40", "gray60")
        ).pack(anchor="w", pady=(0, 2))

        # Preset prompts grid (2x2 layout)
        p_frame = ctk.CTkFrame(pad, fg_color="transparent")
        p_frame.pack(fill="x", pady=(0, 12))
        p_frame.columnconfigure(0, weight=1)
        p_frame.columnconfigure(1, weight=1)

        def _apply_preset(key):
            full_p = PROMPT_PRESETS.get(key, default_prompt)
            prompt_txt.delete("1.0", "end")
            prompt_txt.insert("1.0", full_p)

        for i, key in enumerate(preset_keys):
            r = i // 2
            c = i % 2
            ctk.CTkButton(
                p_frame, text=f"⚡ {key}", height=28,
                fg_color=("gray85", "gray25"), text_color=("gray10", "gray90"),
                hover_color=("#7c3aed", "#7c3aed"), corner_radius=6,
                font=ctk.CTkFont(size=11, weight="bold"),
                command=lambda k=key: _apply_preset(k)
            ).grid(row=r, column=c, padx=3, pady=2, sticky="ew")

        def _execute():
            p = prompt_txt.get("1.0", "end").strip()
            if not p:
                self.show_toast("Vui lòng nhập nội dung prompt!", "warning"); return
            win.destroy()
            self._global_log(f"Manual run agent '{agent.name}'...", "SEND")
            self.orchestr.run_now(agent, p,
                on_done=lambda r: self.after(0, lambda: self._refresh_agents(force=True)))

        btn_row = ctk.CTkFrame(pad, fg_color="transparent")
        btn_row.pack(fill="x")

        ctk.CTkButton(
            btn_row, text="▶  Chạy Agent Ngay", height=38,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("#0284c7", "#0284c7"), hover_color=("#0369a1", "#0369a1"),
            corner_radius=8, command=_execute
        ).pack(side="left", expand=True, fill="x", padx=(0, 8))

        ctk.CTkButton(
            btn_row, text="Hủy", height=38, width=80,
            font=ctk.CTkFont(size=12),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=8,
            command=win.destroy
        ).pack(side="left")

    def _refresh_agent_combobox(self):
        agents = self.registry.list_all()
        vals = [f"{a.icon} {a.name} ({a.model_short})" for a in agents]
        if hasattr(self, "run_agent_cb"):
            self.run_agent_cb.configure(values=vals if vals else ["(Chưa có agent)"])
            if vals:
                self.run_agent_var.set(vals[0])
        self._agents_list = agents

    def _new_task_dialog(self):
        win = ctk.CTkToplevel(self)
        win.title("New Task")
        win.geometry("480x400")
        win.grab_set()

        pad = ctk.CTkFrame(win, fg_color="transparent")
        pad.pack(fill="both", expand=True, padx=20, pady=16)

        ctk.CTkLabel(pad, text="TASK TITLE", font=ctk.CTkFont(size=11, weight="bold")).pack(anchor="w", pady=(0, 2))
        title_var = ctk.StringVar()
        ctk.CTkEntry(pad, textvariable=title_var, font=ctk.CTkFont(size=13)).pack(fill="x", pady=(0, 10))

        ctk.CTkLabel(pad, text="PROMPT / DESCRIPTION", font=ctk.CTkFont(size=11, weight="bold")).pack(anchor="w", pady=(0, 2))
        prompt_txt = ctk.CTkTextbox(pad, height=120, font=ctk.CTkFont(size=12))
        prompt_txt.pack(fill="both", expand=True, pady=(0, 10))

        row = ctk.CTkFrame(pad, fg_color="transparent")
        row.pack(fill="x", pady=(0, 12))

        ctk.CTkLabel(row, text="Priority:", font=ctk.CTkFont(size=12)).pack(side="left", padx=(0, 8))
        pri_var = ctk.StringVar(value="normal")
        ctk.CTkOptionMenu(row, variable=pri_var, values=["low","normal","high","urgent"], width=120).pack(side="left")

        def _save():
            t = title_var.get().strip()
            p = prompt_txt.get("1.0", "end").strip()
            if not t:
                self.show_toast("Nhập tiêu đề!", "warning"); return
            task = Task(title=t, description=p, prompt=p, priority=pri_var.get())
            self.board.add(task)
            self._refresh_tasks()
            self._global_log(f"Đã thêm task: '{t}'", "SUCCESS")
            win.destroy()

        ctk.CTkButton(
            pad, text="💾 Add Task", height=38,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("#0284c7", "#0284c7"), hover_color=("#0369a1", "#0369a1"),
            corner_radius=8, command=_save
        ).pack(side="left")

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
                f"Result:\n{task.result or '(Chưa có)'}")
        messagebox.showinfo(f"Task Detail: {task.title}", info)

    def _clear_done_tasks(self):
        done = self.board.list_by_status("done") + self.board.list_by_status("failed")
        for t in done:
            self.board.delete(t.task_id)
            ref = getattr(self, "_jira_card_map", {}).pop(t.task_id, None)
            if ref and ref.get("card"):
                try: ref["card"].destroy()
                except Exception: pass
        self._refresh_tasks()
        self._global_log(f"Đã dọn dẹp {len(done)} tasks đã hoàn thành/thất bại khỏi Kanban Board.", "SUCCESS")

    def _clear_all_tasks(self):
        all_tasks = self.board.list_all()
        if not all_tasks:
            messagebox.showinfo("Thông báo", "Bảng Kanban hiện đang trống.")
            return

        if not messagebox.askyesno("Xác nhận xóa toàn bộ", f"Bạn có chắc chắn muốn xóa TOÀN BỘ {len(all_tasks)} Tasks hiện tại khỏi Kanban Board?"):
            return

        for t in all_tasks:
            self.board.delete(t.task_id)
            ref = getattr(self, "_jira_card_map", {}).pop(t.task_id, None)
            if ref and ref.get("card"):
                try: ref["card"].destroy()
                except Exception: pass

        self._jira_card_map = {}
        self._refresh_tasks()
        self._global_log(f"🔥 Đã xóa sạch toàn bộ {len(all_tasks)} Tasks khỏi Kanban Board.", "WARN")

    def _run_decompose(self):
        project = self.plan_input.get("1.0", "end").strip()
        if not project:
            self.show_toast("Nhập mô tả project!", "warning"); return
        self.btn_decompose.configure(state="disabled", text="⏳ Đang xử lý AI...")
        self.plan_status.configure(text="Đang gọi Claude Opus 4.8...", text_color="#f59e0b")
        self._pending_plan = []

        def _worker():
            tasks = self.plan_bld.build_from_description(
                project, on_log=self._global_log)
            self.after(0, lambda: self._show_plan(tasks))

        def _on_err():
            self.btn_decompose.configure(state="normal", text="🗺  AI Decompose (Opus 4.8)")
            self.plan_status.configure(text="Lỗi ngầm khi tạo kế hoạch!", text_color="red")
            
        self._run_safe_thread(_worker, on_error_ui_reset=_on_err)

    def _simple_split(self):
        project = self.plan_input.get("1.0", "end").strip()
        if not project:
            self.show_toast("Nhập mô tả project!", "warning"); return
        tasks = self.plan_bld._simple_decompose(project)
        self._show_plan(tasks)

    def _show_plan(self, plan_tasks):
        self.btn_decompose.configure(state="normal", text="🗺  AI Decompose (Opus 4.8)")
        self._pending_plan = plan_tasks
        self.plan_list.delete("1.0", "end")
        for i, pt in enumerate(plan_tasks):
            dep_str = f" [Phụ thuộc: Task {','.join(pt.depends_on)}]" if pt.depends_on else ""
            self.plan_list.insert("end",
                f"[{i+1}] {pt.title}{dep_str}\n"
                f"     Model: {pt.model_hint}  Priority: {pt.priority}\n"
                f"     Prompt: {pt.prompt[:80]}...\n\n")
        self.btn_execute_plan.configure(state="normal")
        self.plan_status.configure(
            text=f"Đã tạo {len(plan_tasks)} tasks. Nhấn 'Execute Plan' để chạy.",
            text_color="#22c55e"
        )

    def _clear_plan(self):
        self.plan_list.delete("1.0", "end")
        self._pending_plan = []
        self.btn_execute_plan.configure(state="disabled")
        self.plan_status.configure(text="")

    def _execute_plan(self):
        if not self._pending_plan:
            return
        task_objects = self.plan_bld.plan_to_tasks(self._pending_plan)
        self.orchestr.execute_plan(task_objects)
        self._refresh_tasks()
        self.tabview.set("📋 Task Board & Planning")
        self._global_log(f"Execute plan: {len(task_objects)} tasks đã thêm vào board.", "SUCCESS")
        self.btn_execute_plan.configure(state="disabled")
        
        # Cập nhật UI Orchestrator
        self.btn_orch.configure(text="⏹  Stop Orchestrator")
        self.lbl_orch_dot.configure(text_color="#22c55e")
        self.lbl_orch_txt.configure(text="Orchestrator: Active", text_color="#22c55e")

    def _show_file_menu(self):
        menu = tk.Menu(
            self, tearoff=0, bg="#1e293b", fg="#f8fafc",
            activebackground="#0284c7", activeforeground="#ffffff",
            font=("Segoe UI", 10)
        )
        menu.add_command(label="📁 Open Project Folder...", command=self._select_global_workspace_folder)

        # Open Recently Submenu
        recent_menu = tk.Menu(
            menu, tearoff=0, bg="#1e293b", fg="#f8fafc",
            activebackground="#0284c7", activeforeground="#ffffff",
            font=("Segoe UI", 10)
        )
        recents = self.ws_config.recent_workspaces
        if recents:
            for idx, p in enumerate(recents, 1):
                def _make_cmd(path_val=p):
                    return lambda: self._load_workspace_folder(path_val)
                recent_menu.add_command(label=f"{idx}. 📁 {p}", command=_make_cmd(p))
            recent_menu.add_separator()
            recent_menu.add_command(label="🗑 Clear Recent History", command=self._clear_recent_workspaces)
        else:
            recent_menu.add_command(label="(Chưa có thư mục nào gần đây)", state="disabled")

        menu.add_cascade(label="🕒 Open Recently...", menu=recent_menu)
        menu.add_separator()
        menu.add_command(label="🌳 Tree Explorer", command=self._open_file_tree_explorer)
        menu.add_command(label="📄 Attach Context Files...", command=self._select_global_workspace_files)
        menu.add_command(label="🗑 Clear Workspace Context", command=self._clear_global_workspace)
        menu.add_separator()
        menu.add_command(label="❌ Exit", command=self.destroy)

        try:
            x = self.btn_menu_file.winfo_rootx()
            y = self.btn_menu_file.winfo_rooty() + self.btn_menu_file.winfo_height() + 2
            menu.post(x, y)
        except Exception:
            pass

    def _show_view_menu(self):
        menu = tk.Menu(
            self, tearoff=0, bg="#1e293b", fg="#f8fafc",
            activebackground="#0284c7", activeforeground="#ffffff",
            font=("Segoe UI", 10)
        )
        menu.add_command(label="👥 Agents Registry", command=lambda: self.tabview.set("👥 Agents"))
        menu.add_command(label="📋 Tasks Kanban Board", command=lambda: self.tabview.set("📋 Tasks Kanban"))
        menu.add_command(label="🔄 Multi-Agent Pipeline", command=lambda: self.tabview.set("🔄 Multi-Agent Pipeline"))
        menu.add_command(label="📐 AI Plan Builder", command=lambda: self.tabview.set("📐 AI Plan Builder"))
        menu.add_command(label="⏰ Win32 Scheduler", command=lambda: self.tabview.set("⏰ Win32 Scheduler"))
        menu.add_command(label="⚡ Quick CLI Runner", command=lambda: self.tabview.set("⚡ Quick CLI"))
        menu.add_command(label="📊 Live System Log", command=lambda: self.tabview.set("📊 Live System Log"))

        try:
            x = self.btn_menu_view.winfo_rootx()
            y = self.btn_menu_view.winfo_rooty() + self.btn_menu_view.winfo_height() + 2
            menu.post(x, y)
        except Exception:
            pass

    def _show_window_menu(self):
        menu = tk.Menu(
            self, tearoff=0, bg="#1e293b", fg="#f8fafc",
            activebackground="#0284c7", activeforeground="#ffffff",
            font=("Segoe UI", 10)
        )
        menu.add_command(label="💬 Floating Prompt Widget", command=self._open_floating_prompt)
        menu.add_command(label="🪄 Prompt Architect Modal", command=self._open_prompt_architect_modal)
        menu.add_separator()
        menu.add_command(label=f"🌗 Switch Theme ({'Light' if self.current_theme=='dark' else 'Dark'})", command=self._toggle_theme)
        menu.add_command(label=f"▶ Toggle Orchestrator ({'Stop' if self.orchestr._running else 'Start'})", command=self._toggle_orchestrator)
        menu.add_command(label=f"🌐 Toggle Webhook ({'Stop' if self.webhook_srv._running else 'Start'})", command=self._toggle_webhook)

        try:
            x = self.btn_menu_window.winfo_rootx()
            y = self.btn_menu_window.winfo_rooty() + self.btn_menu_window.winfo_height() + 2
            menu.post(x, y)
        except Exception:
            pass

    def _load_workspace_folder(self, folder_path: str):
        if folder_path and os.path.exists(folder_path) and os.path.isdir(folder_path):
            self._global_workspace_folder = os.path.normpath(folder_path)
            self.ws_config.add_workspace(self._global_workspace_folder)
            self._update_global_workspace_ui()
            self._global_log(f"Đã mở Workspace từ Open Recently: '{self._global_workspace_folder}'", "SUCCESS")

    def _clear_recent_workspaces(self):
        self.ws_config.clear_recents()
        self._global_log("Đã xóa lịch sử Open Recently.", "INFO")

    def _select_global_workspace_folder(self):
        initial = self._global_workspace_folder if (self._global_workspace_folder and os.path.exists(self._global_workspace_folder)) else os.path.expanduser("~")
        folder = self._pick_folder_native(title="Chọn Thư Mục Dự Án Để Làm Việc", initialdir=initial)

        if folder and os.path.exists(folder) and os.path.isdir(folder):
            self._global_workspace_folder = os.path.normpath(folder)
            self.ws_config.add_workspace(self._global_workspace_folder)
            self._update_global_workspace_ui()
            self._global_log(f"Đã đặt Workspace Dự án toàn cục: '{self._global_workspace_folder}'", "SUCCESS")

    def _pick_folder_native(self, title: str, initialdir: str) -> str:
        # 1. Try Tkinter filedialog
        try:
            folder = filedialog.askdirectory(parent=self, initialdir=initialdir, title=title)
            if folder and os.path.exists(folder):
                return os.path.normpath(folder)
        except Exception:
            pass

        # 2. Try Windows Shell SHBrowseForFolderW via ctypes
        try:
            import ctypes
            from ctypes import wintypes
            class BROWSEINFO(ctypes.Structure):
                _fields_ = [
                    ('hwndOwner', wintypes.HWND),
                    ('pidlRoot', ctypes.c_void_p),
                    ('pszDisplayName', wintypes.LPWSTR),
                    ('lpszTitle', wintypes.LPWSTR),
                    ('ulFlags', wintypes.UINT),
                    ('lpfn', ctypes.c_void_p),
                    ('lParam', wintypes.LPARAM),
                    ('iImage', ctypes.c_int)
                ]
            buf = ctypes.create_unicode_buffer(260)
            bi = BROWSEINFO()
            bi.lpszTitle = title
            bi.ulFlags = 0x00000001 | 0x00000040
            try:
                bi.hwndOwner = self.winfo_id()
            except Exception:
                pass
            pidl = ctypes.windll.shell32.SHBrowseForFolderW(ctypes.byref(bi))
            if pidl:
                ctypes.windll.shell32.SHGetPathFromIDListW(pidl, buf)
                if buf.value and os.path.exists(buf.value):
                    return os.path.normpath(buf.value)
        except Exception:
            pass

        return ""

    def _open_file_tree_explorer(self):
        if not self._global_workspace_folder or not os.path.exists(self._global_workspace_folder):
            self.show_toast("Vui lòng chọn Folder Dự Án trước!", "warning");
            self._select_global_workspace_folder()
            if not self._global_workspace_folder:
                return

        def _on_applied_files(files):
            self._global_workspace_files = files
            self._update_global_workspace_ui()
            self._global_log(f"Tree Explorer: Đã chọn {len(files)} files bối cảnh AI.", "SUCCESS")

        modal = FileTreeExplorerModal(
            parent=self,
            workspace_folder=self._global_workspace_folder,
            current_attached_files=self._global_workspace_files,
            on_apply=_on_applied_files
        )
        modal.grab_set()

    def _select_global_workspace_files(self):
        initial = self._global_workspace_folder if (self._global_workspace_folder and os.path.exists(self._global_workspace_folder)) else os.path.expanduser("~")
        files = []
        try:
            files = filedialog.askopenfilenames(parent=self, initialdir=initial, title="Đính Kèm Files Bối Cảnh Dự Án")
        except Exception:
            pass

        if files:
            self._global_workspace_files.extend([os.path.normpath(f) for f in files if os.path.exists(f)])
            self._update_global_workspace_ui()
            self._global_log(f"Đã đính kèm {len(files)} files bối cảnh toàn cục.", "SUCCESS")

    def _clear_global_workspace(self):
        self._global_workspace_files = []
        self._update_global_workspace_ui()
        self._global_log("Đã dọn sạch các file bối cảnh đính kèm toàn cục.", "INFO")

    def _update_global_workspace_ui(self):
        folder = self._global_workspace_folder
        folder_label = os.path.basename(folder) or folder if folder else "Chưa chọn"
        if hasattr(self, "btn_ws_folder_pill"):
            self.btn_ws_folder_pill.configure(
                text=f"📁 {folder_label}",
                text_color=("#22c55e" if folder else "gray50")
            )

        all_scanned = self.ctx_mgr.scan_folder(folder) if (folder and os.path.exists(folder)) else []
        all_scanned.extend(self._global_workspace_files)
        dedup_files = list(dict.fromkeys(all_scanned))
        cnt = len(dedup_files)

        if hasattr(self, "lbl_ws_summary"):
            self.lbl_ws_summary.configure(
                text=f"🟢 {cnt} Files Context",
                text_color=("#38bdf8" if self.current_theme == "dark" else "#0284c7")
            )
            
        if hasattr(self, "cockpit_frame"):
            self.cockpit_frame.set_workspace(folder)
            
        # Update Quick CLI badge as well if it exists
        if hasattr(self, "lbl_ctx_badge"):
            self._update_quick_context_badge()

    def _pick_quick_folder(self):
        desktop_dir = os.path.join(os.path.expanduser("~"), "Desktop")
        folder = filedialog.askdirectory(initialdir=desktop_dir, title="Select Workspace Folder")
        if folder:
            self._quick_context_paths.append(folder)
            self._update_quick_context_badge()

    def _pick_quick_files(self):
        desktop_dir = os.path.join(os.path.expanduser("~"), "Desktop")
        files = filedialog.askopenfilenames(initialdir=desktop_dir, title="Select Context Files")
        if files:
            self._quick_context_paths.extend(files)
            self._update_quick_context_badge()

    def _clear_quick_context(self):
        self._quick_context_paths = []
        self._update_quick_context_badge()

    def _update_quick_context_badge(self):
        cnt = len(self._quick_context_paths)
        global_ws = self._global_workspace_folder
        
        if cnt == 0 and not global_ws:
            self.lbl_ctx_badge.configure(text="Chưa chọn file/folder", text_color="gray50")
        else:
            names = []
            if global_ws:
                names.append(os.path.basename(global_ws))
            names.extend([os.path.basename(p) for p in self._quick_context_paths[:2]])
            
            total_items = (1 if global_ws else 0) + cnt
            suffix = f" (+{total_items-2} khác)" if total_items > 2 else ""
            self.lbl_ctx_badge.configure(text=f"Đã nạp: {', '.join(names[:2])}{suffix}", text_color="#38bdf8")

    def _quick_run(self):
        user_prompt = self.run_prompt.get("1.0", "end").strip()
        if not user_prompt:
            self.show_toast("Nhập prompt!", "warning"); return

        # Build workspace context combining global project workspace & local attached files
        attached_paths = []
        if self._global_workspace_folder and os.path.exists(self._global_workspace_folder):
            attached_paths.append(self._global_workspace_folder)
        attached_paths.extend(self._global_workspace_files)
        attached_paths.extend(self._quick_context_paths)

        context_str = self.ctx_mgr.build_context_prompt(attached_paths)
        full_prompt = f"{context_str}\n\n{user_prompt}" if context_str else user_prompt
        # Xóa ký tự null (nếu có do đọc nhầm file binary) để tránh lỗi 'embedded null character' của Windows subprocess
        full_prompt = full_prompt.replace("\x00", "")

        agents = getattr(self, "_agents_list", [])
        sel_str = self.run_agent_var.get()
        agent = None
        for a in agents:
            if a.name in sel_str:
                agent = a; break

        model = self.run_model_var.get()
        self.btn_run.configure(state="disabled", text="⏳ Đang chạy CLI...")
        self.run_result.delete("1.0", "end")
        self._global_log(f"Quick run: model={model} (context files={len(self._quick_context_paths)})", "SEND")

        def _worker():
            if agent:
                orig = agent.model
                agent.model = model
                result = self.cli.run_agent(agent, full_prompt, on_log=self._global_log)
                agent.model = orig
                self.registry.update(agent)
            else:
                result = self.cli.run_once(full_prompt, model=model, on_log=self._global_log)

            def _done():
                self.btn_run.configure(state="normal", text="▶  Chạy CLI Trực Tiếp")
                if result.success:
                    self.run_result.insert("1.0", result.output)
                    self._global_log(f"Hoàn thành ({result.duration_s:.1f}s)", "SUCCESS")
                else:
                    self.run_result.insert("1.0", f"ERROR: {result.error}")
                    self._global_log(f"Thất bại: {result.error[:80]}", "ERROR")
                self._refresh_agents(force=True)

            self.after(0, _done)

        def _on_err():
            self.btn_run.configure(state="normal", text="▶  Chạy CLI Trực Tiếp")
            self.run_result.insert("1.0", "Lỗi Python ngầm, xem log.")

        self._run_safe_thread(_worker, on_error_ui_reset=_on_err)

    def _prompt_engine_fallback(self, agent_name: str, error_msg: str) -> str:
        """Hiển thị popup hỏi người dùng đổi Engine khi có lỗi API. Chạy trong UI thread."""
        event = threading.Event()
        result = {"model": ""}

        def _ask():
            msg = (
                f"Agent '{agent_name}' gặp lỗi API (Quota Exceeded/Rate Limit).\n\n"
                f"Chi tiết: {error_msg[:150]}...\n\n"
                "Bạn có muốn chuyển sang dùng Antigravity CLI (Gemini 3.6 Flash) để tiếp tục không?"
            )
            ans = messagebox.askyesno("Lỗi API - Chuyển đổi AI Engine", msg, parent=self)
            if ans:
                result["model"] = "gemini-3.6-flash-high"
            event.set()

        self.after(0, _ask)
        event.wait()
        return result["model"]

    def _toggle_orchestrator(self):
        if self.orchestr._running:
            self.orchestr.stop()
            self.btn_orch.configure(text="▶  Start Orchestrator")
            self.lbl_orch_dot.configure(text_color="#64748b")
            self.lbl_orch_txt.configure(text="Orchestrator: Inactive", text_color=("gray30", "gray70"))
        else:
            self.orchestr.start()
            self.btn_orch.configure(text="⏹  Stop Orchestrator")
            self.lbl_orch_dot.configure(text_color="#22c55e")
            self.lbl_orch_txt.configure(text="Orchestrator: Active", text_color="#22c55e")

    def _start_refresh(self):
        def _tick():
            try:
                self._refresh_tasks()
                self._refresh_agents()
            except Exception:
                pass
            self.after(3000, _tick)
        self.after(3000, _tick)

    def _toggle_theme(self):
        global current_theme
        if self.current_theme == "dark":
            self.current_theme = "light"
            current_theme = "light"
            ctk.set_appearance_mode("light")
            self.btn_theme.configure(text="🌙  Dark")
        else:
            self.current_theme = "dark"
            current_theme = "dark"
            ctk.set_appearance_mode("dark")
            self.btn_theme.configure(text="☀  Light")

        self._setup_log_tags()

    # ══════════════════════════════════════════════════════════════════════
    # TAB: MULTI-AGENT PIPELINE
    # ══════════════════════════════════════════════════════════════════════

    def _build_pipeline_tab(self, parent):
        t = parent

        grid = ctk.CTkFrame(t, fg_color="transparent")
        grid.pack(fill="both", expand=True, padx=4, pady=4)
        grid.columnconfigure(0, weight=1)
        grid.columnconfigure(1, weight=1)
        grid.rowconfigure(0, weight=1)

        # Left: Config & Steps
        left_card = ctk.CTkFrame(grid, corner_radius=12, fg_color=("gray90", "#1e293b"))
        left_card.grid(row=0, column=0, sticky="nsew", padx=(0, 6), pady=4)

        ctk.CTkLabel(
            left_card, text="🎯 MỤC TIÊU DỰ ÁN PIPELINE (5-AGENT CHAIN)",
            font=ctk.CTkFont(size=12, weight="bold"),
            text_color=("gray30", "gray70")
        ).pack(anchor="w", padx=16, pady=(14, 6))

        self.pipeline_input = ctk.CTkTextbox(left_card, height=100, font=ctk.CTkFont(size=12), corner_radius=8)
        self.pipeline_input.pack(fill="x", padx=16, pady=(0, 10))
        self.pipeline_input.insert("1.0", "Xây dựng REST API bằng Python FastAPI, thêm authentication JWT và viết bộ test PyTest")

        ctk.CTkLabel(
            left_card, text="🔄 LUỒNG XỬ LÝ 5 BƯỚC NỐI TIẾP",
            font=ctk.CTkFont(size=12, weight="bold"), text_color=("gray30", "gray70")
        ).pack(anchor="w", padx=16, pady=(4, 6))

        self.pipeline_steps_scroll = ctk.CTkScrollableFrame(left_card, fg_color="transparent")
        self.pipeline_steps_scroll.pack(fill="both", expand=True, padx=16, pady=(0, 10))

        self.btn_run_pipeline = ctk.CTkButton(
            left_card, text="▶  Chạy Multi-Agent Pipeline (5 Bước)", height=40,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("#16a34a", "#22c55e"), hover_color=("#15803d", "#16a34a"),
            corner_radius=8, command=self._run_pipeline_chain
        )
        self.btn_run_pipeline.pack(fill="x", padx=16, pady=(0, 14))

        # Right: Output Viewer & Exporter
        right_card = ctk.CTkFrame(grid, corner_radius=12, fg_color=("gray90", "#1e293b"))
        right_card.grid(row=0, column=1, sticky="nsew", padx=(6, 0), pady=4)

        hdr_r = ctk.CTkFrame(right_card, fg_color="transparent")
        hdr_r.pack(fill="x", padx=16, pady=(14, 6))

        ctk.CTkLabel(
            hdr_r, text="📄 KẾT QUẢ XUẤT PIPELINE",
            font=ctk.CTkFont(size=12, weight="bold"), text_color=("gray30", "gray70")
        ).pack(side="left")

        ctk.CTkButton(
            hdr_r, text="📤 Export Report", height=30, width=120,
            font=ctk.CTkFont(size=11, weight="bold"),
            fg_color=("#0284c7", "#0284c7"), hover_color=("#0369a1", "#0369a1"),
            corner_radius=6, command=self._export_pipeline_report
        ).pack(side="right")

        self.pipeline_result_txt = ctk.CTkTextbox(right_card, font=ctk.CTkFont(family="Consolas", size=11), corner_radius=8)
        self.pipeline_result_txt.pack(fill="both", expand=True, padx=16, pady=(0, 14))

        self._active_pipeline_steps = []
        self._render_pipeline_steps_list()

    def _render_pipeline_steps_list(self):
        for w in self.pipeline_steps_scroll.winfo_children():
            w.destroy()

        if not self._active_pipeline_steps:
            goal = self.pipeline_input.get("1.0", "end").strip()
            self._active_pipeline_steps = self.pipeline_eng.create_default_software_pipeline(goal)

        for i, step in enumerate(self._active_pipeline_steps, 1):
            f = ctk.CTkFrame(self.pipeline_steps_scroll, corner_radius=8, fg_color=("gray95", "#0f172a"))
            f.pack(fill="x", pady=3)

            st_colors = {"pending": "#64748b", "running": "#f59e0b", "completed": "#22c55e", "failed": "#ef4444"}
            ctk.CTkLabel(
                f, text=f"Bước {i}: {step.role_name}", font=ctk.CTkFont(size=12, weight="bold"),
                text_color=("gray10", "gray95")
            ).pack(side="left", padx=10, pady=8)

            badge = ctk.CTkFrame(f, corner_radius=6, fg_color=st_colors.get(step.status, "#64748b"))
            badge.pack(side="right", padx=10)
            ctk.CTkLabel(badge, text=step.status.upper(), font=ctk.CTkFont(size=9, weight="bold"), text_color="#fff").pack(padx=6, pady=2)

    def _run_pipeline_chain(self):
        goal = self.pipeline_input.get("1.0", "end").strip()
        if not goal:
            self.show_toast("Vui lòng nhập mục tiêu dự án!", "warning"); return

        self.btn_run_pipeline.configure(state="disabled", text="⏳ Đang chạy Pipeline 5 bước...")
        self._active_pipeline_steps = self.pipeline_eng.create_default_software_pipeline(goal)
        self._render_pipeline_steps_list()

        def _step_cb(idx, step):
            self.after(0, self._render_pipeline_steps_list)

        def _done_cb(success, steps):
            def _ui():
                self.btn_run_pipeline.configure(state="normal", text="▶  Chạy Multi-Agent Pipeline (5 Bước)")
                self._render_pipeline_steps_list()
                self.pipeline_result_txt.delete("1.0", "end")
                for s in steps:
                    self.pipeline_result_txt.insert("end", f"=== {s.role_name.upper()} [{s.status.upper()}] ===\n{s.output}\n\n")
                if success:
                    messagebox.showinfo("Thành công", "Đã hoàn thành toàn bộ Pipeline 5 bước!")
                else:
                    messagebox.showerror("Lỗi", "Pipeline gặp sự cố ở 1 bước.")
            self.after(0, _ui)

        self.pipeline_eng.execute_pipeline_async(self._active_pipeline_steps, goal, _step_cb, _done_cb)

    def _export_pipeline_report(self):
        goal = self.pipeline_input.get("1.0", "end").strip()
        if not self._active_pipeline_steps:
            self.show_toast("Chưa có kết quả Pipeline để xuất!", "warning"); return
        res = self.exporter.export_pipeline_report(goal, self._active_pipeline_steps)
        messagebox.showinfo("Đã xuất báo cáo", f"Báo cáo Pipeline đã xuất ra Desktop:\n{res['md']}")

    def _export_kanban_report(self):
        tasks = self.board.list_all()
        if not tasks:
            self.show_toast("Chưa có Task nào để xuất!", "warning"); return
        res = self.exporter.export_kanban_report(tasks)
        messagebox.showinfo("Đã xuất báo cáo", f"Báo cáo Kanban đã xuất ra Desktop:\n- Markdown: {res['md']}\n- HTML: {res['html']}")

    def _toggle_webhook(self):
        if self.webhook_srv._running:
            self.webhook_srv.stop()
            self.btn_webhook.configure(text="🌐 Webhook: Off", fg_color=("gray85", "#1e293b"))
            self._global_log("Webhook server đã tắt.", "WARN")
        else:
            self.webhook_srv.start()
            self.btn_webhook.configure(text="🌐 Webhook: Port 9090", fg_color=("#16a34a", "#22c55e"))
            self._global_log("Webhook server đang lắng nghe tại http://localhost:9090", "SUCCESS")

    def _trigger_office_communication(self, from_agent, to_agent, message):
        if hasattr(self, 'office_frame'):
            self.office_frame.trigger_communication(from_agent, to_agent, message)

    def _trigger_office_task(self, agent_name, event, data):
        if hasattr(self, 'office_frame'):
            if event == "started":
                self.office_frame.assign_task(agent_name, data)
            elif event == "completed":
                self.office_frame.complete_task(agent_name)

    def _on_webhook_received(self, payload: dict):
        title = payload.get("title", payload.get("action", "External Webhook Event"))
        desc = json.dumps(payload, ensure_ascii=False, indent=2)
        task = Task(title=f"Webhook: {title}", description=desc, prompt=desc, priority="high")
        self.board.add(task)
        self.after(0, lambda: (self._refresh_tasks(), self._global_log(f"Received Webhook POST -> Created Task '{title}'", "SUCCESS")))

    def _on_window_close(self):
        res = messagebox.askyesnocancel(
            "Claude Suite Live Background Mode",
            "Bạn có muốn ứng dụng tiếp tục CHẠY NGẦM LIVE để Orchestrator & Webhook làm việc liên tục?\n\n• Chọn YES: Chạy ngầm (Bấm Floating Prompt trên thanh tiêu đề hoặc mở lại shortcut để hiển thị app)\n• Chọn NO: Thoát ứng dụng hoàn toàn"
        )
        if res is True:
            self.withdraw()
            self._global_log("Ứng dụng chuyển sang trạng thái CHẠY NGẦM LIVE.", "SUCCESS")
        elif res is False:
            if self.orchestr._running:
                self.orchestr.stop()
            if self.webhook_srv._running:
                self.webhook_srv.stop()
            self.destroy()

    def _show_agent_detail_modal(self, agent: Agent):
        win = ctk.CTkToplevel(self)
        win.title(f"🔍 Chi Tiết Agent: {agent.name} ({agent.model_short})")
        win.geometry("560x480")
        win.grab_set()

        pad = ctk.CTkFrame(win, fg_color="transparent")
        pad.pack(fill="both", expand=True, padx=20, pady=16)

        # Header
        hdr = ctk.CTkFrame(pad, fg_color="transparent")
        hdr.pack(fill="x", pady=(0, 12))
        ctk.CTkLabel(hdr, text=f"{agent.icon} {agent.name}", font=ctk.CTkFont(size=16, weight="bold"), text_color=("gray10", "gray95")).pack(side="left")

        st_badge = ctk.CTkFrame(hdr, corner_radius=6, fg_color="#22c55e" if agent.status=="idle" else "#0284c7" if agent.status=="running" else "#ef4444")
        st_badge.pack(side="right")
        ctk.CTkLabel(st_badge, text=f"{agent.status_dot} {agent.status.upper()}", font=ctk.CTkFont(size=10, weight="bold"), text_color="#fff").pack(padx=8, pady=2)

        # Token Quota Inspector Card
        tok_card = ctk.CTkFrame(pad, corner_radius=10, fg_color=("gray90", "#1e293b"))
        tok_card.pack(fill="x", pady=(0, 12), padx=2)

        ctk.CTkLabel(tok_card, text="📊 HẠN MỨC & DUNG LƯỢNG TOKEN (TOKEN QUOTA)", font=ctk.CTkFont(size=11, weight="bold"), text_color=("gray30", "gray70")).pack(anchor="w", padx=12, pady=(10, 4))
        
        info_str = f"• Model Engine: {agent.model}\n" \
                   f"• Hạn mức Token cấp: {agent.token_limit_max:,} tokens\n" \
                   f"• Token đã sử dụng: {agent.tokens_used:,} tokens ({agent.token_usage_percent:.1f}%)\n" \
                   f"• Token CÒN LẠI: {agent.token_remaining:,} tokens (Còn {100.0-agent.token_usage_percent:.1f}%)"
        
        ctk.CTkLabel(tok_card, text=info_str, font=ctk.CTkFont(size=11), justify="left", text_color=("gray20", "gray90")).pack(anchor="w", padx=12, pady=(0, 8))

        # Progress bar
        pbar = ctk.CTkProgressBar(tok_card, height=8, corner_radius=4)
        pbar.pack(fill="x", padx=12, pady=(0, 10))
        pbar.set(agent.token_usage_percent / 100.0)
        rem_p = 100.0 - agent.token_usage_percent
        pbar.configure(progress_color="#22c55e" if rem_p > 30 else "#f59e0b" if rem_p > 10 else "#ef4444")

        # System Prompt
        ctk.CTkLabel(pad, text="SYSTEM INSTRUCTIONS:", font=ctk.CTkFont(size=11, weight="bold"), text_color=("gray30", "gray70")).pack(anchor="w", pady=(0, 2))
        sys_txt = ctk.CTkTextbox(pad, height=110, font=ctk.CTkFont(size=11), corner_radius=6)
        sys_txt.pack(fill="both", expand=True, pady=(0, 12))
        sys_txt.insert("1.0", agent.system or "(Mặc định)")

        # Action Buttons
        btn_r = ctk.CTkFrame(pad, fg_color="transparent")
        btn_r.pack(fill="x")

        def _do_reset_tokens():
            self.registry.reset_tokens(agent.agent_id)
            win.destroy()
            self._refresh_agents()
            self._global_log(f"Đã Reset bộ đếm Token cho Agent {agent.name}!", "SUCCESS")

        ctk.CTkButton(
            btn_r, text="🔄 Reset Bộ Đếm Token", height=32,
            font=ctk.CTkFont(size=11, weight="bold"),
            fg_color=("#0284c7", "#0284c7"), hover_color=("#0369a1", "#0369a1"),
            corner_radius=6, command=_do_reset_tokens
        ).pack(side="left")

        ctk.CTkButton(
            btn_r, text="Đóng", height=32, width=80,
            font=ctk.CTkFont(size=11),
            fg_color=("gray85", "#1e293b"), text_color=("gray10", "gray90"),
            hover_color=("gray75", "#334155"), corner_radius=6,
            command=win.destroy
        ).pack(side="right")

    def _open_prompt_architect_modal(self):
        win = ctk.CTkToplevel(self)
        win.title("🪄 Enterprise Prompt Architect (Role-Context-Action-Output)")
        win.geometry("640x580")
        win.grab_set()

        pad = ctk.CTkFrame(win, fg_color="transparent")
        pad.pack(fill="both", expand=True, padx=20, pady=16)

        hdr_frame = ctk.CTkFrame(pad, fg_color="transparent")
        hdr_frame.pack(fill="x", pady=(0, 10))

        ctk.CTkLabel(
            hdr_frame, text="🪄 PROMPT ARCHITECT (ROLE - CONTEXT - ACTION - OUTPUT)",
            font=ctk.CTkFont(size=14, weight="bold"), text_color=("#7c3aed", "#a855f7")
        ).pack(side="left")

        # Preset Selector Row
        preset_row = ctk.CTkFrame(pad, fg_color="transparent")
        preset_row.pack(fill="x", pady=(0, 10))

        ctk.CTkLabel(preset_row, text="⚡ Presets Mẫu:", font=ctk.CTkFont(size=11, weight="bold"), text_color=("gray30", "gray70")).pack(side="left", padx=(0, 6))

        # 1. Role
        ctk.CTkLabel(pad, text="1. 🎭 ROLE / PERSONA (VAI TRÒ CHUYÊN GIA):", font=ctk.CTkFont(size=11, weight="bold"), text_color=("gray30", "gray70")).pack(anchor="w", pady=(0, 2))
        role_entry = ctk.CTkEntry(pad, placeholder_text="Ví dụ: Staff Security Engineer & SAST Specialist", font=ctk.CTkFont(size=12), height=32)
        role_entry.pack(fill="x", pady=(0, 8))
        role_entry.insert(0, "Staff Software Engineer & Full-Stack Architect")

        # 2. Context
        ctk.CTkLabel(pad, text="2. 📁 WORKSPACE CONTEXT (BỐI CẢNH DỰ ÁN & CÔNG NGHỆ):", font=ctk.CTkFont(size=11, weight="bold"), text_color=("gray30", "gray70")).pack(anchor="w", pady=(0, 2))
        ctx_entry = ctk.CTkEntry(pad, placeholder_text="Ví dụ: FastAPI REST API, Python 3.12, SQLite, PyTest", font=ctk.CTkFont(size=12), height=32)
        ctx_entry.pack(fill="x", pady=(0, 8))
        ctx_entry.insert(0, "Python 3.12, CustomTkinter, SQLite, Claude CLI Async Workers")

        # 3. Action
        ctk.CTkLabel(pad, text="3. ⚡ TECHNICAL ACTION (HÀNH ĐỘNG & QUY CHUẨN KỸ THUẬT):", font=ctk.CTkFont(size=11, weight="bold"), text_color=("gray30", "gray70")).pack(anchor="w", pady=(0, 2))
        act_txt = ctk.CTkTextbox(pad, height=80, font=ctk.CTkFont(size=11), corner_radius=6)
        act_txt.pack(fill="x", pady=(0, 8))
        act_txt.insert("1.0", "Xây dựng module xử lý authentication JWT, validate mật khẩu mã hóa bcrypt và xử lý ngoại lệ chuẩn REST HTTP status codes.")

        # 4. Output Format
        ctk.CTkLabel(pad, text="4. 📄 OUTPUT FORMAT (ĐỊNH DẠNG ĐẦU RA YÊU CẦU):", font=ctk.CTkFont(size=11, weight="bold"), text_color=("gray30", "gray70")).pack(anchor="w", pady=(0, 2))
        out_entry = ctk.CTkEntry(pad, placeholder_text="Ví dụ: Trả về code sản phẩm hoàn chỉnh 100%, type-safe, không có TODO placeholder", font=ctk.CTkFont(size=12), height=32)
        out_entry.pack(fill="x", pady=(0, 14))
        out_entry.insert(0, "Trả về code Python hoàn chỉnh 100%, type-safe, runnable, kèm docstrings chi tiết.")

        def _apply_preset(p_type: str):
            role_entry.delete(0, "end")
            ctx_entry.delete(0, "end")
            act_txt.delete("1.0", "end")
            out_entry.delete(0, "end")

            if p_type == "security":
                role_entry.insert(0, "Staff Security Auditor & Penetration Tester")
                ctx_entry.insert(0, "Enterprise Web Application & REST API Endpoints")
                act_txt.insert("1.0", "Kiểm thử lỗ hổng OWASP Top 10 (SQLi, XSS, CSRF, RCE, Auth bypass). Phân tích mã nguồn và đưa ra bản vá bảo mật chi tiết.")
                out_entry.insert(0, "Báo cáo lỗ hổng xếp hạng CVSS v3.1 kèm code refactored khắc phục 100%.")
            elif p_type == "fastapi":
                role_entry.insert(0, "Senior Backend Engineer (Python & FastAPI Specialist)")
                ctx_entry.insert(0, "Python 3.12, FastAPI, Pydantic v2, SQLAlchemy 2.0, PostgreSQL")
                act_txt.insert("1.0", "Viết REST API Endpoint hoàn chỉnh: Pydantic schemas validation, async database query, error handling và HTTP status codes.")
                out_entry.insert(0, "Code Python hoàn chỉnh 100%, type-safe, runnable, không dùng placeholder.")
            elif p_type == "pytest":
                role_entry.insert(0, "Lead QA & Test Automation Engineer")
                ctx_entry.insert(0, "PyTest, PyTest-Asyncio, Mocking, Coverage Report")
                act_txt.insert("1.0", "Viết bộ kiểm thử unit & integration test phủ 100% happy paths, boundary cases, và mock external service calls.")
                out_entry.insert(0, "File code PyTest hoàn chỉnh chạy được ngay với lệnh pytest -v.")
            elif p_type == "arch":
                role_entry.insert(0, "Chief Technology Officer & Solution Architect")
                ctx_entry.insert(0, "Distributed Systems, Microservices, Event-Driven Architecture")
                act_txt.insert("1.0", "Thiết kế kiến trúc hệ thống, phân rã microservices, thiết kế CSDL và Mermaid sequence diagram.")
                out_entry.insert(0, "Tài liệu Markdown thiết kế hệ thống chi tiết kèm Mermaid diagrams.")

        for p_key, p_label in [("security", "🛡 Security SAST"), ("fastapi", "💻 FastAPI Endpoint"), ("pytest", "🧪 PyTest Suite"), ("arch", "🗺 System Architecture")]:
            ctk.CTkButton(
                preset_row, text=p_label, height=26, font=ctk.CTkFont(size=10, weight="bold"),
                fg_color=("gray85", "#1e293b"), text_color=("gray10", "gray90"),
                hover_color=("#7c3aed", "#7c3aed"), corner_radius=6,
                command=lambda k=p_key: _apply_preset(k)
            ).pack(side="left", padx=2)

        def _generate():
            r = role_entry.get().strip()
            c = ctx_entry.get().strip()
            a = act_txt.get("1.0", "end").strip()
            o = out_entry.get().strip()

            final_prompt = f"""[ROLE]: {r}
[CONTEXT]: {c}
[ACTION]:
{a}
[OUTPUT FORMAT]: {o}"""

            win.destroy()
            self.run_prompt.delete("1.0", "end")
            self.run_prompt.insert("1.0", final_prompt)
            self._global_log("Đã tạo Prompt chuẩn Enterprise bằng Prompt Architect!", "SUCCESS")

        btn_r = ctk.CTkFrame(pad, fg_color="transparent")
        btn_r.pack(fill="x")
        ctk.CTkButton(
            btn_r, text="⚡ Sinh Prompt Chuẩn Enterprise", height=38,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("#7c3aed", "#8b5cf6"), hover_color=("#6d28d9", "#7c3aed"),
            corner_radius=8, command=_generate
        ).pack(side="left", expand=True, fill="x", padx=(0, 6))

        ctk.CTkButton(
            btn_r, text="Hủy", height=38, width=80,
            font=ctk.CTkFont(size=12),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=8,
            command=win.destroy
        ).pack(side="left")

    def _open_floating_prompt(self):
        if not self.winfo_viewable():
            self.deiconify()
        def _on_submit(prompt_text: str):
            self._global_log(f"Quick Floating Prompt: {prompt_text}", "SEND")
            self.cli.run_once(prompt_text, on_log=self._global_log)
        FloatingQuickWidget(self, on_submit=_on_submit)
        self.log_text.configure(
            bg="#0f172a" if current_theme == "dark" else "#ffffff",
            fg="#f8fafc" if current_theme == "dark" else "#0f172a"
        )
        self._refresh_tasks()


if __name__ == "__main__":
    app = ClaudeSuiteApp()
    app.mainloop()
