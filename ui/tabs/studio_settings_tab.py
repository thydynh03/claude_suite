import customtkinter as ctk

class StudioSettingsTabFrame(ctk.CTkFrame):
    """
    Không gian làm việc thứ 3: Studio & Settings.
    Gộp chung Agent Registry, Win32 Scheduler, Quick CLI và Live System Log.
    """
    def __init__(self, master, registry, cli, **kwargs):
        super().__init__(master, fg_color="transparent", **kwargs)
        self.registry = registry
        self.cli = cli
        self._build_ui()

    def _build_ui(self):
        banner = ctk.CTkFrame(self, fg_color=("#475569", "#334155"), corner_radius=8, height=40)
        banner.pack(fill="x", padx=10, pady=(5, 10))
        banner.pack_propagate(False)
        ctk.CTkLabel(
            banner, text="⚙️ Studio & Settings — Cài đặt hệ thống", 
            font=ctk.CTkFont(size=14, weight="bold"), text_color="white"
        ).pack(side="left", padx=15, pady=5)

        self.internal_tab = ctk.CTkTabview(self, corner_radius=8)
        self.internal_tab.pack(fill="both", expand=True, padx=10, pady=5)
        
        self.tab_agents = self.internal_tab.add("Agents Registry")
        self.tab_pipeline = self.internal_tab.add("Multi-Agent Pipeline")
        self.tab_scheduler = self.internal_tab.add("Win32 Scheduler")
        self.tab_cli = self.internal_tab.add("Quick CLI")
        self.tab_log = self.internal_tab.add("Live System Log")
        self.tab_updates = self.internal_tab.add("Info & Updates")

        self._build_updates_tab()

    def _build_updates_tab(self):
        import sys
        from modules.version import CURRENT_VERSION
        
        frame = ctk.CTkFrame(self.tab_updates, fg_color="transparent")
        frame.pack(fill="both", expand=True, padx=20, pady=20)
        
        ctk.CTkLabel(frame, text="Claude Suite - Control Center", font=ctk.CTkFont(size=20, weight="bold")).pack(pady=(10, 5))
        ctk.CTkLabel(frame, text=f"Phiên bản hiện tại: {CURRENT_VERSION}", font=ctk.CTkFont(size=14)).pack(pady=(0, 20))
        
        self.update_status_lbl = ctk.CTkLabel(frame, text="", font=ctk.CTkFont(size=12), text_color="gray60")
        self.update_status_lbl.pack(pady=10)
        
        self.update_progress = ctk.CTkProgressBar(frame, width=300)
        self.update_progress.set(0)
        
        self.btn_check_update = ctk.CTkButton(
            frame, text="🔄 Kiểm tra bản cập nhật", font=ctk.CTkFont(weight="bold"),
            command=self._on_check_update
        )
        self.btn_check_update.pack(pady=10)
        
        if not getattr(sys, 'frozen', False):
            ctk.CTkLabel(frame, text="(Bạn đang chạy từ mã nguồn. Dùng 'git pull' để cập nhật thay vì nút này)", text_color="orange").pack(pady=5)

    def _on_check_update(self):
        from modules.version import CURRENT_VERSION
        from modules.updater import AutoUpdater
        import threading
        import sys
        
        self.btn_check_update.configure(state="disabled", text="Đang kiểm tra...")
        self.update_status_lbl.configure(text="Đang kết nối tới GitHub...", text_color="gray60")
        
        updater = AutoUpdater(CURRENT_VERSION)
        
        def _check():
            result = updater.check_for_updates()
            self.after(0, self._on_check_complete, result, updater)
            
        threading.Thread(target=_check, daemon=True).start()
        
    def _on_check_complete(self, result, updater):
        if result.get("has_update"):
            self.update_status_lbl.configure(text=f"Có phiên bản mới: {result['version']}\nĐang chuẩn bị tải...", text_color="#10b981")
            self.update_progress.pack(pady=10)
            self.update_progress.set(0)
            
            def _on_progress(downloaded, total):
                if total > 0:
                    percent = downloaded / total
                    self.after(0, self.update_progress.set, percent)
                    self.after(0, lambda: self.update_status_lbl.configure(text=f"Đang tải: {downloaded/1024/1024:.1f}MB / {total/1024/1024:.1f}MB ({int(percent*100)}%)"))
                    
            def _on_complete(success, msg):
                def _ui_update():
                    self.update_status_lbl.configure(text=msg, text_color="#10b981" if success else "#ef4444")
                    self.btn_check_update.configure(text="Hoàn tất" if success else "Thử lại", state="normal")
                self.after(0, _ui_update)
                
            updater.download_and_install(result['download_url'], _on_progress, _on_complete)
        else:
            self.update_status_lbl.configure(text="Bạn đang ở phiên bản mới nhất!", text_color="#10b981")
            self.btn_check_update.configure(text="Đã cập nhật", state="normal")


