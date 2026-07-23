"""
diff_viewer.py — Trình hiển thị Code Diff & Subtask Checklist cho Claude Suite
"""

import difflib
import customtkinter as ctk
import tkinter as tk
from typing import List, Dict


class CodeDiffViewer(ctk.CTkFrame):
    """
    Component hiển thị so sánh Code Diff (Before vs After) với định dạng màu sắc trực quan.
    """

    def __init__(self, parent, old_code: str = "", new_code: str = ""):
        super().__init__(parent, corner_radius=8, fg_color=("gray95", "#0f172a"))
        
        self.text_area = ctk.CTkTextbox(self, font=ctk.CTkFont(family="Consolas", size=11), corner_radius=8)
        self.text_area.pack(fill="both", expand=True, padx=8, pady=8)
        
        self.set_diff(old_code, new_code)

    def set_diff(self, old_code: str, new_code: str):
        self.text_area.configure(state="normal")
        self.text_area.delete("1.0", "end")

        old_lines = old_code.splitlines()
        new_lines = new_code.splitlines()

        diff = list(difflib.unified_diff(old_lines, new_lines, fromfile="Original", tofile="Modified", lineterm=""))

        if not diff:
            self.text_area.insert("1.0", "(Không có thay đổi code nào được phát hiện)")
        else:
            for line in diff:
                if line.startswith("+") and not line.startswith("+++"):
                    self.text_area.insert("end", line + "\n", "ADD")
                elif line.startswith("-") and not line.startswith("---"):
                    self.text_area.insert("end", line + "\n", "DEL")
                elif line.startswith("@@"):
                    self.text_area.insert("end", line + "\n", "HEADER")
                else:
                    self.text_area.insert("end", line + "\n", "NORM")

        # Configure diff colors
        self.text_area.tag_config("ADD", foreground="#22c55e", background="#052e16")
        self.text_area.tag_config("DEL", foreground="#ef4444", background="#450a0a")
        self.text_area.tag_config("HEADER", foreground="#38bdf8")
        self.text_area.configure(state="disabled")
