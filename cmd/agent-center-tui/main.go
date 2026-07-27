package main

import (
	"flag"
	"fmt"
	"os"

	"agent_center/backend/tui"

	tea "charm.land/bubbletea/v2"
)

func main() {
	dbPath := flag.String("db", "", "path to an existing Agent Center SQLite database (required)")
	check := flag.Bool("check", false, "validate the database without starting the UI")
	write := flag.Bool("write", false, "enable task mutations through the shared repository")
	flag.Parse()

	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "Agent Center TUI: --db PATH is required")
		flag.Usage()
		os.Exit(2)
	}
	snapshot, loadErr := tui.LoadReadOnly(*dbPath)
	if *check {
		if loadErr != nil {
			fmt.Fprintln(os.Stderr, "Agent Center TUI:", loadErr)
			os.Exit(1)
		}
		fmt.Printf("Agent Center database OK (read-only): %d agents, %d tasks, %d recent memory entries\n",
			len(snapshot.Agents), len(snapshot.Tasks), len(snapshot.RecentMemory))
		return
	}

	var actions tui.TaskActions
	if *write {
		// LoadReadOnly already rejected this database — opening it read/write anyway
		// would let mutations run against a schema the read model refused. Read-only
		// mode still renders loadErr in the UI, so only the write path exits here.
		if loadErr != nil {
			fmt.Fprintln(os.Stderr, "Agent Center TUI: refusing --write on an unusable database:", loadErr)
			os.Exit(1)
		}
		repositoryActions, err := tui.OpenRepositoryTaskActions(*dbPath, snapshot.Workspace)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Agent Center TUI:", err)
			os.Exit(1)
		}
		defer repositoryActions.Close()
		actions = repositoryActions
	}
	model := tui.NewWithTaskActions(snapshot, *dbPath, loadErr, tui.LoadReadOnly, actions)
	if _, err := tea.NewProgram(model).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Agent Center TUI:", err)
		os.Exit(1)
	}
}
