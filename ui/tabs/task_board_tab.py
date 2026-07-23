import customtkinter as ctk

class TaskBoardTabFrame(ctk.CTkFrame):
    """
    Không gian làm việc thứ 2: Task Board & Planning.
    Gộp chung Kanban Board và AI Plan Builder.
    """
    def __init__(self, master, task_board, plan_builder, **kwargs):
        super().__init__(master, fg_color="transparent", **kwargs)
        self.task_board = task_board
        self.plan_builder = plan_builder
        self._build_ui()

    def _build_ui(self):
        banner = ctk.CTkFrame(self, fg_color=("#8b5cf6", "#7c3aed"), corner_radius=8, height=40)
        banner.pack(fill="x", padx=10, pady=(5, 10))
        banner.pack_propagate(False)
        ctk.CTkLabel(
            banner, text="📋 Task Board & Planning — Quản lý tác vụ", 
            font=ctk.CTkFont(size=14, weight="bold"), text_color="white"
        ).pack(side="left", padx=15, pady=5)

        # Tabs phụ bên trong (Segmented Button hoặc internal tabs)
        self.internal_tab = ctk.CTkTabview(self, corner_radius=8)
        self.internal_tab.pack(fill="both", expand=True, padx=10, pady=5)
        
        self.tab_kanban = self.internal_tab.add("Interactive Kanban")
        self.tab_plan = self.internal_tab.add("AI Plan Builder")
        self.tab_report = self.internal_tab.add("Project Reports")


