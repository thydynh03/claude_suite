package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"claude_suite/backend/models"
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

	mdFile := filepath.Join(outDir, fmt.Sprintf("ClaudeSuite_Kanban_Report_%s.md", ts))
	htmlFile := filepath.Join(outDir, fmt.Sprintf("ClaudeSuite_Kanban_Report_%s.html", ts))

	var md strings.Builder
	md.WriteString("# 📋 Claude Suite — Kanban Report\n\n")
	md.WriteString(fmt.Sprintf("**Thời gian xuất:** %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	statuses := []string{"backlog", "queued", "running", "done", "failed"}
	for _, status := range statuses {
		md.WriteString(fmt.Sprintf("## %s\n", strings.ToUpper(status)))
		count := 0
		for _, t := range tasks {
			if t.Status == status {
				count++
				md.WriteString(fmt.Sprintf("- **[%s]** %s\n  - Priority: `%s` | Assigned: `%s`\n  - Prompt: %s\n", t.TaskID[:8], t.Title, t.Priority, t.AssignedTo, t.Prompt))
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
<title>Claude Suite Kanban Report</title>
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

	_ = os.WriteFile(htmlFile, []byte(htmlContent), 0644)

	return mdFile, htmlFile, nil
}
