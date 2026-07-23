#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Claude Code Prompt Scheduler v3.0
He gio tu dong gui prompt vao Claude Code
"""

import sys, io
if sys.stdout is not None:
    sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8', errors='replace')
if sys.stderr is not None:
    sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding='utf-8', errors='replace')

import tkinter as tk
from tkinter import scrolledtext, messagebox
import threading
import datetime
import time
import subprocess
import ctypes

user32   = ctypes.windll.user32
kernel32 = ctypes.windll.kernel32

def install_deps():
    try:
        import win32gui
    except ImportError:
        subprocess.check_call(
            [sys.executable, "-m", "pip", "install", "pywin32", "-q"],
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

install_deps()
import win32gui, win32con, win32api

# ── Theme ─────────────────────────────────────────────────────────────────────

THEMES = {
    "dark": {
        "bg":         "#0d1117",
        "surface":    "#161b22",
        "surface2":   "#21262d",
        "border":     "#30363d",
        "accent":     "#f78166",
        "accent_btn": "#da3633",
        "accent2":    "#79c0ff",
        "text":       "#e6edf3",
        "text_muted": "#8b949e",
        "success":    "#3fb950",
        "warning":    "#e3b341",
        "error":      "#f85149",
        "input_bg":   "#010409",
    },
    "light": {
        "bg":         "#f0f2f5",
        "surface":    "#ffffff",
        "surface2":   "#eaeef2",
        "border":     "#d0d7de",
        "accent":     "#cf222e",
        "accent_btn": "#cf222e",
        "accent2":    "#0969da",
        "text":       "#1f2328",
        "text_muted": "#57606a",
        "success":    "#1a7f37",
        "warning":    "#9a6700",
        "error":      "#cf222e",
        "input_bg":   "#ffffff",
    }
}
current_theme = "dark"

def T():
    return THEMES[current_theme]

# ── Automation ────────────────────────────────────────────────────────────────

def find_claude_windows():
    found = []
    def cb(hwnd, _):
        if win32gui.IsWindowVisible(hwnd):
            t = win32gui.GetWindowText(hwnd).lower()
            if any(k in t for k in ["claude", "anthropic"]) \
               and "scheduler" not in t and "prompt scheduler" not in t:
                found.append((hwnd, win32gui.GetWindowText(hwnd)))
    win32gui.EnumWindows(cb, None)
    return found


def set_clipboard(text):
    import win32clipboard
    win32clipboard.OpenClipboard()
    win32clipboard.EmptyClipboard()
    win32clipboard.SetClipboardText(text, win32clipboard.CF_UNICODETEXT)
    win32clipboard.CloseClipboard()


def click_pos(x, y):
    win32api.SetCursorPos((x, y))
    time.sleep(0.2)
    win32api.mouse_event(win32con.MOUSEEVENTF_LEFTDOWN, 0, 0, 0, 0)
    time.sleep(0.08)
    win32api.mouse_event(win32con.MOUSEEVENTF_LEFTUP, 0, 0, 0, 0)
    time.sleep(0.4)


def paste_and_send(text, press_enter=True):
    set_clipboard(text)
    time.sleep(0.25)
    win32api.keybd_event(win32con.VK_CONTROL, 0, 0, 0)
    win32api.keybd_event(ord('V'), 0, 0, 0)
    time.sleep(0.12)
    win32api.keybd_event(ord('V'), 0, win32con.KEYEVENTF_KEYUP, 0)
    win32api.keybd_event(win32con.VK_CONTROL, 0, win32con.KEYEVENTF_KEYUP, 0)
    time.sleep(0.3)
    if press_enter:
        win32api.keybd_event(win32con.VK_RETURN, 0, 0, 0)
        time.sleep(0.08)
        win32api.keybd_event(win32con.VK_RETURN, 0, win32con.KEYEVENTF_KEYUP, 0)


def try_uia_send(hwnd, text, press_enter, log):
    """Tier 1: Windows UI Automation — tim chinh xac Edit control."""
    try:
        import uiautomation as auto
        ctrl = auto.ControlFromHandle(hwnd)
        if not ctrl:
            log("[T1] Khong lay duoc ControlFromHandle", "TIER1")
            return False

        input_ctrl = None
        for depth in [8, 12, 20]:
            try:
                c = ctrl.EditControl(searchDepth=depth)
                if c.Exists(maxSearchSeconds=0.5):
                    input_ctrl = c
                    log(f"[T1] Tim thay EditControl (depth={depth})", "TIER1")
                    break
            except Exception:
                pass

        if not input_ctrl:
            log("[T1] EditControl khong thay, thu DocumentControl...", "TIER1")
            try:
                c = ctrl.DocumentControl(searchDepth=20)
                if c.Exists(maxSearchSeconds=0.5):
                    input_ctrl = c
                    log("[T1] Tim thay DocumentControl", "TIER1")
            except Exception:
                pass

        if not input_ctrl:
            log("[T1] Khong tim thay bat ky input control nao", "TIER1")
            return False

        name = (input_ctrl.Name or "")[:50]
        log(f"[T1] Control: {input_ctrl.ControlTypeName}  Name={name!r}", "TIER1")

        try:
            br = input_ctrl.BoundingRectangle
            log(f"[T1] Vi tri: ({br.left},{br.top}) → ({br.right},{br.bottom})", "TIER1")
            cx = (br.left + br.right) // 2
            cy = (br.top + br.bottom) // 2
            click_pos(cx, cy)
        except Exception:
            input_ctrl.Click()

        time.sleep(0.4)
        win32api.keybd_event(win32con.VK_CONTROL, 0, 0, 0)
        win32api.keybd_event(ord('A'), 0, 0, 0)
        time.sleep(0.05)
        win32api.keybd_event(ord('A'), 0, win32con.KEYEVENTF_KEYUP, 0)
        win32api.keybd_event(win32con.VK_CONTROL, 0, win32con.KEYEVENTF_KEYUP, 0)
        time.sleep(0.15)

        log("[T1] Dang paste prompt...", "TIER1")
        paste_and_send(text, press_enter)
        log("[T1] THANH CONG!", "SUCCESS")
        return True

    except ImportError:
        log("[T1] uiautomation chua cai, bo qua Tier 1", "TIER1")
        return False
    except Exception as e:
        log(f"[T1] Loi: {e}", "ERROR")
        return False


def try_position_send(hwnd, text, press_enter, log):
    """Tier 2: Click offset tu day cua so (khong can toa do tuyet doi)."""
    rect = win32gui.GetWindowRect(hwnd)
    x1, y1, x2, y2 = rect
    cx = (x1 + x2) // 2
    log(f"[T2] Kich thuoc cua so: {x2-x1}x{y2-y1}px, center_x={cx}", "TIER2")

    offsets = [55, 70, 90, 110]
    for offset in offsets:
        cy = y2 - offset
        log(f"[T2] Thu click ({cx}, {cy}) — -{offset}px tu day", "TIER2")
        click_pos(cx, cy)
        win32api.keybd_event(win32con.VK_CONTROL, 0, 0, 0)
        win32api.keybd_event(ord('A'), 0, 0, 0)
        time.sleep(0.05)
        win32api.keybd_event(ord('A'), 0, win32con.KEYEVENTF_KEYUP, 0)
        win32api.keybd_event(win32con.VK_CONTROL, 0, win32con.KEYEVENTF_KEYUP, 0)
        time.sleep(0.15)
        log(f"[T2] Dang paste tai offset -{offset}px...", "TIER2")
        paste_and_send(text, press_enter)
        log(f"[T2] Da gui (offset -{offset}px)", "SUCCESS")
        return True

    return False


def send_prompt_to_claude(prompt, press_enter=True, status_cb=None):
    def log(m, lvl=None):
        if status_cb:
            if lvl:
                status_cb(m, lvl)
            else:
                status_cb(m)

    wins = find_claude_windows()
    if not wins:
        log("Khong tim thay cua so Claude Code!", "ERROR")
        return False

    hwnd, title = wins[0]
    log(f"Cua so: [{hwnd}] {title}", "INFO")

    user32.ShowWindow(hwnd, win32con.SW_RESTORE)
    time.sleep(0.3)
    user32.SetForegroundWindow(hwnd)
    time.sleep(0.7)

    log("--- Tier 1: UI Automation ---", "TIER1")
    if try_uia_send(hwnd, prompt, press_enter, log):
        return True

    log("--- Tier 2: Position offset ---", "TIER2")
    if try_position_send(hwnd, prompt, press_enter, log):
        return True

    log("Gui that bai - khong focus duoc input", "ERROR")
    return False


# ── Main App ──────────────────────────────────────────────────────────────────

class ClaudeScheduler(tk.Tk):
    def __init__(self):
        super().__init__()
        self.title("Claude Prompt Scheduler")
        self.geometry("560x840")
        self.resizable(True, True)
        self.minsize(480, 640)

        self.is_running    = False
        self.timer_thread  = None
        self.countdown_var = tk.StringVar(value="00:00:00")
        self.status_var    = tk.StringVar(value="San sang")
        self.target_str    = tk.StringVar(value="--")
        self.mode_var      = tk.StringVar(value="time")
        self.press_enter   = tk.BooleanVar(value=True)
        self.repeat_var    = tk.BooleanVar(value=False)
        self.hour_var      = tk.StringVar(value="05")
        self.minute_var    = tk.StringVar(value="40")
        self.cd_hour_var   = tk.StringVar(value="0")
        self.cd_min_var    = tk.StringVar(value="30")
        self.cd_sec_var    = tk.StringVar(value="0")
        self.log_count     = 0
        self._cards        = []

        self._build_ui()
        self._apply_theme()
        self._tick()

    # ── Build UI ───────────────────────────────────────────────────────────

    def _build_ui(self):
        # Header
        self.header_frame = tk.Frame(self, height=58)
        self.header_frame.pack(fill="x")
        self.header_frame.pack_propagate(False)

        self.lbl_title = tk.Label(self.header_frame,
            text="  Claude Prompt Scheduler",
            font=("Segoe UI", 14, "bold"))
        self.lbl_title.pack(side="left", padx=18, pady=14)

        self.btn_theme = tk.Button(self.header_frame, text="Light",
            font=("Segoe UI", 9), relief="flat", bd=0,
            padx=12, pady=6, cursor="hand2", command=self._toggle_theme)
        self.btn_theme.pack(side="right", padx=14, pady=12)

        self._divider = tk.Frame(self, height=1)
        self._divider.pack(fill="x")

        # PanedWindow: top=controls (fixed min), bottom=log (resizable)
        self.paned = tk.PanedWindow(self, orient="vertical",
            sashwidth=7, sashrelief="flat", bd=0)
        self.paned.pack(fill="both", expand=True)

        self.top_panel = tk.Frame(self.paned)
        self.paned.add(self.top_panel, minsize=395)

        self.body = tk.Frame(self.top_panel)
        self.body.pack(fill="both", expand=True, padx=16, pady=10)

        self.log_panel = tk.Frame(self.paned)
        self.paned.add(self.log_panel, minsize=190)

        self._build_status_card()
        self._build_mode_card()
        self._build_time_card()
        self._build_prompt_card()
        self._build_options_card()
        self._build_action_buttons()
        self._build_log_panel()

    def _card(self, title):
        f = tk.Frame(self.body)
        f.pack(fill="x", pady=(0, 8))
        lbl = tk.Label(f, text=title, font=("Segoe UI", 8, "bold"))
        lbl.pack(anchor="w", padx=2, pady=(0, 4))
        inner = tk.Frame(f)
        inner.pack(fill="x")
        self._cards.append((f, inner, lbl))
        return inner

    def _build_status_card(self):
        inner = self._card("TRANG THAI")
        self.status_panel = tk.Frame(inner, height=88)
        self.status_panel.pack(fill="x")
        self.status_panel.pack_propagate(False)

        left = tk.Frame(self.status_panel)
        left.pack(side="left", fill="both", expand=True)
        self.lbl_countdown = tk.Label(left, textvariable=self.countdown_var,
            font=("Segoe UI Mono", 30, "bold"))
        self.lbl_countdown.pack(pady=(10, 2))
        self.lbl_status = tk.Label(left, textvariable=self.status_var,
            font=("Segoe UI", 8), wraplength=230, justify="center")
        self.lbl_status.pack()

        right = tk.Frame(self.status_panel, width=150)
        right.pack(side="right", fill="y", padx=(0, 12))
        right.pack_propagate(False)
        self.lbl_target_hdr = tk.Label(right, text="Muc tieu",
            font=("Segoe UI", 8, "bold"))
        self.lbl_target_hdr.pack(pady=(12, 2))
        self.lbl_target = tk.Label(right, textvariable=self.target_str,
            font=("Segoe UI Mono", 11, "bold"))
        self.lbl_target.pack()
        self.dot_indicator = tk.Label(right, text="●", font=("Segoe UI", 22))
        self.dot_indicator.pack(pady=(2, 0))

    def _build_mode_card(self):
        inner = self._card("CHE DO")
        row = tk.Frame(inner)
        row.pack(fill="x")
        self.rb_time = tk.Radiobutton(row, text="Theo gio cu the",
            variable=self.mode_var, value="time",
            font=("Segoe UI", 10), relief="flat",
            command=self._on_mode_change, cursor="hand2")
        self.rb_time.pack(side="left", padx=(0, 24))
        self.rb_countdown = tk.Radiobutton(row, text="Dem nguoc",
            variable=self.mode_var, value="countdown",
            font=("Segoe UI", 10), relief="flat",
            command=self._on_mode_change, cursor="hand2")
        self.rb_countdown.pack(side="left")

    def _build_time_card(self):
        self.time_card = tk.Frame(self.body)
        self.time_card.pack(fill="x", pady=(0, 8))
        lbl = tk.Label(self.time_card, text="THOI GIAN", font=("Segoe UI", 8, "bold"))
        lbl.pack(anchor="w", padx=2, pady=(0, 4))
        self._cards.append((self.time_card, None, lbl))

        self.time_input_frame = tk.Frame(self.time_card)
        self.time_input_frame.pack(fill="x")
        row = tk.Frame(self.time_input_frame)
        row.pack()
        tk.Label(row, text="Gio:", font=("Segoe UI", 10)).pack(side="left", padx=(0, 4))
        self.entry_hour = tk.Spinbox(row, from_=0, to=23, width=3,
            textvariable=self.hour_var, font=("Segoe UI Mono", 16, "bold"),
            justify="center", relief="flat", bd=0, format="%02.0f",
            command=self._preview_target)
        self.entry_hour.pack(side="left", padx=(0, 4))
        tk.Label(row, text=":", font=("Segoe UI Mono", 18, "bold")).pack(side="left", padx=2)
        tk.Label(row, text="Phut:", font=("Segoe UI", 10)).pack(side="left", padx=(4, 4))
        self.entry_minute = tk.Spinbox(row, from_=0, to=59, width=3,
            textvariable=self.minute_var, font=("Segoe UI Mono", 16, "bold"),
            justify="center", relief="flat", bd=0, format="%02.0f",
            command=self._preview_target)
        self.entry_minute.pack(side="left")

        qrow = tk.Frame(self.time_input_frame)
        qrow.pack(pady=(8, 0))
        self.quick_btns = []
        for label, h, m in [("Reset 5:40","05","40"),("Reset 9:00","09","00"),
                             ("+30 phut",None,None),("+1 gio",None,None)]:
            b = tk.Button(qrow, text=label, font=("Segoe UI", 8),
                relief="flat", bd=0, padx=8, pady=4, cursor="hand2",
                command=lambda _h=h,_m=m,_l=label: self._quick_time(_h,_m,_l))
            b.pack(side="left", padx=3)
            self.quick_btns.append(b)

        self.countdown_input_frame = tk.Frame(self.time_card)
        row2 = tk.Frame(self.countdown_input_frame)
        row2.pack()
        for var, label, maxv in [(self.cd_hour_var,"Gio",99),
                                  (self.cd_min_var,"Phut",59),
                                  (self.cd_sec_var,"Giay",59)]:
            tk.Label(row2, text=label+":", font=("Segoe UI",10)).pack(side="left", padx=(4,2))
            tk.Spinbox(row2, from_=0, to=maxv, width=4, textvariable=var,
                font=("Segoe UI Mono",14,"bold"),
                justify="center", relief="flat", bd=0).pack(side="left", padx=(0,6))

    def _build_prompt_card(self):
        inner = self._card("NOI DUNG PROMPT")
        self.prompt_text = scrolledtext.ScrolledText(inner, height=5,
            font=("Segoe UI", 10), relief="flat", bd=0, wrap="word",
            padx=10, pady=8)
        self.prompt_text.pack(fill="both", expand=True)
        self.prompt_text.insert("1.0", "continue")

        prow = tk.Frame(inner)
        prow.pack(fill="x", pady=(6,0))
        self.preset_btns = []
        for p in ["continue","tiep tuc","lam tiep di","next task","go ahead"]:
            b = tk.Button(prow, text=p, font=("Segoe UI",8),
                relief="flat", bd=0, padx=7, pady=3, cursor="hand2",
                command=lambda _p=p: self._set_preset(_p))
            b.pack(side="left", padx=2)
            self.preset_btns.append(b)

    def _build_options_card(self):
        inner = self._card("TUY CHON")
        row = tk.Frame(inner)
        row.pack(fill="x")
        self.chk_enter = tk.Checkbutton(row,
            text="Tu dong nhan Enter sau khi gui",
            variable=self.press_enter, font=("Segoe UI",10), relief="flat")
        self.chk_enter.pack(side="left")
        self.chk_repeat = tk.Checkbutton(row,
            text="Lap lai moi ngay",
            variable=self.repeat_var, font=("Segoe UI",10), relief="flat")
        self.chk_repeat.pack(side="left", padx=(20,0))

    def _build_action_buttons(self):
        self.btn_frame = tk.Frame(self.body)
        self.btn_frame.pack(fill="x", pady=(4,0))
        self.btn_start = tk.Button(self.btn_frame, text="  Bat dau hen gio",
            font=("Segoe UI",10,"bold"), relief="flat", bd=0,
            padx=16, pady=10, cursor="hand2", command=self._toggle_timer)
        self.btn_start.pack(side="left", padx=(0,6))
        self.btn_test = tk.Button(self.btn_frame, text="Test ngay",
            font=("Segoe UI",10), relief="flat", bd=0,
            padx=12, pady=10, cursor="hand2", command=self._test_send)
        self.btn_test.pack(side="left", padx=(0,6))
        self.btn_find = tk.Button(self.btn_frame, text="Tim Claude",
            font=("Segoe UI",10), relief="flat", bd=0,
            padx=12, pady=10, cursor="hand2", command=self._find_windows)
        self.btn_find.pack(side="left")

    def _build_log_panel(self):
        hdr = tk.Frame(self.log_panel)
        hdr.pack(fill="x", padx=12, pady=(8,4))

        self.lbl_log_hdr = tk.Label(hdr, text="LOG HOAT DONG",
            font=("Segoe UI",9,"bold"))
        self.lbl_log_hdr.pack(side="left")

        self.lbl_log_count = tk.Label(hdr, text="0 dong", font=("Segoe UI",8))
        self.lbl_log_count.pack(side="left", padx=(8,0))

        self.btn_copy_log = tk.Button(hdr, text="Copy log",
            font=("Segoe UI",8), relief="flat", bd=0,
            padx=8, pady=3, cursor="hand2", command=self._copy_log)
        self.btn_copy_log.pack(side="right", padx=(4,0))

        self.btn_clear_log = tk.Button(hdr, text="Xoa log",
            font=("Segoe UI",8), relief="flat", bd=0,
            padx=8, pady=3, cursor="hand2", command=self._clear_log)
        self.btn_clear_log.pack(side="right")

        container = tk.Frame(self.log_panel)
        container.pack(fill="both", expand=True, padx=12, pady=(0,8))

        self.log_scroll = tk.Scrollbar(container)
        self.log_scroll.pack(side="right", fill="y")

        self.log_text = tk.Text(container,
            font=("Consolas",9), relief="flat", bd=0,
            state="disabled", wrap="word", padx=10, pady=8,
            yscrollcommand=self.log_scroll.set)
        self.log_text.pack(fill="both", expand=True)
        self.log_scroll.config(command=self.log_text.yview)

        # Colour tags (filled in _apply_theme)
        for tag in ["TIME","INFO","SUCCESS","ERROR","WARN","TIER1","TIER2","SEND","SEP"]:
            self.log_text.tag_configure(tag)
        self.log_text.tag_configure("BOLD", font=("Consolas",9,"bold"))

    # ── Logging ────────────────────────────────────────────────────────────

    BADGES = {
        "SUCCESS": " OK  ",
        "ERROR":   " ERR ",
        "WARN":    " WRN ",
        "TIER1":   " T1  ",
        "TIER2":   " T2  ",
        "SEND":    " SND ",
        "INFO":    " --- ",
        "SEP":     "",
    }

    def _log(self, msg, level=None):
        if level is None:
            # auto-detect
            ml = msg.lower()
            if any(x in ml for x in ["thanh cong","ok!","hoan thanh","t1] tim thay","t2] da gui"]):
                level = "SUCCESS"
            elif any(x in ml for x in ["loi:","that bai","khong tim thay","error","exception"]):
                level = "ERROR"
            elif any(x in ml for x in ["warn","dung hen gio","con "]):
                level = "WARN"
            elif "[t1]" in ml or "tier 1" in ml:
                level = "TIER1"
            elif "[t2]" in ml or "tier 2" in ml:
                level = "TIER2"
            elif any(x in ml for x in ["gui prompt","paste","test gui","den gio","send","dang paste"]):
                level = "SEND"
            else:
                level = "INFO"

        ts = datetime.datetime.now().strftime("%H:%M:%S")
        self.log_count += 1
        badge = self.BADGES.get(level, " --- ")

        self.log_text.configure(state="normal")
        if level == "SEP":
            self.log_text.insert("end", msg + "\n", "SEP")
        else:
            self.log_text.insert("end", f"[{ts}] ", "TIME")
            self.log_text.insert("end", badge, (level, "BOLD"))
            self.log_text.insert("end", f"  {msg}\n", level)
        self.log_text.see("end")
        self.log_text.configure(state="disabled")

        self.lbl_log_count.configure(text=f"{self.log_count} dong")
        short = msg[:52] + ("..." if len(msg) > 52 else "")
        self.status_var.set(short)

    def _clear_log(self):
        self.log_text.configure(state="normal")
        self.log_text.delete("1.0", "end")
        self.log_text.configure(state="disabled")
        self.log_count = 0
        self.lbl_log_count.configure(text="0 dong")
        self._log("Log da duoc xoa.", "INFO")

    def _copy_log(self):
        content = self.log_text.get("1.0", "end").strip()
        self.clipboard_clear()
        self.clipboard_append(content)
        self._log("Da copy toan bo log vao clipboard!", "SUCCESS")

    # ── Timer Logic ────────────────────────────────────────────────────────

    def _preview_target(self):
        try:
            now = datetime.datetime.now()
            h = int(self.hour_var.get())
            m = int(self.minute_var.get())
            t = now.replace(hour=h, minute=m, second=0, microsecond=0)
            if t <= now:
                t += datetime.timedelta(days=1)
            self.target_str.set(t.strftime("%H:%M\n%d/%m"))
        except Exception:
            pass

    def _on_mode_change(self):
        if self.mode_var.get() == "time":
            self.countdown_input_frame.pack_forget()
            self.time_input_frame.pack(fill="x")
        else:
            self.time_input_frame.pack_forget()
            self.countdown_input_frame.pack(fill="x")

    def _quick_time(self, h, m, label):
        now = datetime.datetime.now()
        if h is not None:
            self.hour_var.set(h); self.minute_var.set(m)
        else:
            delta = datetime.timedelta(minutes=30) if "+30" in label else datetime.timedelta(hours=1)
            t = now + delta
            self.hour_var.set(f"{t.hour:02d}"); self.minute_var.set(f"{t.minute:02d}")
        self.mode_var.set("time"); self._on_mode_change()
        self._preview_target()

    def _set_preset(self, text):
        self.prompt_text.delete("1.0", "end")
        self.prompt_text.insert("1.0", text)

    def _get_target(self):
        now = datetime.datetime.now()
        if self.mode_var.get() == "time":
            try:
                h, m = int(self.hour_var.get()), int(self.minute_var.get())
            except ValueError:
                return None, "Gio/phut khong hop le"
            target = now.replace(hour=h, minute=m, second=0, microsecond=0)
            if target <= now:
                target += datetime.timedelta(days=1)
            return target, None
        else:
            try:
                secs = (int(self.cd_hour_var.get()) * 3600 +
                        int(self.cd_min_var.get())  * 60 +
                        int(self.cd_sec_var.get()))
            except ValueError:
                return None, "Thoi gian khong hop le"
            if secs <= 0:
                return None, "Phai > 0 giay"
            return now + datetime.timedelta(seconds=secs), None

    def _toggle_timer(self):
        if self.is_running: self._stop_timer()
        else:               self._start_timer()

    def _start_timer(self):
        prompt = self.prompt_text.get("1.0", "end").strip()
        if not prompt:
            messagebox.showwarning("Thieu prompt", "Vui long nhap noi dung prompt!")
            return
        target, err = self._get_target()
        if err:
            messagebox.showerror("Loi", err); return

        self.is_running = True
        self.target_dt  = target
        self.btn_start.configure(text="  Dung hen gio", bg=T()["error"])
        self.target_str.set(target.strftime("%H:%M\n%d/%m"))
        self.dot_indicator.configure(fg=T()["warning"])

        self._log("=" * 36, "SEP")
        self._log("HEN GIO BAT DAU", "INFO")
        self._log(f"Muc tieu : {target.strftime('%H:%M:%S  %d/%m/%Y')}", "INFO")
        prev = prompt[:35] + ("..." if len(prompt) > 35 else "")
        self._log(f"Prompt   : \"{prev}\"", "INFO")
        self._log(f"Enter    : {'Co' if self.press_enter.get() else 'Khong'}", "INFO")
        self._log(f"Lap lai  : {'Moi ngay' if self.repeat_var.get() else 'Khong'}", "INFO")

        self.timer_thread = threading.Thread(target=self._timer_loop, daemon=True)
        self.timer_thread.start()

    def _stop_timer(self):
        self.is_running = False
        self.btn_start.configure(text="  Bat dau hen gio", bg=T()["accent_btn"])
        self.target_str.set("--")
        self.dot_indicator.configure(fg=T()["text_muted"])
        self._log("Hen gio da dung.", "WARN")

    def _timer_loop(self):
        warned_10 = False
        while self.is_running:
            remaining = (self.target_dt - datetime.datetime.now()).total_seconds()
            if remaining <= 0:
                self.countdown_var.set("00:00:00")
                self._fire()
                if self.repeat_var.get():
                    self.target_dt += datetime.timedelta(days=1)
                    self._log(f"Lap lai: {self.target_dt.strftime('%H:%M %d/%m')}", "INFO")
                    warned_10 = False
                    time.sleep(5); continue
                else:
                    self.after(0, self._stop_timer); break

            if remaining <= 10 and not warned_10:
                self._log(f"Con {int(remaining)} giay!", "WARN")
                warned_10 = True

            h = int(remaining // 3600)
            m = int((remaining % 3600) // 60)
            s = int(remaining % 60)
            self.countdown_var.set(f"{h:02d}:{m:02d}:{s:02d}")

            if remaining < 10:
                clr = T()["error"] if int(remaining) % 2 == 0 else T()["warning"]
                self.after(0, lambda c=clr: self.dot_indicator.configure(fg=c))

            time.sleep(0.5)

    def _fire(self):
        self._log("=" * 36, "SEP")
        self._log("DEN GIO! DANG GUI PROMPT...", "SEND")
        self.dot_indicator.configure(fg=T()["success"])
        prompt = self.prompt_text.get("1.0", "end").strip()
        try:
            ok = send_prompt_to_claude(prompt, self.press_enter.get(), self._log)
            if ok:
                self._log("HOAN THANH: Gui prompt thanh cong!", "SUCCESS")
            else:
                self._log("THAT BAI: Khong gui duoc prompt.", "ERROR")
        except Exception as e:
            self._log(f"Exception: {e}", "ERROR")
        self._log("=" * 36, "SEP")

    def _test_send(self):
        prompt = self.prompt_text.get("1.0", "end").strip()
        if not prompt:
            messagebox.showwarning("Thieu", "Nhap prompt truoc!"); return
        self._log("-" * 36, "SEP")
        self._log("TEST GUI NGAY...", "SEND")
        prev = prompt[:35] + ("..." if len(prompt) > 35 else "")
        self._log(f"Prompt: \"{prev}\"", "SEND")
        threading.Thread(target=send_prompt_to_claude,
            args=(prompt, self.press_enter.get()),
            kwargs={"status_cb": self._log}, daemon=True).start()

    def _find_windows(self):
        self._log("Tim kiem cua so Claude Code...", "INFO")
        wins = find_claude_windows()
        if wins:
            self._log(f"Tim thay {len(wins)} cua so:", "SUCCESS")
            for hwnd, title in wins:
                self._log(f"  [{hwnd}] {title}", "SUCCESS")
        else:
            self._log("Khong tim thay cua so nao!", "ERROR")
            self._log("Hay dam bao Claude Code dang mo.", "WARN")

    def _tick(self):
        if not self.is_running:
            self.countdown_var.set(datetime.datetime.now().strftime("%H:%M:%S"))
        self.after(1000, self._tick)

    # ── Theme ──────────────────────────────────────────────────────────────

    def _toggle_theme(self):
        global current_theme
        current_theme = "light" if current_theme == "dark" else "dark"
        self.btn_theme.configure(
            text="Dark" if current_theme == "light" else "Light")
        self._apply_theme()

    def _apply_theme(self):
        t = T()
        self.configure(bg=t["bg"])
        self.header_frame.configure(bg=t["surface"])
        self.lbl_title.configure(bg=t["surface"], fg=t["text"])
        self.btn_theme.configure(bg=t["surface2"], fg=t["text_muted"],
            activebackground=t["border"], activeforeground=t["text"])
        self._divider.configure(bg=t["border"])
        self.paned.configure(bg=t["border"])
        self.top_panel.configure(bg=t["bg"])
        self.body.configure(bg=t["bg"])
        self.log_panel.configure(bg=t["bg"])

        for frame, inner, lbl in self._cards:
            frame.configure(bg=t["bg"])
            lbl.configure(bg=t["bg"], fg=t["text_muted"])
            if inner: inner.configure(bg=t["bg"])

        self.status_panel.configure(bg=t["surface"])
        self.lbl_countdown.configure(bg=t["surface"], fg=t["accent2"])
        self.lbl_status.configure(bg=t["surface"], fg=t["text_muted"])
        for w in self.status_panel.winfo_children():
            if isinstance(w, tk.Frame):
                w.configure(bg=t["surface"])
                for ww in w.winfo_children():
                    if isinstance(ww, tk.Label):
                        ww.configure(bg=t["surface"], fg=t["text_muted"])
        self.lbl_countdown.configure(fg=t["accent2"])
        self.lbl_target.configure(fg=t["accent"])
        self.dot_indicator.configure(
            fg=t["warning"] if self.is_running else t["text_muted"])

        for rb in [self.rb_time, self.rb_countdown]:
            rb.configure(bg=t["bg"], fg=t["text"],
                activebackground=t["bg"], selectcolor=t["surface2"])

        self.time_card.configure(bg=t["bg"])
        self.time_input_frame.configure(bg=t["bg"])
        self.countdown_input_frame.configure(bg=t["bg"])

        def style_frame(f):
            f.configure(bg=t["bg"])
            for w in f.winfo_children():
                if isinstance(w, tk.Frame):    style_frame(w)
                elif isinstance(w, tk.Label):  w.configure(bg=t["bg"], fg=t["text"])
                elif isinstance(w, tk.Spinbox): w.configure(
                    bg=t["input_bg"], fg=t["accent2"],
                    buttonbackground=t["surface2"],
                    insertbackground=t["text"], relief="flat")

        style_frame(self.time_input_frame)
        style_frame(self.countdown_input_frame)

        for b in self.quick_btns:
            b.configure(bg=t["surface2"], fg=t["text_muted"],
                activebackground=t["border"], activeforeground=t["text"])

        self.prompt_text.configure(bg=t["input_bg"], fg=t["text"],
            insertbackground=t["text"])
        for b in self.preset_btns:
            b.configure(bg=t["surface2"], fg=t["text_muted"],
                activebackground=t["border"], activeforeground=t["text"])

        for chk in [self.chk_enter, self.chk_repeat]:
            chk.configure(bg=t["bg"], fg=t["text"],
                activebackground=t["bg"], selectcolor=t["surface2"])

        self.btn_start.configure(bg=t["accent_btn"], fg="#ffffff",
            activebackground=t["accent"], activeforeground="#ffffff")
        self.btn_test.configure(bg=t["surface2"], fg=t["text"],
            activebackground=t["border"], activeforeground=t["text"])
        self.btn_find.configure(bg=t["surface2"], fg=t["text"],
            activebackground=t["border"], activeforeground=t["text"])
        self.btn_frame.configure(bg=t["bg"])

        # Log panel
        self.lbl_log_hdr.configure(bg=t["bg"], fg=t["text_muted"])
        self.lbl_log_count.configure(bg=t["bg"], fg=t["text_muted"])
        self.btn_clear_log.configure(bg=t["surface2"], fg=t["error"],
            activebackground=t["border"], activeforeground=t["error"])
        self.btn_copy_log.configure(bg=t["surface2"], fg=t["accent2"],
            activebackground=t["border"], activeforeground=t["accent2"])
        self.log_text.configure(bg=t["surface"], fg=t["text"],
            insertbackground=t["text"])
        self.log_scroll.configure(bg=t["surface2"],
            troughcolor=t["bg"], activebackground=t["border"])

        # Log color tags
        self.log_text.tag_configure("TIME",    foreground=t["text_muted"])
        self.log_text.tag_configure("INFO",    foreground=t["text_muted"])
        self.log_text.tag_configure("SEP",     foreground=t["border"])
        self.log_text.tag_configure("SUCCESS", foreground=t["success"])
        self.log_text.tag_configure("ERROR",   foreground=t["error"])
        self.log_text.tag_configure("WARN",    foreground=t["warning"])
        self.log_text.tag_configure("TIER1",   foreground="#d2a8ff")
        self.log_text.tag_configure("TIER2",   foreground="#ffa657")
        self.log_text.tag_configure("SEND",    foreground=t["accent2"])
        self.log_text.tag_configure("BOLD",    font=("Consolas",9,"bold"))


if __name__ == "__main__":
    app = ClaudeScheduler()
    app.mainloop()
