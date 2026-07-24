package pipeline

import (
	"context"
	"fmt"

	"claude_suite/backend/cli"
	"claude_suite/backend/database"
	"claude_suite/backend/models"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type PipelineEngine struct {
	agentRepo *database.AgentRepository
	cliRunner cli.CLIRunner
	ctx       context.Context
}

func NewPipelineEngine(agentRepo *database.AgentRepository, cliRunner cli.CLIRunner) *PipelineEngine {
	return &PipelineEngine{
		agentRepo: agentRepo,
		cliRunner: cliRunner,
	}
}

func (pe *PipelineEngine) SetContext(ctx context.Context) {
	pe.ctx = ctx
}

func (pe *PipelineEngine) GetDefaultSteps() []models.PipelineStep {
	return []models.PipelineStep{
		{StepID: 1, StageName: "Planner & Architect", Role: "Business Analyst (BA)", Status: "pending", Prompt: "Define high level system goals and user stories."},
		{StepID: 2, StageName: "System Architect", Role: "Chief Architect & Tech Lead", Status: "pending", Prompt: "Design C4 component diagrams, DB schemas, and file structure."},
		{StepID: 3, StageName: "Software Engineer", Role: "Senior Fullstack Developer", Status: "pending", Prompt: "Write clean, modular, production-ready code implementation."},
		{StepID: 4, StageName: "Security Auditor", Role: "Senior Code Reviewer & Security Auditor", Status: "pending", Prompt: "Review code against OWASP Top 10 and suggest refactorings."},
		{StepID: 5, StageName: "QA Specialist", Role: "Lead QA & Test Specialist", Status: "pending", Prompt: "Write automated tests and verify execution correctness."},
	}
}

func (pe *PipelineEngine) RunPipeline(steps []models.PipelineStep, cwd string) ([]models.PipelineStep, error) {
	agents, err := pe.agentRepo.GetAll()
	if err != nil || len(agents) == 0 {
		return steps, fmt.Errorf("no agents available")
	}

	cumulativeContext := ""

	for i := range steps {
		step := &steps[i]
		step.Status = "running"
		pe.emitStepEvent(step)

		// Find matching agent for role
		var agent *models.Agent
		for j, a := range agents {
			if a.Name == step.Role || a.Role == step.Role {
				agent = &agents[j]
				break
			}
		}
		if agent == nil {
			agent = &agents[0]
		}

		fullPrompt := fmt.Sprintf("%s\n\n### PREVIOUS STAGE DELIVERABLES ###\n%s\n\n### YOUR STAGE TASK ###\n%s", cumulativeContext, cumulativeContext, step.Prompt)

		onLog := func(msg, level string) {
			if pe.ctx != nil {
				runtime.EventsEmit(pe.ctx, "pipeline_log", map[string]string{
					"step_id": fmt.Sprintf("%d", step.StepID),
					"message": msg,
					"level":   level,
				})
			}
		}

		result := pe.cliRunner.RunAgent(agent, fullPrompt, onLog, cwd)

		if result.Success {
			step.Status = "completed"
			step.Output = result.Output
			step.DurationSec = result.DurationSec
			cumulativeContext += fmt.Sprintf("\n--- DELIVERABLE FROM STAGE %d: %s ---\n%s\n", step.StepID, step.StageName, result.Output)
		} else {
			step.Status = "error"
			step.Output = result.Error
			pe.emitStepEvent(step)
			return steps, fmt.Errorf("pipeline failed at step %d: %s", step.StepID, result.Error)
		}

		pe.emitStepEvent(step)
	}

	return steps, nil
}

func (pe *PipelineEngine) emitStepEvent(step *models.PipelineStep) {
	if pe.ctx != nil {
		runtime.EventsEmit(pe.ctx, "pipeline_step_updated", step)
	}
}
