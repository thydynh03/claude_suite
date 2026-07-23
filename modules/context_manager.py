"""
context_manager.py — Quản lý đọc Context từ File & Thư mục trên Máy tính (Desktop/Projects)
Cho phép AI Agent đọc toàn bộ nội dung file & cấu trúc thư mục để làm bối cảnh xử lý.
"""

import os
import glob
from typing import List, Dict

# Các đuôi file văn bản / source code / config / docs được hỗ trợ đọc nội dung
TEXT_EXTENSIONS = {
    # Programming Languages
    ".py", ".pyw", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".java", ".kt", ".kts",
    ".rs", ".go", ".c", ".cpp", ".cc", ".cxx", ".h", ".hpp", ".cs", ".php", ".rb",
    ".swift", ".m", ".mm", ".scala", ".groovy", ".lua", ".r", ".pl", ".pm", ".dart",
    ".ex", ".exs", ".erl", ".hrl", ".hs", ".lhs", ".clj", ".cljs", ".elm", ".fs",
    ".fsi", ".fsx", ".pas", ".asm", ".s", ".nim", ".zig", ".v", ".odin", ".sol",
    
    # Web & Stylesheets
    ".html", ".htm", ".xhtml", ".vue", ".svelte", ".astro", ".css", ".scss", ".sass",
    ".less", ".styl", ".svg",
    
    # Data & Configuration
    ".json", ".json5", ".jsonc", ".yaml", ".yml", ".xml", ".toml", ".ini", ".conf",
    ".config", ".env", ".properties", ".plist", ".gradle", ".lock", ".editorconfig",
    
    # Scripts & Shells
    ".sh", ".bash", ".zsh", ".fish", ".bat", ".cmd", ".ps1", ".psm1", ".vbs",
    
    # Database & Queries
    ".sql", ".prisma", ".graphql", ".gql",
    
    # Documentation & Data
    ".md", ".markdown", ".mdx", ".txt", ".rst", ".tex", ".adoc", ".org", ".csv",
    ".tsv", ".log", ".jsonlines", ".jsonl",
    
    # DevOps & Build Tools
    ".dockerfile", ".spec"
}

IGNORE_DIRS = {".git", "node_modules", "__pycache__", ".venv", "venv", "dist", "build"}


class ContextManager:
    """
    Trích xuất và đóng gói nội dung File/Folder làm Context cho AI Agents.
    """

    @staticmethod
    def read_file_content(file_path: str, max_size_kb: int = 500) -> str:
        if not os.path.exists(file_path):
            return f"[File not found: {file_path}]"

        # Check file size
        size_kb = os.path.getsize(file_path) / 1024
        if size_kb > max_size_kb:
            return f"[File too large ({size_kb:.1f}KB > {max_size_kb}KB): {os.path.basename(file_path)}]"

        try:
            with open(file_path, "r", encoding="utf-8", errors="replace") as f:
                return f.read()
        except Exception as e:
            return f"[Error reading file {os.path.basename(file_path)}: {e}]"

    @classmethod
    def is_text_file(cls, file_path: str) -> bool:
        ext = os.path.splitext(file_path)[1].lower()
        if ext in TEXT_EXTENSIONS:
            return True
        basename = os.path.basename(file_path).lower()
        if basename in {"dockerfile", "makefile", "license", "readme", "procfile", "gemfile", "pipfile", "jenkinsfile", "cmakelists.txt"}:
            return True
        return False

    @classmethod
    def scan_folder(cls, folder_path: str, max_files: int = 40) -> List[str]:
        """Quét danh sách các file trong thư mục (loại bỏ node_modules, .git...)"""
        matched_files = []
        if not os.path.exists(folder_path):
            return []

        for root, dirs, files in os.walk(folder_path):
            # Prune ignored directories
            dirs[:] = [d for d in dirs if d not in IGNORE_DIRS]

            for file in files:
                full_path = os.path.join(root, file)
                if cls.is_text_file(full_path):
                    matched_files.append(full_path)
                    if len(matched_files) >= max_files:
                        break
            if len(matched_files) >= max_files:
                break

        return matched_files

    @classmethod
    def build_context_prompt(cls, selected_paths: List[str]) -> str:
        """
        Tạo khối Markdown Context từ danh sách đường dẫn File hoặc Folder.
        """
        if not selected_paths:
            return ""

        context_blocks = []
        all_files = []

        for path in selected_paths:
            path = os.path.abspath(path)
            if os.path.isfile(path):
                all_files.append(path)
            elif os.path.isdir(path):
                folder_files = cls.scan_folder(path)
                all_files.extend(folder_files)

        # De-duplicate files
        all_files = list(dict.fromkeys(all_files))

        if not all_files:
            return ""

        context_blocks.append("═══════════════════════════════════════════════════════════════")
        context_blocks.append("📁 ATTACHED WORKSPACE CONTEXT (CUNG CẤP BỞI NGƯỜI DÙNG):")
        context_blocks.append("═══════════════════════════════════════════════════════════════\n")

        for fpath in all_files[:25]: # Max 25 files
            rel_name = os.path.basename(fpath)
            ext = os.path.splitext(fpath)[1].lstrip(".")
            content = cls.read_file_content(fpath)
            context_blocks.append(f"📄 FILE: {rel_name} ({fpath})")
            context_blocks.append(f"```{ext}\n{content}\n```\n")

        context_blocks.append("═══════════════════════════════════════════════════════════════\n")
        return "\n".join(context_blocks)
