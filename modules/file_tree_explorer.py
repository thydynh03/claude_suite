"""
file_tree_explorer.py — Giao diện Cây Thư mục & Files (Tree Explorer) cho dự án.
Hiển thị cấu trúc cây thư mục + file preview + chọn file làm bối cảnh AI.
"""

import os
import tkinter as tk
from tkinter import ttk
import customtkinter as ctk
from typing import List, Callable
from modules.context_manager import ContextManager, IGNORE_DIRS


class FileTreeExplorerModal(ctk.CTkToplevel):
    def __init__(self, parent, workspace_folder: str, current_attached_files: List[str], on_apply: Callable[[List[str]], None]):
        super().__init__(parent)
        self.workspace_folder = os.path.abspath(workspace_folder) if workspace_folder and os.path.exists(workspace_folder) else ""
        self.attached_files = set(os.path.abspath(f) for f in current_attached_files)
        self.on_apply = on_apply
        self.title(f"🌳 Project File Tree Explorer: {os.path.basename(self.workspace_folder) or self.workspace_folder}")
        self.geometry("980x660")
        self.minsize(750, 500)

        self._build_ui()
        if self.workspace_folder:
            self._populate_tree()

    def _build_ui(self):
        # Top bar
        top_bar = ctk.CTkFrame(self, height=45, fg_color="transparent")
        top_bar.pack(fill="x", padx=16, pady=(12, 8))

        ctk.CTkLabel(
            top_bar, text=f"📁 WORKSPACE: {self.workspace_folder or 'Chưa chọn thư mục'}",
            font=ctk.CTkFont(size=13, weight="bold")
        ).pack(side="left")

        # Search bar
        self.search_var = ctk.StringVar()
        self.search_var.trace_add("write", lambda *_: self._filter_tree())
        search_entry = ctk.CTkEntry(
            top_bar, textvariable=self.search_var, placeholder_text="🔍 Tìm kiếm file...",
            width=240, height=32, font=ctk.CTkFont(size=12)
        )
        search_entry.pack(side="right")

        # Main Paned Frame (Tree on left, Preview on right)
        main_pane = ctk.CTkFrame(self, fg_color="transparent")
        main_pane.pack(fill="both", expand=True, padx=16, pady=4)

        # Left Frame: Tree
        tree_frame = ctk.CTkFrame(main_pane, fg_color=("gray90", "#1e293b"), corner_radius=10)
        tree_frame.pack(side="left", fill="both", expand=True, padx=(0, 6))

        # Styled ttk.Treeview
        tree_scroll_y = ttk.Scrollbar(tree_frame, orient="vertical")
        tree_scroll_x = ttk.Scrollbar(tree_frame, orient="horizontal")

        style = ttk.Style()
        style.theme_use("clam")
        style.configure("Treeview",
                        background="#0f172a", foreground="#f8fafc",
                        fieldbackground="#0f172a", rowheight=24,
                        font=("Consolas", 10))
        style.map("Treeview", background=[("selected", "#0284c7")], foreground=[("selected", "#ffffff")])

        self.tree = ttk.Treeview(
            tree_frame,
            yscrollcommand=tree_scroll_y.set,
            xscrollcommand=tree_scroll_x.set,
            selectmode="browse"
        )
        tree_scroll_y.config(command=self.tree.yview)
        tree_scroll_x.config(command=self.tree.xview)

        tree_scroll_y.pack(side="right", fill="y")
        tree_scroll_x.pack(side="bottom", fill="x")
        self.tree.pack(fill="both", expand=True, padx=6, pady=6)

        self.tree.heading("#0", text="  📁 Cấu Trúc Thư Mục & File (Double click chọn file)", anchor="w")
        self.tree.bind("<<TreeviewSelect>>", self._on_tree_select)
        self.tree.bind("<Double-1>", self._on_tree_double_click)

        # Right Frame: File Content Preview
        preview_frame = ctk.CTkFrame(main_pane, fg_color=("gray90", "#1e293b"), corner_radius=10, width=440)
        preview_frame.pack(side="right", fill="both", expand=True, padx=(6, 0))

        self.lbl_preview_title = ctk.CTkLabel(
            preview_frame, text="📄 Xem trước nội dung File",
            font=ctk.CTkFont(size=12, weight="bold"), anchor="w"
        )
        self.lbl_preview_title.pack(fill="x", padx=12, pady=(10, 4))

        self.preview_text = ctk.CTkTextbox(
            preview_frame, font=ctk.CTkFont(family="Consolas", size=11),
            wrap="none", corner_radius=8
        )
        self.preview_text.pack(fill="both", expand=True, padx=10, pady=(0, 10))

        # Bottom Bar
        bottom_bar = ctk.CTkFrame(self, height=50, fg_color="transparent")
        bottom_bar.pack(fill="x", padx=16, pady=12)

        self.lbl_status = ctk.CTkLabel(
            bottom_bar, text=f"📊 Đã chọn {len(self.attached_files)} files bối cảnh AI",
            font=ctk.CTkFont(size=12, weight="bold"), text_color="#38bdf8"
        )
        self.lbl_status.pack(side="left")

        ctk.CTkButton(
            bottom_bar, text="✅ Áp Dụng Bối Cảnh AI", height=36,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("#0284c7", "#0284c7"), hover_color=("#0369a1", "#0369a1"),
            corner_radius=8, command=self._apply_and_close
        ).pack(side="right", padx=(8, 0))

        ctk.CTkButton(
            bottom_bar, text="➕ Chọn TẤT CẢ Files Code", height=36,
            font=ctk.CTkFont(size=11),
            fg_color=("gray80", "#334155"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "#475569"), corner_radius=8,
            command=self._select_all_text_files
        ).pack(side="right", padx=4)

        ctk.CTkButton(
            bottom_bar, text="🗑 Xóa Chọn All", height=36,
            font=ctk.CTkFont(size=11),
            fg_color=("gray80", "#334155"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "#475569"), corner_radius=8,
            command=self._clear_selection
        ).pack(side="right", padx=4)

    def _get_icon(self, path: str, is_dir: bool) -> str:
        if is_dir:
            return "📁 "
        ext = os.path.splitext(path)[1].lower()
        if ext in {".py", ".pyw"}: return "🐍 "
        if ext in {".js", ".ts", ".jsx", ".tsx"}: return "⚡ "
        if ext in {".json", ".yaml", ".yml", ".toml", ".ini"}: return "⚙️ "
        if ext in {".html", ".css", ".scss"}: return "🎨 "
        if ext in {".md", ".txt"}: return "📝 "
        if ext in {".sql", ".db", ".sqlite"}: return "🗄️ "
        return "📄 "

    def _populate_tree(self):
        self.tree.delete(*self.tree.get_children())
        self.nodes_map = {} # node_id -> full_path

        root_node = self.tree.insert("", "end", text=f" 📁 {os.path.basename(self.workspace_folder)}", open=True)
        self.nodes_map[root_node] = self.workspace_folder

        self._add_folder_nodes(root_node, self.workspace_folder)

    def _add_folder_nodes(self, parent_node: str, folder_path: str):
        try:
            entries = sorted(os.listdir(folder_path), key=lambda x: (not os.path.isdir(os.path.join(folder_path, x)), x.lower()))
        except Exception:
            return

        for entry in entries:
            if entry in IGNORE_DIRS or entry.startswith("."):
                continue

            full_path = os.path.join(folder_path, entry)
            is_dir = os.path.isdir(full_path)
            icon = self._get_icon(full_path, is_dir)
            if is_dir:
                display_name = f"{icon}{entry}"
            else:
                is_checked = "[x]" if full_path in self.attached_files else "[ ]"
                display_name = f"{is_checked} {icon}{entry}"

            node_id = self.tree.insert(parent_node, "end", text=display_name, open=False)
            self.nodes_map[node_id] = full_path

            if is_dir:
                self._add_folder_nodes(node_id, full_path)

    def _on_tree_select(self, event):
        sel = self.tree.selection()
        if not sel:
            return
        node_id = sel[0]
        full_path = self.nodes_map.get(node_id, "")
        if full_path and os.path.isfile(full_path):
            self.lbl_preview_title.configure(text=f"📄 {os.path.basename(full_path)} ({os.path.getsize(full_path)} bytes)")
            content = ContextManager.read_file_content(full_path, max_size_kb=300)
            self.preview_text.delete("1.0", "end")
            self.preview_text.insert("1.0", content)

    def _on_tree_double_click(self, event):
        sel = self.tree.selection()
        if not sel:
            return
        node_id = sel[0]
        full_path = self.nodes_map.get(node_id, "")
        if full_path and os.path.isfile(full_path):
            if full_path in self.attached_files:
                self.attached_files.remove(full_path)
            else:
                self.attached_files.add(full_path)
            self._update_node_display(node_id, full_path)
            self.lbl_status.configure(text=f"📊 Đã chọn {len(self.attached_files)} files bối cảnh AI")

    def _update_node_display(self, node_id: str, full_path: str):
        is_dir = os.path.isdir(full_path)
        icon = self._get_icon(full_path, is_dir)
        entry = os.path.basename(full_path)
        if is_dir:
            self.tree.item(node_id, text=f"{icon}{entry}")
        else:
            is_checked = "[x]" if full_path in self.attached_files else "[ ]"
            self.tree.item(node_id, text=f"{is_checked} {icon}{entry}")

    def _filter_tree(self):
        query = self.search_var.get().strip().lower()
        if not query:
            return
        for node_id, full_path in self.nodes_map.items():
            if query in os.path.basename(full_path).lower():
                self.tree.see(node_id)
                self.tree.selection_set(node_id)

    def _select_all_text_files(self):
        if not self.workspace_folder:
            return
        scanned = ContextManager.scan_folder(self.workspace_folder, max_files=100)
        for f in scanned:
            self.attached_files.add(os.path.abspath(f))
        self._populate_tree()
        self.lbl_status.configure(text=f"📊 Đã chọn {len(self.attached_files)} files bối cảnh AI")

    def _clear_selection(self):
        self.attached_files.clear()
        self._populate_tree()
        self.lbl_status.configure(text="📊 Đã chọn 0 files bối cảnh AI")

    def _apply_and_close(self):
        self.on_apply(list(self.attached_files))
        self.destroy()
