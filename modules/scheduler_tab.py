"""
scheduler_tab.py — UI Automation & Hẹn giờ gửi Prompt vào Claude Desktop / Win32 window
Tích hợp customtkinter cho giao diện Modern UI/UX.
"""

import customtkinter as ctk
import tkinter as tk
from tkinter import messagebox
import threading
import datetime
import time
import subprocess
import ctypes
import sys

user32   = ctypes.windll.user32
kernel32 = ctypes.windll.kernel32

try:
    import win32gui, win32con, win32api
except ImportError:
    pass


def find_claude_windows():
    found = []
    def cb(hwnd, _):
        if win32gui.IsWindowVisible(hwnd):
            t = win32gui.GetWindowText(hwnd).lower()
            if any(k in t for k in ["claude", "anthropic"]) \
               and "suite" not in t and "manager" not in t and "scheduler" not in t:
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


def send_prompt_to_claude_gui(prompt, press_enter=True, status_cb=None):
    def log(m, lvl="INFO"):
        if status_cb:
            status_cb(m, lvl)

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


class SchedulerTabFrame(ctk.CTkFrame):
    """
    Scheduler Tab bằng CustomTkinter — Giao diện hiện đại.
    """
    def __init__(self, parent, log_fn):
        super().__init__(parent, fg_color="transparent")
        self.log_fn = log_fn

        self.is_running    = False
        self.timer_thread  = None
        self.countdown_var = ctk.StringVar(value="00:00:00")
        self.status_var    = ctk.StringVar(value="Sẵn sàng")
        self.target_str    = ctk.StringVar(value="--:--")
        self.mode_var      = ctk.StringVar(value="time")
        self.press_enter   = ctk.BooleanVar(value=True)
        self.repeat_var    = ctk.BooleanVar(value=False)
        self.hour_var      = ctk.StringVar(value="05")
        self.minute_var    = ctk.StringVar(value="40")
        self.cd_hour_var   = ctk.StringVar(value="0")
        self.cd_min_var    = ctk.StringVar(value="30")
        self.cd_sec_var    = ctk.StringVar(value="0")

        self._build_ui()
        self._tick()

    def _build_ui(self):
        # Scrollable container for scheduler settings
        self.scroll = ctk.CTkScrollableFrame(self, fg_color="transparent")
        self.scroll.pack(fill="both", expand=True, padx=12, pady=12)

        # ── Card 1: Status Hero ──
        self.status_card = ctk.CTkFrame(self.scroll, corner_radius=12)
        self.status_card.pack(fill="x", pady=(0, 12))

        header_frame = ctk.CTkFrame(self.status_card, fg_color="transparent")
        header_frame.pack(fill="x", padx=16, pady=(12, 0))

        ctk.CTkLabel(header_frame, text="⏰  TRẠNG THÁI HẸN GIỜ",
                     font=ctk.CTkFont(size=12, weight="bold"),
                     text_color=("gray30", "gray70")).pack(side="left")

        hero_body = ctk.CTkFrame(self.status_card, fg_color="transparent")
        hero_body.pack(fill="x", padx=16, pady=12)

        # Left countdown
        left_hero = ctk.CTkFrame(hero_body, fg_color="transparent")
        left_hero.pack(side="left", expand=True)

        self.lbl_countdown = ctk.CTkLabel(
            left_hero, textvariable=self.countdown_var,
            font=ctk.CTkFont(family="Segoe UI Mono", size=36, weight="bold"),
            text_color=("#1f6feb", "#38bdf8")
        )
        self.lbl_countdown.pack()

        self.lbl_status = ctk.CTkLabel(
            left_hero, textvariable=self.status_var,
            font=ctk.CTkFont(size=12), text_color=("gray40", "gray60")
        )
        self.lbl_status.pack()

        # Right target pill
        right_hero = ctk.CTkFrame(hero_body, fg_color=("gray90", "gray20"), corner_radius=10, width=140)
        right_hero.pack(side="right", padx=(12, 0), fill="y")

        ctk.CTkLabel(right_hero, text="MỤC TIÊU", font=ctk.CTkFont(size=11, weight="bold"),
                     text_color=("gray40", "gray60")).pack(pady=(10, 2), padx=14)

        self.lbl_target = ctk.CTkLabel(
            right_hero, textvariable=self.target_str,
            font=ctk.CTkFont(family="Segoe UI Mono", size=14, weight="bold"),
            text_color=("#d97706", "#fbbf24")
        )
        self.lbl_target.pack(padx=14)

        self.dot_indicator = ctk.CTkLabel(right_hero, text="●", font=ctk.CTkFont(size=16),
                                         text_color="gray50")
        self.dot_indicator.pack(pady=(2, 8))

        # ── Card 2: Mode & Time settings ──
        self.time_card = ctk.CTkFrame(self.scroll, corner_radius=12)
        self.time_card.pack(fill="x", pady=(0, 12))

        ctk.CTkLabel(self.time_card, text="⚙️  CẤU HÌNH THỜI GIAN",
                     font=ctk.CTkFont(size=12, weight="bold"),
                     text_color=("gray30", "gray70")).pack(anchor="w", padx=16, pady=(12, 8))

        # Mode radio segment
        mode_row = ctk.CTkFrame(self.time_card, fg_color="transparent")
        mode_row.pack(fill="x", padx=16, pady=(0, 8))

        self.rb_time = ctk.CTkRadioButton(
            mode_row, text="Theo giờ cố định (24h)", variable=self.mode_var,
            value="time", command=self._on_mode_change, font=ctk.CTkFont(size=12)
        )
        self.rb_time.pack(side="left", padx=(0, 20))

        self.rb_countdown = ctk.CTkRadioButton(
            mode_row, text="Đếm ngược (Timer)", variable=self.mode_var,
            value="countdown", command=self._on_mode_change, font=ctk.CTkFont(size=12)
        )
        self.rb_countdown.pack(side="left")

        # Fixed time inputs
        self.time_input_frame = ctk.CTkFrame(self.time_card, fg_color="transparent")
        self.time_input_frame.pack(fill="x", padx=16, pady=(0, 12))

        t_row = ctk.CTkFrame(self.time_input_frame, fg_color="transparent")
        t_row.pack(anchor="w")

        ctk.CTkLabel(t_row, text="Giờ:", font=ctk.CTkFont(size=12)).pack(side="left", padx=(0, 4))
        self.entry_hour = ctk.CTkEntry(t_row, textvariable=self.hour_var, width=50,
                                        font=ctk.CTkFont(family="Segoe UI Mono", size=14, weight="bold"),
                                        justify="center")
        self.entry_hour.pack(side="left", padx=(0, 8))

        ctk.CTkLabel(t_row, text="Phút:", font=ctk.CTkFont(size=12)).pack(side="left", padx=(0, 4))
        self.entry_minute = ctk.CTkEntry(t_row, textvariable=self.minute_var, width=50,
                                          font=ctk.CTkFont(family="Segoe UI Mono", size=14, weight="bold"),
                                          justify="center")
        self.entry_minute.pack(side="left", padx=(0, 12))

        # Quick preset buttons
        q_row = ctk.CTkFrame(self.time_input_frame, fg_color="transparent")
        q_row.pack(anchor="w", pady=(8, 0))

        for label, h, m in [("5:40 AM", "05", "40"), ("9:00 AM", "09", "00"),
                             ("+30 Phút", None, None), ("+1 Giờ", None, None)]:
            ctk.CTkButton(
                q_row, text=label, width=80, height=28,
                fg_color=("gray85", "gray25"), text_color=("gray10", "gray90"),
                hover_color=("gray75", "gray35"), corner_radius=6,
                font=ctk.CTkFont(size=11),
                command=lambda _h=h, _m=m, _l=label: self._quick_time(_h, _m, _l)
            ).pack(side="left", padx=3)

        # Countdown inputs (hidden by default)
        self.cd_input_frame = ctk.CTkFrame(self.time_card, fg_color="transparent")
        cd_row = ctk.CTkFrame(self.cd_input_frame, fg_color="transparent")
        cd_row.pack(anchor="w")

        for var, lbl_t in [(self.cd_hour_var, "Giờ:"), (self.cd_min_var, "Phút:"), (self.cd_sec_var, "Giây:")]:
            ctk.CTkLabel(cd_row, text=lbl_t, font=ctk.CTkFont(size=12)).pack(side="left", padx=(0, 4))
            ctk.CTkEntry(cd_row, textvariable=var, width=50,
                         font=ctk.CTkFont(family="Segoe UI Mono", size=14, weight="bold"),
                         justify="center").pack(side="left", padx=(0, 10))

        # ── Card 3: Prompt content ──
        self.prompt_card = ctk.CTkFrame(self.scroll, corner_radius=12)
        self.prompt_card.pack(fill="x", pady=(0, 12))

        ctk.CTkLabel(self.prompt_card, text="💬  PROMPT NỘI DUNG TỰ ĐỘNG",
                     font=ctk.CTkFont(size=12, weight="bold"),
                     text_color=("gray30", "gray70")).pack(anchor="w", padx=16, pady=(12, 6))

        self.prompt_text = ctk.CTkTextbox(
            self.prompt_card, height=90, font=ctk.CTkFont(size=12),
            corner_radius=8, border_width=1
        )
        self.prompt_text.pack(fill="x", padx=16, pady=(0, 8))
        self.prompt_text.insert("1.0", "continue")

        # Preset prompts row
        p_row = ctk.CTkFrame(self.prompt_card, fg_color="transparent")
        p_row.pack(fill="x", padx=16, pady=(0, 12))

        for p in ["continue", "tiếp tục", "làm tiếp đi", "next task", "go ahead"]:
            ctk.CTkButton(
                p_row, text=p, width=70, height=26,
                fg_color=("gray85", "gray25"), text_color=("gray10", "gray90"),
                hover_color=("gray75", "gray35"), corner_radius=6,
                font=ctk.CTkFont(size=11),
                command=lambda _p=p: self._set_preset(_p)
            ).pack(side="left", padx=2)

        # ── Card 4: Options & Actions ──
        self.opt_card = ctk.CTkFrame(self.scroll, corner_radius=12)
        self.opt_card.pack(fill="x", pady=(0, 12))

        opts_row = ctk.CTkFrame(self.opt_card, fg_color="transparent")
        opts_row.pack(fill="x", padx=16, pady=12)

        self.chk_enter = ctk.CTkCheckBox(
            opts_row, text="Tự động nhấn Enter sau khi paste",
            variable=self.press_enter, font=ctk.CTkFont(size=12)
        )
        self.chk_enter.pack(side="left", padx=(0, 20))

        self.chk_repeat = ctk.CTkCheckBox(
            opts_row, text="Lặp lại hàng ngày",
            variable=self.repeat_var, font=ctk.CTkFont(size=12)
        )
        self.chk_repeat.pack(side="left")

        # Action Buttons
        btn_row = ctk.CTkFrame(self.scroll, fg_color="transparent")
        btn_row.pack(fill="x", pady=(0, 8))

        self.btn_start = ctk.CTkButton(
            btn_row, text="▶  Bắt Đầu Hẹn Giờ", height=42,
            font=ctk.CTkFont(size=13, weight="bold"),
            fg_color=("#16a34a", "#22c55e"), hover_color=("#15803d", "#16a34a"),
            corner_radius=8, command=self._toggle_timer
        )
        self.btn_start.pack(side="left", expand=True, fill="x", padx=(0, 6))

        self.btn_test = ctk.CTkButton(
            btn_row, text="⚡ Test GUI Ngay", height=42,
            font=ctk.CTkFont(size=12, weight="bold"),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=8,
            command=self._test_send
        )
        self.btn_test.pack(side="left", padx=(0, 6))

        self.btn_find = ctk.CTkButton(
            btn_row, text="🔍 Tìm Cửa Sổ Claude", height=42,
            font=ctk.CTkFont(size=12),
            fg_color=("gray80", "gray25"), text_color=("gray10", "gray90"),
            hover_color=("gray70", "gray35"), corner_radius=8,
            command=self._find_windows
        )
        self.btn_find.pack(side="left")

    def _preview_target(self):
        try:
            now = datetime.datetime.now()
            h = int(self.hour_var.get())
            m = int(self.minute_var.get())
            t = now.replace(hour=h, minute=m, second=0, microsecond=0)
            if t <= now:
                t += datetime.timedelta(days=1)
            self.target_str.set(t.strftime("%H:%M  %d/%m"))
        except Exception:
            pass

    def _on_mode_change(self):
        if self.mode_var.get() == "time":
            self.cd_input_frame.pack_forget()
            self.time_input_frame.pack(fill="x", padx=16, pady=(0, 12))
        else:
            self.time_input_frame.pack_forget()
            self.cd_input_frame.pack(fill="x", padx=16, pady=(0, 12))

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
                return None, "Giờ/phút không hợp lệ"
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
                return None, "Thời gian không hợp lệ"
            if secs <= 0:
                return None, "Phải > 0 giây"
            return now + datetime.timedelta(seconds=secs), None

    def _toggle_timer(self):
        if self.is_running: self._stop_timer()
        else:               self._start_timer()

    def _start_timer(self):
        prompt = self.prompt_text.get("1.0", "end").strip()
        if not prompt:
            messagebox.showwarning("Thiếu prompt", "Vui lòng nhập nội dung prompt!")
            return
        target, err = self._get_target()
        if err:
            messagebox.showerror("Lỗi", err); return

        self.is_running = True
        self.target_dt  = target
        self.btn_start.configure(
            text="⏹  Dừng Hẹn Giờ",
            fg_color=("#dc2626", "#ef4444"), hover_color=("#b91c1c", "#dc2626")
        )
        self.target_str.set(target.strftime("%H:%M  %d/%m"))
        self.dot_indicator.configure(text_color="#f59e0b")

        self.log_fn("=" * 36, "SEP")
        self.log_fn("HẸN GIỜ BẮT ĐẦU (Win32 GUI Automation)", "INFO")
        self.log_fn(f"Mục tiêu : {target.strftime('%H:%M:%S  %d/%m/%Y')}", "INFO")
        prev = prompt[:35] + ("..." if len(prompt) > 35 else "")
        self.log_fn(f"Prompt   : \"{prev}\"", "INFO")

        self.timer_thread = threading.Thread(target=self._timer_loop, daemon=True)
        self.timer_thread.start()

    def _stop_timer(self):
        self.is_running = False
        self.btn_start.configure(
            text="▶  Bắt Đầu Hẹn Giờ",
            fg_color=("#16a34a", "#22c55e"), hover_color=("#15803d", "#16a34a")
        )
        self.target_str.set("--:--")
        self.dot_indicator.configure(text_color="gray50")
        self.log_fn("Hẹn giờ đã dừng.", "WARN")

    def _timer_loop(self):
        warned_10 = False
        while self.is_running:
            remaining = (self.target_dt - datetime.datetime.now()).total_seconds()
            if remaining <= 0:
                self.countdown_var.set("00:00:00")
                self._fire()
                if self.repeat_var.get():
                    self.target_dt += datetime.timedelta(days=1)
                    self.log_fn(f"Lặp lại: {self.target_dt.strftime('%H:%M %d/%m')}", "INFO")
                    warned_10 = False
                    time.sleep(5); continue
                else:
                    self.after(0, self._stop_timer); break

            if remaining <= 10 and not warned_10:
                self.log_fn(f"Còn {int(remaining)} giây!", "WARN")
                warned_10 = True

            h = int(remaining // 3600)
            m = int((remaining % 3600) // 60)
            s = int(remaining % 60)
            self.countdown_var.set(f"{h:02d}:{m:02d}:{s:02d}")

            if remaining < 10:
                clr = "#ef4444" if int(remaining) % 2 == 0 else "#f59e0b"
                self.after(0, lambda c=clr: self.dot_indicator.configure(text_color=c))

            time.sleep(0.5)

    def _fire(self):
        self.log_fn("=" * 36, "SEP")
        self.log_fn("ĐẾN GIỜ! ĐANG GỬI PROMPT QUA WIN32...", "SEND")
        self.dot_indicator.configure(text_color="#22c55e")
        prompt = self.prompt_text.get("1.0", "end").strip()
        try:
            ok = send_prompt_to_claude_gui(prompt, self.press_enter.get(), self.log_fn)
            if ok:
                self.log_fn("HOÀN THÀNH: Gửi prompt thành công!", "SUCCESS")
            else:
                self.log_fn("THẤT BẠI: Không gửi được prompt.", "ERROR")
        except Exception as e:
            self.log_fn(f"Exception: {e}", "ERROR")
        self.log_fn("=" * 36, "SEP")

    def _test_send(self):
        prompt = self.prompt_text.get("1.0", "end").strip()
        if not prompt:
            messagebox.showwarning("Thiếu", "Nhập prompt trước!"); return
        self.log_fn("-" * 36, "SEP")
        self.log_fn("TEST GỬI NGAY (WIN32)...", "SEND")
        threading.Thread(target=send_prompt_to_claude_gui,
            args=(prompt, self.press_enter.get()),
            kwargs={"status_cb": self.log_fn}, daemon=True).start()

    def _find_windows(self):
        self.log_fn("Tìm kiếm cửa sổ Claude Code...", "INFO")
        wins = find_claude_windows()
        if wins:
            self.log_fn(f"Tìm thấy {len(wins)} cửa sổ:", "SUCCESS")
            for hwnd, title in wins:
                self.log_fn(f"  [{hwnd}] {title}", "SUCCESS")
        else:
            self.log_fn("Không tìm thấy cửa sổ nào!", "ERROR")

    def _tick(self):
        if not self.is_running:
            self.countdown_var.set(datetime.datetime.now().strftime("%H:%M:%S"))
        self.after(1000, self._tick)
