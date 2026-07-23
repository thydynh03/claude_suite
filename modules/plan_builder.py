"""
plan_builder.py — AI-assisted plan generation + subtask decompose
"""

import json
import re
from dataclasses import dataclass, field
from typing import List, Optional, Callable

from modules.task_board import Task, PRIORITIES


from modules.models import PlanTask


class PlanBuilder:
    """
    Dùng Claude CLI để decompose 1 project lớn thành tasks.
    Fallback về simple parser nếu CLI fail.
    """

    DECOMPOSE_PROMPT = """You are a project planning expert.
Break down the following project/goal into specific, actionable tasks.

Project: {project}

Return ONLY valid JSON (no markdown, no explanation):
{{
  "tasks": [
    {{
      "title": "Short task title",
      "description": "What needs to be done",
      "prompt": "Exact prompt to send to an AI agent to complete this task",
      "priority": "normal",
      "depends_on": [],
      "model_hint": "claude-opus-4-8"
    }}
  ]
}}

Rules:
- 3-8 tasks, specific and actionable
- depends_on: list of 0-based indices of tasks this depends on
- model_hint: "claude-opus-4-8" for complex, "claude-opus-4-8" for medium, "claude-opus-4-8" for simple
- priority: "low" | "normal" | "high" | "urgent"
"""

    def __init__(self, cli=None):
        self.cli = cli   # ClaudeCLI instance (optional)

    def build_from_description(
        self,
        project_description: str,
        on_log: Optional[Callable] = None
    ) -> List[PlanTask]:
        log = on_log or (lambda m, lvl="INFO": print(f"[{lvl}] {m}"))
        log(f"Dang decompose project: '{project_description[:60]}'...", "INFO")

        if self.cli:
            return self._build_via_cli(project_description, log)
        else:
            log("CLI khong kha dung, dung simple decompose...", "WARN")
            return self._simple_decompose(project_description)

    def _build_via_cli(self, project: str, log: Callable) -> List[PlanTask]:
        prompt = self.DECOMPOSE_PROMPT.format(project=project)
        result = self.cli.run_once(
            prompt, model="claude-opus-4-8",
            output_format="json", on_log=log
        )

        if not result.success:
            log(f"CLI fail: {result.error}, dung fallback", "WARN")
            return self._simple_decompose(project)

        try:
            # result.output có thể là JSON string hoặc đã parse
            text = result.output
            # Tìm JSON block nếu có
            match = re.search(r'\{.*\}', text, re.DOTALL)
            if match:
                text = match.group(0)
            data = json.loads(text)
            tasks_raw = data.get("tasks", [])
            tasks = []
            for i, t in enumerate(tasks_raw):
                # Convert depends_on từ indices sang task titles (sẽ resolve sau)
                deps = [str(d) for d in t.get("depends_on", [])]
                tasks.append(PlanTask(
                    title=t.get("title", f"Task {i+1}"),
                    description=t.get("description", ""),
                    prompt=t.get("prompt", t.get("description", "")),
                    priority=t.get("priority", "normal"),
                    depends_on=deps,
                    model_hint=t.get("model_hint", "claude-opus-4-8"),
                ))
            log(f"Da tao {len(tasks)} tasks tu AI!", "SUCCESS")
            return tasks
        except Exception as e:
            log(f"Parse JSON fail: {e}, dung fallback", "WARN")
            return self._simple_decompose(project)

    def _simple_decompose(self, project: str) -> List[PlanTask]:
        """Fallback: tạo template tasks khi CLI không available."""
        return [
            PlanTask(
                title="Phan tich yeu cau",
                description=f"Phan tich va hieu ro yeu cau: {project}",
                prompt=f"Analyze and understand the requirements for: {project}. List key requirements and constraints.",
                priority="high",
                model_hint="claude-opus-4-8",
            ),
            PlanTask(
                title="Lap ke hoach thuc hien",
                description="Tao ke hoach chi tiet de thuc hien",
                prompt=f"Create a detailed execution plan for: {project}",
                priority="high",
                depends_on=["0"],
                model_hint="claude-opus-4-8",
            ),
            PlanTask(
                title="Thuc hien Phase 1",
                description="Bat dau thuc hien phan chinh",
                prompt=f"Execute phase 1 of: {project}",
                priority="normal",
                depends_on=["1"],
                model_hint="claude-opus-4-8",
            ),
            PlanTask(
                title="Review va hoan thien",
                description="Kiem tra lai va sua chua",
                prompt=f"Review and finalize the implementation of: {project}",
                priority="normal",
                depends_on=["2"],
                model_hint="claude-opus-4-8",
            ),
        ]

    def plan_to_tasks(self, plan_tasks: List[PlanTask], parent_id: str = None) -> List[Task]:
        """Convert PlanTask list → Task list (ready to add to TaskBoard)."""
        from modules.task_board import Task
        import json

        task_objects = []
        id_map = {}  # index → task_id

        for i, pt in enumerate(plan_tasks):
            t = Task(
                title=pt.title,
                description=pt.description,
                prompt=pt.prompt,
                priority=pt.priority,
                parent_id=parent_id,
                status="backlog",
            )
            id_map[str(i)] = t.task_id
            task_objects.append((t, pt.depends_on))

        # Resolve depends_on indices → task_ids
        result = []
        for task, deps in task_objects:
            resolved = [id_map[d] for d in deps if d in id_map]
            task.depends_on = json.dumps(resolved)
            result.append(task)

        return result
