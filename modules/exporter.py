"""
exporter.py — Xuất Báo Cáo Chuyên Nghiệp (Markdown / HTML Report Exporter)
Cho phép xuất kết quả Kanban, AI Plan và Agent Memory ra file Markdown & HTML có định dạng đẹp mắt.
"""

import os
import datetime
import html
from typing import List, Dict
from modules.task_board import Task
from modules.agent_registry import Agent
from modules.pipeline_engine import PipelineStep


class ReportExporter:
    """
    Xuất báo cáo dự án & kết quả Agent ra file Markdown và HTML.
    """

    @staticmethod
    def export_kanban_report(tasks: List[Task], output_dir: str = None) -> Dict[str, str]:
        """Xuất báo cáo Bảng Kanban ra Markdown & HTML."""
        if not output_dir:
            output_dir = os.path.join(os.path.expanduser("~"), "Desktop")

        os.makedirs(output_dir, exist_ok=True)

        ts = datetime.datetime.now().strftime("%Y%m%d_%H%M%S")
        md_file = os.path.join(output_dir, f"ClaudeSuite_Kanban_Report_{ts}.md")
        html_file = os.path.join(output_dir, f"ClaudeSuite_Kanban_Report_{ts}.html")

        # Build Markdown content
        md_lines = []
        md_lines.append("# 📊 BÁO CÁO TIẾN ĐỘ DỰ ÁN — CLAUDE SUITE")
        md_lines.append(f"**Thời gian xuất:** {datetime.datetime.now().strftime('%d/%m/%Y %H:%M:%S')}\n")

        md_lines.append("## 📈 TỔNG QUAN CÁC TASK")
        status_counts = {}
        for t in tasks:
            status_counts[t.status] = status_counts.get(t.status, 0) + 1

        md_lines.append("| Trạng thái | Số lượng Task |")
        md_lines.append("|---|---|")
        for s, cnt in status_counts.items():
            md_lines.append(f"| `{s.upper()}` | **{cnt}** |")
        md_lines.append("\n---\n")

        md_lines.append("## 📋 CHI TIẾT DANH SÁCH TASK")
        for i, t in enumerate(tasks, 1):
            md_lines.append(f"### [{i}] [{t.status_icon} {t.status.upper()}] {t.title}")
            md_lines.append(f"- **Issue Key:** `TSK-{t.task_id[:4].upper()}`")
            md_lines.append(f"- **Độ ưu tiên:** `{t.priority.upper()}`")
            md_lines.append(f"- **Phụ thuộc:** `{t.depends_on}`")
            if t.result:
                md_lines.append("\n#### 📄 Kết quả thực thi:")
                md_lines.append("```text")
                md_lines.append(t.result[:1000] + ("..." if len(t.result) > 1000 else ""))
                md_lines.append("```")
            md_lines.append("\n---\n")

        md_content = "\n".join(md_lines)

        with open(md_file, "w", encoding="utf-8") as f:
            f.write(md_content)

        safe_md = html.escape(md_content)
        html_content = f"""<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>Claude Suite Kanban Report</title>
<style>
  body {{ font-family: 'Segoe UI', system-ui, sans-serif; background: #0f172a; color: #f8fafc; padding: 30px; line-height: 1.6; }}
  h1 {{ color: #38bdf8; border-bottom: 2px solid #334155; padding-bottom: 10px; }}
  h3 {{ color: #facc15; margin-top: 20px; }}
  table {{ border-collapse: collapse; width: 100%; max-width: 500px; margin: 15px 0; }}
  th, td {{ border: 1px solid #334155; padding: 8px 12px; text-align: left; }}
  th {{ background: #1e293b; color: #38bdf8; }}
  pre {{ background: #1e293b; padding: 15px; border-radius: 8px; overflow-x: auto; color: #4ade80; font-family: 'Consolas', monospace; }}
  .badge {{ background: #0284c7; color: white; padding: 2px 8px; border-radius: 12px; font-size: 0.85em; font-weight: bold; }}
</style>
</head>
<body>
  <h1>📊 BÁO CÁO TIẾN ĐỘ DỰ ÁN — CLAUDE SUITE</h1>
  <p><strong>Thời gian xuất:</strong> {datetime.datetime.now().strftime('%d/%m/%Y %H:%M:%S')}</p>
  <pre>{safe_md}</pre>
</body>
</html>"""

        with open(html_file, "w", encoding="utf-8") as f:
            f.write(html_content)

        return {"md": md_file, "html": html_file}

    @staticmethod
    def export_pipeline_report(goal: str, steps: List[PipelineStep], output_dir: str = None) -> Dict[str, str]:
        """Xuất báo cáo Multi-Agent Pipeline ra Markdown & HTML."""
        if not output_dir:
            output_dir = os.path.join(os.path.expanduser("~"), "Desktop")

        ts = datetime.datetime.now().strftime("%Y%m%d_%H%M%S")
        md_file = os.path.join(output_dir, f"ClaudeSuite_Pipeline_Report_{ts}.md")

        md_lines = []
        md_lines.append("# ⚡ BÁO CÁO QUY TRÌNH MULTI-AGENT PIPELINE")
        md_lines.append(f"**Mục tiêu dự án:** {goal}")
        md_lines.append(f"**Thời gian thực hiện:** {datetime.datetime.now().strftime('%d/%m/%Y %H:%M:%S')}\n")

        for idx, step in enumerate(steps, 1):
            md_lines.append(f"## 📌 BƯỚC {idx}: {step.role_name.upper()} [{step.status.upper()}]")
            md_lines.append(f"**Nhiệm vụ:** {step.prompt_template[:150]}...")
            if step.output:
                md_lines.append("\n```text")
                md_lines.append(step.output)
                md_lines.append("```\n")
            md_lines.append("---\n")

        md_content = "\n".join(md_lines)

        with open(md_file, "w", encoding="utf-8") as f:
            f.write(md_content)

        return {"md": md_file}
