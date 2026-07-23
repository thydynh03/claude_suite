"""
pipeline_engine.py — Multi-Agent Sequential Workflow (Agent Swarm)
Tự động phối hợp nối tiếp giữa các Agent theo luồng:
Planner ➔ Architect ➔ Coder ➔ Reviewer ➔ Tester
Kết quả của bước trước làm Context tự động cho bước sau.
"""

import threading
import datetime
import time
from typing import List, Dict, Callable, Optional
from dataclasses import dataclass, field

from modules.agent_registry import AgentRegistry, Agent
from modules.claude_cli import ClaudeCLI, RunResult
from modules.memory_store import MemoryStore, MemoryItem


from modules.models import PipelineStep


class AgentPipeline:
    """
    Quản lý chuỗi công việc nối tiếp giữa nhiều Agents.
    """

    def __init__(self, registry: AgentRegistry, cli: ClaudeCLI,
                 memory: MemoryStore, on_log: Optional[Callable] = None,
                 get_context_fn: Optional[Callable[[], str]] = None):
        self.registry = registry
        self.cli      = cli
        self.memory   = memory
        self.on_log   = on_log or (lambda msg, lvl="INFO": print(f"[{lvl}] {msg}"))
        self.get_context_fn = get_context_fn

    def create_default_software_pipeline(self, initial_goal: str) -> List[PipelineStep]:
        """Tạo Pipeline 5 bước tiêu chuẩn cho phát triển phần mềm."""
        return [
            PipelineStep(
                step_id="step_1_planner",
                role_name="Planner & Architect",
                prompt_template=f"Mục tiêu dự án: {initial_goal}\n\nHãy phân rã dự án thành cây kế hoạch kỹ thuật chi tiết và danh sách task nguyên tử."
            ),
            PipelineStep(
                step_id="step_2_architect",
                role_name="System Architect",
                prompt_template="Dựa trên kế hoạch của Planner ở trên, hãy thiết kế chi tiết cấu trúc File/Folder, API Schemas và Data Models."
            ),
            PipelineStep(
                step_id="step_3_coder",
                role_name="Staff Software Engineer",
                prompt_template="Dựa trên thiết kế kiến trúc ở trên, hãy viết code thực thi hoàn chỉnh, production-ready cho các module chính."
            ),
            PipelineStep(
                step_id="step_4_reviewer",
                role_name="Security & Code Reviewer",
                prompt_template="Hãy review chi tiết toàn bộ đoạn code vừa được tạo ra ở trên. Kiểm tra lỗ hổng an ninh (OWASP), hiệu năng và đưa ra bản code đã refactor chuẩn."
            ),
            PipelineStep(
                step_id="step_5_tester",
                role_name="QA & Test Engineer",
                prompt_template="Dựa trên code đã được review ở trên, hãy viết bộ kiểm thử tự động (PyTest / Jest) phủ 100% happy path và edge cases."
            )
        ]

    def execute_pipeline_async(
        self,
        steps: List[PipelineStep],
        initial_goal: str,
        on_step_change: Optional[Callable[[int, PipelineStep], None]] = None,
        on_pipeline_done: Optional[Callable[[bool, List[PipelineStep]], None]] = None
    ):
        def _run():
            ws_ctx = self.get_context_fn() if self.get_context_fn else ""
            accumulated_context = f"{ws_ctx}\n\n🎯 MỤC TIÊU DỰ ÁN BAN ĐẦU: {initial_goal}\n\n" if ws_ctx else f"🎯 MỤC TIÊU DỰ ÁN BAN ĐẦU: {initial_goal}\n\n"
            agents = self.registry.list_all()

            success_all = True

            for idx, step in enumerate(steps):
                step.status = "running"
                if on_step_change:
                    on_step_change(idx, step)

                self.on_log(f"⚡ PIPELINE [{idx+1}/{len(steps)}]: Bắt đầu vai trò '{step.role_name}'...", "SEND")

                # Find best matching agent for this role
                matched_agent = self._find_agent_for_role(step.role_name, agents)
                if matched_agent:
                    step.agent_id = matched_agent.agent_id
                    self.registry.update_status(matched_agent.agent_id, "running")

                # Prepare full prompt with accumulated context
                full_prompt = f"{accumulated_context}\n\n=========================================\n📌 NHIỆM VỤ HIỆN TẠI ({step.role_name.upper()}):\n{step.prompt_template}\n========================================="

                # Run Claude CLI
                if matched_agent:
                    result = self.cli.run_agent(matched_agent, full_prompt, on_log=self.on_log)
                    self.registry.update_status(matched_agent.agent_id, "idle", tasks_done_delta=1 if result.success else 0)
                else:
                    result = self.cli.run_once(full_prompt, model="claude-opus-4-8", on_log=self.on_log)

                if result.success:
                    step.status = "completed"
                    step.output = result.output
                    self.on_log(f"✅ PIPELINE Step {idx+1} ({step.role_name}) hoàn thành ({result.duration_s:.1f}s)", "SUCCESS")

                    # Record SQLite memory
                    self.memory.add(MemoryItem(
                        agent_id=matched_agent.agent_id if matched_agent else "pipeline",
                        role="assistant", content=f"[Pipeline Step {idx+1}: {step.role_name}]\n{result.output}"
                    ))

                    # Accumulate output into context for NEXT agent step!
                    accumulated_context += f"\n\n=========================================\n📄 KẾT QUẢ BƯỚC {idx+1} ({step.role_name.upper()}):\n{result.output}\n=========================================\n"
                else:
                    step.status = "failed"
                    step.output = f"Lỗi: {result.error}"
                    self.on_log(f"❌ PIPELINE Step {idx+1} ({step.role_name}) thất bại: {result.error[:80]}", "ERROR")
                    success_all = False
                    if on_step_change:
                        on_step_change(idx, step)
                    break

                if on_step_change:
                    on_step_change(idx, step)

            if on_pipeline_done:
                on_pipeline_done(success_all, steps)

        threading.Thread(target=_run, daemon=True).start()

    def _find_agent_for_role(self, role_name: str, agents: List[Agent]) -> Optional[Agent]:
        role_lower = role_name.lower()
        for a in agents:
            if any(k in a.name.lower() for k in role_lower.split()):
                return a
        return agents[0] if agents else None
