import customtkinter as ctk
import tkinter as tk
from tkinter import filedialog, messagebox
import os

from modules.document_parser import DocumentParser

class CockpitTabFrame(ctk.CTkFrame):
    """
    Không gian làm việc chính (🚀 AI Cockpit).
    Tích hợp luồng 3 bước:
    Step 1: Context & File Attachments
    Step 2: Prompt Input
    Step 3: Live Progress (Split Pane: Agent status & Human/Coder View)
    """
    def __init__(self, master, app, **kwargs):
        super().__init__(master, fg_color="transparent", **kwargs)
        self.app = app
        self.attached_files = {} # dict filepath -> content
        self.view_mode = "coder" # "coder" or "human"
        
        self._build_ui()

    def _build_ui(self):
        # --- Banners ---
        banner = ctk.CTkFrame(self, fg_color=("#0284c7", "#0369a1"), corner_radius=8, height=40)
        banner.pack(fill="x", padx=10, pady=(5, 10))
        banner.pack_propagate(False)
        ctk.CTkLabel(
            banner, text="🚀 AI Cockpit — Không gian điều khiển tự động", 
            font=ctk.CTkFont(size=14, weight="bold"), text_color="white"
        ).pack(side="left", padx=15, pady=5)

        # ── Step 1: Project & Document Context ──
        step1_frame = ctk.CTkFrame(self, fg_color=("gray90", "#1e293b"), corner_radius=8)
        step1_frame.pack(fill="x", padx=10, pady=5)
        
        step1_header = ctk.CTkFrame(step1_frame, fg_color="transparent")
        step1_header.pack(fill="x", padx=10, pady=(10, 5))
        ctk.CTkLabel(step1_header, text="Step 1: Đính kèm tài liệu (Context)", font=ctk.CTkFont(weight="bold")).pack(side="left")
        
        self.lbl_token_indicator = ctk.CTkLabel(step1_header, text="Context size: 0 files", font=ctk.CTkFont(size=11), text_color="gray50")
        self.lbl_token_indicator.pack(side="right")

        btn_frame = ctk.CTkFrame(step1_frame, fg_color="transparent")
        btn_frame.pack(fill="x", padx=10, pady=(0, 10))
        
        ctk.CTkButton(
            btn_frame, text="🌳 Chọn Files từ Project Tree", width=200, height=28,
            command=self._attach_folder, fg_color=("gray70", "#334155")
        ).pack(side="left", padx=(0, 5))
        
        ctk.CTkButton(
            btn_frame, text="📄 Đính kèm Files", width=120, height=28,
            command=self._attach_files, fg_color=("gray70", "#334155")
        ).pack(side="left")

        # Khung chứa chips các file đính kèm
        self.chips_frame = ctk.CTkScrollableFrame(step1_frame, height=40, fg_color="transparent", orientation="horizontal")
        self.chips_frame.pack(fill="x", padx=10, pady=(0, 10))

        # ── Step 2: Prompt Input ──
        step2_frame = ctk.CTkFrame(self, fg_color=("gray90", "#1e293b"), corner_radius=8)
        step2_frame.pack(fill="x", padx=10, pady=5)
        
        ctk.CTkLabel(step2_frame, text="Step 2: Nhập Yêu Cầu (Prompt)", font=ctk.CTkFont(weight="bold")).pack(anchor="w", padx=10, pady=(10, 5))
        
        self.txt_prompt = ctk.CTkTextbox(step2_frame, height=100, font=ctk.CTkFont(size=13))
        self.txt_prompt.pack(fill="x", padx=10, pady=(0, 5))
        self.txt_prompt.insert("1.0", "Ví dụ: Xây dựng form đăng nhập theo giao diện đính kèm...")
        
        # Prompt Pills
        pills_frame = ctk.CTkFrame(step2_frame, fg_color="transparent")
        pills_frame.pack(fill="x", padx=10, pady=(0, 10))
        
        def _fill_prompt(template_type):
            self.txt_prompt.delete("1.0", "end")
            if template_type == "bug":
                self.txt_prompt.insert("1.0", "Tôi muốn tìm & sửa lỗi trong dự án.\n- Lỗi hiện tại là:\n- File cần kiểm tra:\n- Hành vi mong muốn là:")
            elif template_type == "feature":
                self.txt_prompt.insert("1.0", "Tôi muốn thêm tính năng mới.\n- Tính năng:\n- File cần sửa/tạo:\n- Logic đặc biệt:")
            elif template_type == "refactor":
                self.txt_prompt.insert("1.0", "Tôi muốn tối ưu hóa mã nguồn.\n- File cần tối ưu:\n- Mục tiêu (vd: Clean code, tăng tốc độ):")
            elif template_type == "docs":
                self.txt_prompt.insert("1.0", "Tôi muốn viết tài liệu/comment cho code.\n- File cần viết:\n- Chuẩn format:")
                
        ctk.CTkButton(pills_frame, text="🐛 Sửa lỗi", width=80, height=24, font=ctk.CTkFont(size=11), corner_radius=12, fg_color=("#f59e0b", "#d97706"), hover_color=("#d97706", "#b45309"), command=lambda: _fill_prompt("bug")).pack(side="left", padx=(0, 5))
        ctk.CTkButton(pills_frame, text="✨ Thêm tính năng", width=120, height=24, font=ctk.CTkFont(size=11), corner_radius=12, fg_color=("#3b82f6", "#2563eb"), hover_color=("#2563eb", "#1d4ed8"), command=lambda: _fill_prompt("feature")).pack(side="left", padx=5)
        ctk.CTkButton(pills_frame, text="♻️ Tối ưu", width=80, height=24, font=ctk.CTkFont(size=11), corner_radius=12, fg_color=("#10b981", "#059669"), hover_color=("#059669", "#047857"), command=lambda: _fill_prompt("refactor")).pack(side="left", padx=5)
        ctk.CTkButton(pills_frame, text="📝 Viết Docs", width=90, height=24, font=ctk.CTkFont(size=11), corner_radius=12, fg_color=("#8b5cf6", "#7c3aed"), hover_color=("#7c3aed", "#6d28d9"), command=lambda: _fill_prompt("docs")).pack(side="left", padx=5)
        
        actions_frame = ctk.CTkFrame(step2_frame, fg_color="transparent")
        actions_frame.pack(fill="x", padx=10, pady=(0, 10))
        
        self.btn_run_swarm = ctk.CTkButton(
            actions_frame, text="⚡ Chạy Tự Động", width=150, height=36,
            fg_color=("#16a34a", "#15803d"), hover_color=("#15803d", "#166534"),
            font=ctk.CTkFont(weight="bold"), command=self._run_swarm
        )
        self.btn_run_swarm.pack(side="right", padx=(5, 0))
        
        self.btn_plan = ctk.CTkButton(
            actions_frame, text="📐 Phân Rã Kế Hoạch", width=150, height=36,
            fg_color=("#7c3aed", "#6d28d9"), hover_color=("#6d28d9", "#5b21b6"),
            font=ctk.CTkFont(weight="bold"), command=self._plan_builder
        )
        self.btn_plan.pack(side="right")

        # ── Step 3: Live Progress (Split Pane) ──
        step3_frame = ctk.CTkFrame(self, fg_color="transparent")
        step3_frame.pack(fill="both", expand=True, padx=10, pady=5)
        
        step3_header = ctk.CTkFrame(step3_frame, fg_color="transparent")
        step3_header.pack(fill="x", pady=(0, 5))
        ctk.CTkLabel(step3_header, text="Step 3: Live Progress", font=ctk.CTkFont(weight="bold")).pack(side="left")
        
        # Toggle Code vs Human
        self.view_toggle = ctk.CTkSegmentedButton(
            step3_header, values=["</> Góc nhìn Coder", "👁️ Góc nhìn Quản lý"],
            command=self._on_view_toggle
        )
        self.view_toggle.set("</> Góc nhìn Coder")
        self.view_toggle.pack(side="right")
        
        self.btn_stop = ctk.CTkButton(
            step3_header, text="🛑 Dừng Khẩn Cấp", width=120, height=28,
            fg_color=("#dc2626", "#b91c1c"), hover_color=("#b91c1c", "#991b1b"),
            text_color="white", command=self._stop_execution
        )
        self.btn_stop.pack(side="right", padx=10)
        self.btn_stop.configure(state="disabled")

        # Split content
        split_frame = ctk.CTkFrame(step3_frame, fg_color="transparent")
        split_frame.pack(fill="both", expand=True)
        
        # Left: Agent status
        self.agent_status_frame = ctk.CTkFrame(split_frame, width=200, fg_color=("gray90", "#1e293b"))
        self.agent_status_frame.pack(side="left", fill="y", padx=(0, 5))
        self.agent_status_frame.pack_propagate(False)
        ctk.CTkLabel(self.agent_status_frame, text="Agent Status", font=ctk.CTkFont(weight="bold")).pack(pady=10)
        
        # Right: Log / Timeline / Webview
        self.output_frame = ctk.CTkFrame(split_frame, fg_color=("gray90", "#1e293b"))
        self.output_frame.pack(side="left", fill="both", expand=True)
        
        # Code View TextBox
        self.txt_coder_log = ctk.CTkTextbox(self.output_frame, font=ctk.CTkFont(family="Consolas", size=12))
        self.txt_coder_log.pack(fill="both", expand=True, padx=5, pady=5)
        
        # Human View TextBox (Timeline)
        self.txt_human_log = ctk.CTkTextbox(self.output_frame, font=ctk.CTkFont(size=14))
        # Hide by default
        
    def _on_view_toggle(self, value):
        if "Coder" in value:
            self.view_mode = "coder"
            self.txt_human_log.pack_forget()
            self.txt_coder_log.pack(fill="both", expand=True, padx=5, pady=5)
        else:
            self.view_mode = "human"
            self.txt_coder_log.pack_forget()
            self.txt_human_log.pack(fill="both", expand=True, padx=5, pady=5)

    def _attach_folder(self):
        folder = filedialog.askdirectory(title="Chọn Folder Dự Án")
        if folder:
            # We import FileTreeExplorerModal locally to avoid circular imports if any
            from modules.file_tree_explorer import FileTreeExplorerModal
            
            def _on_files_selected(selected_paths):
                # Clear and replace current attached files with selected ones
                self.attached_files.clear()
                for path in selected_paths:
                    if os.path.exists(path):
                        self.attached_files[path] = DocumentParser.parse_file(path)
                self._update_chips()
                
            current_selected = list(self.attached_files.keys())
            modal = FileTreeExplorerModal(self.winfo_toplevel(), folder, current_selected, _on_files_selected)
            modal.grab_set()

    def _attach_files(self):
        files = filedialog.askopenfilenames(title="Chọn Files đính kèm", filetypes=[("All Files", "*.*")])
        if files:
            for f in files:
                self.attached_files[f] = DocumentParser.parse_file(f)
            self._update_chips()

    def _update_chips(self):
        for w in self.chips_frame.winfo_children():
            w.destroy()
            
        for filepath in self.attached_files.keys():
            chip = ctk.CTkFrame(self.chips_frame, corner_radius=12, fg_color=("#38bdf8", "#0284c7"))
            chip.pack(side="left", padx=2, pady=2)
            name = os.path.basename(filepath)
            ctk.CTkLabel(chip, text=name, text_color="white", font=ctk.CTkFont(size=11)).pack(side="left", padx=(8, 2))
            btn = ctk.CTkButton(
                chip, text="✕", width=20, height=20, corner_radius=10, 
                fg_color="transparent", hover_color=("#0ea5e9", "#0369a1"), text_color="white",
                command=lambda f=filepath: self._remove_chip(f)
            )
            btn.pack(side="left", padx=(0, 2))
            
        self.lbl_token_indicator.configure(text=f"Context: {len(self.attached_files)} files")
        
    def _remove_chip(self, filepath):
        if filepath in self.attached_files:
            del self.attached_files[filepath]
        self._update_chips()

    def _run_swarm(self):
        prompt = self.txt_prompt.get("1.0", "end-1c").strip()
        if not prompt:
            messagebox.showwarning("Thiếu thông tin", "Vui lòng nhập Yêu Cầu (Prompt) ở Step 2!")
            return
            
        # Update context in main app
        for path in self.attached_files.keys():
            if path not in self.app._global_workspace_files:
                self.app._global_workspace_files.append(path)
                
        # Start orchestrator
        self.btn_stop.configure(state="normal")
        self.btn_run_swarm.configure(state="disabled")
        
        # Add a task to the board via app logic
        from modules.models import Task
        import uuid
        task = Task(task_id=str(uuid.uuid4())[:8], title=prompt[:40], prompt=prompt, status="queued")
        self.app.board.add(task)
        self.app.orchestr.start()
        
    def _plan_builder(self):
        # Switch to Task Board tab and select Plan Builder
        self.app.tabview.set("📋 Task Board & Planning")
        self.app.board_frame.internal_tab.set("AI Plan Builder")
        
    def _stop_execution(self):
        if messagebox.askyesno("Xác nhận", "Bạn có chắc chắn muốn ngắt quá trình chạy của AI?"):
            self.app.orchestr.stop()
            self.btn_stop.configure(state="disabled")
            self.btn_run_swarm.configure(state="normal")
