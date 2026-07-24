package main

import (
	"flag"
	"fmt"
	"os"

	"claude_suite/backend/tui"

	tea "charm.land/bubbletea/v2"
)

func main() {
	dbPath := flag.String("db", "", "path to an existing Claude Suite SQLite database (required)")
	check := flag.Bool("check", false, "validate the database without starting the UI")
	write := flag.Bool("write", false, "enable task mutations through the shared repository")
	flag.Parse()

	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "Claude Suite TUI: --db PATH is required")
		flag.Usage()
		os.Exit(2)
	}
	snapshot, loadErr := tui.LoadReadOnly(*dbPath)
	if *check {
		if loadErr != nil {
			fmt.Fprintln(os.Stderr, "Claude Suite TUI:", loadErr)
			os.Exit(1)
		}
		fmt.Printf("Claude Suite database OK (read-only): %d agents, %d tasks, %d recent memory entries\n",
			len(snapshot.Agents), len(snapshot.Tasks), len(snapshot.RecentMemory))
		return
	}

	var actions tui.TaskActions
	if *write {
		repositoryActions, err := tui.OpenRepositoryTaskActions(*dbPath, snapshot.Workspace)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Claude Suite TUI:", err)
			os.Exit(1)
		}
		defer repositoryActions.Close()
		actions = repositoryActions
	}
	model := tui.NewWithTaskActions(snapshot, *dbPath, loadErr, tui.LoadReadOnly, actions)
	if _, err := tea.NewProgram(model).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Claude Suite TUI:", err)
		os.Exit(1)
	}
}
