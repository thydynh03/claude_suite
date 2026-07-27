package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent_center/backend/models"
	"agent_center/backend/textutil"
)

type ExporterService struct{}

func NewExporterService() *ExporterService {
	return &ExporterService{}
}

func (e *ExporterService) ExportKanbanReport(tasks []models.Task, outDir string) (string, string, error) {
	ts := time.Now().Format("20060102_150405")
	if outDir == "" {
		outDir = "."
	}

	mdFile := filepath.Join(outDir, fmt.Sprintf("AgentCenter_Kanban_Report_%s.md", ts))
	htmlFile := filepath.Join(outDir, fmt.Sprintf("AgentCenter_Kanban_Report_%s.html", ts))

	var md strings.Builder
	md.WriteString("# 📋 Agent Center — Kanban Report\n\n")
	md.WriteString(fmt.Sprintf("**Thời gian xuất:** %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	statuses := []string{"backlog", "queued", "running", "done", "failed"}
	for _, status := range statuses {
		md.WriteString(fmt.Sprintf("## %s\n", strings.ToUpper(status)))
		count := 0
		for _, t := range tasks {
			if t.Status == status {
				count++
				// textutil.Truncate, not t.TaskID[:8]: a task whose ID is shorter
				// than 8 bytes panics the export and takes the whole app with it.
				// Nothing guarantees the length — IDs come from the AI planner and
				// from imports as well as from the generator here.
				md.WriteString(fmt.Sprintf("- **[%s]** %s\n  - Priority: `%s` | Assigned: `%s`\n  - Prompt: %s\n", textutil.Truncate(t.TaskID, 8, ""), t.Title, t.Priority, t.AssignedTo, t.Prompt))
				if t.Result != "" {
					md.WriteString(fmt.Sprintf("  - Result: %s\n", t.Result))
				}
			}
		}
		if count == 0 {
			md.WriteString("*(Không có task nào)*\n")
		}
		md.WriteString("\n")
	}

	if err := os.WriteFile(mdFile, []byte(md.String()), 0644); err != nil {
		return "", "", err
	}

	// HTML version
	htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>Agent Center Kanban Report</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; margin: 40px; background: #f8fafc; color: #0f172a; }
h1 { color: #2563eb; }
.card { background: white; padding: 16px; border-radius: 8px; margin-bottom: 12px; border: 1px solid #e2e8f0; }
.status-header { text-transform: uppercase; font-weight: bold; margin-top: 24px; color: #475569; }
</style>
</head>
<body>
<pre>%s</pre>
</body>
</html>`, md.String())

	// The HTML copy used to be written with the error discarded, so a full disk
	// or a read-only folder produced a report the UI happily announced and the
	// user could not open. The markdown is already on disk at this point, so the
	// path is still returned — the caller decides how loudly to complain.
	if err := os.WriteFile(htmlFile, []byte(htmlContent), 0644); err != nil {
		return mdFile, "", fmt.Errorf("ghi bản HTML thất bại (bản markdown đã lưu tại %s): %w", mdFile, err)
	}

	return mdFile, htmlFile, nil
}

type ProjectSnapshot struct {
	Version    string        `json:"version"`
	ExportedAt string        `json:"exported_at"`
	Tasks      []models.Task `json:"tasks"`
}

func (e *ExporterService) ExportProjectSnapshotJSON(tasks []models.Task, outDir string) (string, error) {
	ts := time.Now().Format("20060102_150405")
	if outDir == "" {
		outDir = "."
	}

	snap := ProjectSnapshot{
		Version:    "v2.2.0",
		ExportedAt: time.Now().Format(time.RFC3339),
		Tasks:      tasks,
	}

	jsonBytes, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", err
	}

	filePath := filepath.Join(outDir, fmt.Sprintf("AgentCenter_Backup_%s.json", ts))
	if err := os.WriteFile(filePath, jsonBytes, 0644); err != nil {
		return "", err
	}

	return filePath, nil
}
