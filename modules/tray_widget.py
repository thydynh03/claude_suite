"""
tray_widget.py — System Tray Integration & Floating Quick Prompt Window
Cho phép thu nhỏ app xuống System Tray và hiển thị Floating Widget phục vụ nhập prompt nhanh.
"""

import customtkinter as ctk
import tkinter as tk
from typing import Callable, Optional


class FloatingQuickWidget(ctk.CTkToplevel):
    """
    Cửa sổ Quick Prompt nổi (Floating Widget) mở bằng phím tắt hoặc Tray Menu.
    """

    def __init__(self, parent, on_submit: Callable[[str], None]):
        super().__init__(parent)
        self.on_submit = on_submit

        self.title("⚡ Claude Quick Prompt")
        self.geometry("520x180+400+250")
        self.overrideredirect(False)
        self.attributes("-topmost", True)
        self.grab_set()

        pad = ctk.CTkFrame(self, fg_color="transparent")
        pad.pack(fill="both", expand=True, padx=16, pady=12)

        hdr = ctk.CTkFrame(pad, fg_color="transparent")
        hdr.pack(fill="x", pady=(0, 6))

        ctk.CTkLabel(
            hdr, text="⚡  Claude Quick Prompt (Global Shortcut)",
            font=ctk.CTkFont(size=13, weight="bold"),
            text_color=("#0284c7", "#38bdf8")
        ).pack(side="left")

        self.entry_prompt = ctk.CTkEntry(
            pad, placeholder_text="Nhập yêu cầu nhanh cho Claude (Nhấn Enter để gửi)...",
            font=ctk.CTkFont(size=12), height=36
        )
        self.entry_prompt.pack(fill="x", pady=(0, 10))
        self.entry_prompt.bind("<Return>", lambda e: self._submit())
        self.entry_prompt.focus()

        btn_row = ctk.CTkFrame(pad, fg_color="transparent")
        btn_row.pack(fill="x")

        ctk.CTkButton(
            btn_row, text="▶  Gửi Ngay", height=32,
            font=ctk.CTkFont(size=11, weight="bold"),
            fg_color=("#0284c7", "#0284c7"), hover_color=("#0369a1", "#0369a1"),
            corner_radius=6, command=self._submit
        ).pack(side="left", expand=True, fill="x", padx=(0, 6))

        ctk.CTkButton(
            btn_row, text="Đóng", height=32, width=70,
            font=ctk.CTkFont(size=11),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=6,
            command=self.destroy
        ).pack(side="left")

    def _submit(self):
        text = self.entry_prompt.get().strip()
        if text:
            self.on_submit(text)
            self.destroy()
