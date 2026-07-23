"""
workspace_config.py — Quản lý lưu trữ Cấu hình Workspace & Lịch sử Thư mục Gần đây (Open Recently).
Lưu dữ liệu vào file workspace_config.json để tự động khôi phục thư mục làm việc sau khi khởi động lại ứng dụng.
"""

import os
import json
from typing import List


def _get_config_file_path() -> str:
    base = r"e:\exe" if os.path.exists(r"e:\exe") else os.path.expanduser("~\\.claude_suite")
    os.makedirs(base, exist_ok=True)
    return os.path.join(base, "workspace_config.json")


class WorkspaceConfigStore:
    def __init__(self):
        self.config_path = _get_config_file_path()
        self.last_workspace_folder: str = ""
        self.recent_workspaces: List[str] = []
        self.load()

    def load(self):
        if os.path.exists(self.config_path):
            try:
                with open(self.config_path, "r", encoding="utf-8") as f:
                    data = json.load(f)
                    self.last_workspace_folder = data.get("last_workspace_folder", "")
                    self.recent_workspaces = data.get("recent_workspaces", [])
            except Exception:
                pass

        # Filter out paths that no longer exist on disk
        valid_recents = []
        for path in self.recent_workspaces:
            norm = os.path.normpath(path)
            if os.path.exists(norm) and norm not in valid_recents:
                valid_recents.append(norm)
        self.recent_workspaces = valid_recents

        if self.last_workspace_folder and not os.path.exists(self.last_workspace_folder):
            self.last_workspace_folder = ""

    def save(self):
        try:
            with open(self.config_path, "w", encoding="utf-8") as f:
                json.dump({
                    "last_workspace_folder": self.last_workspace_folder,
                    "recent_workspaces": self.recent_workspaces
                }, f, indent=2, ensure_ascii=False)
        except Exception:
            pass

    def add_workspace(self, folder_path: str):
        if not folder_path or not os.path.exists(folder_path):
            return
        folder_path = os.path.normpath(folder_path)
        self.last_workspace_folder = folder_path

        if folder_path in self.recent_workspaces:
            self.recent_workspaces.remove(folder_path)

        self.recent_workspaces.insert(0, folder_path)
        self.recent_workspaces = self.recent_workspaces[:10]  # Max 10 recent folders
        self.save()

    def clear_recents(self):
        self.recent_workspaces = []
        self.save()
