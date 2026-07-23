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


