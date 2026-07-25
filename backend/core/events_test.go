package core

import "testing"

// A frontend that does not render one of these still wants the others. Nothing
// should panic because a callback was left out.
func TestEventFuncsToleratesMissingCallbacks(t *testing.T) {
	var empty EventFuncs
	empty.Log("a message", "INFO")
	empty.Approval("task-1", "Architect", "Design it")
	empty.BoardChanged()
}

func TestEventFuncsForwardsEverything(t *testing.T) {
	var (
		gotMessage, gotLevel string
		gotTask, gotAgent    string
		gotTitle             string
		boardChanged         bool
	)

	sink := EventFuncs{
		OnLog:      func(message, level string) { gotMessage, gotLevel = message, level },
		OnApproval: func(taskID, agentName, taskTitle string) { gotTask, gotAgent, gotTitle = taskID, agentName, taskTitle },
		OnBoard:    func() { boardChanged = true },
	}

	sink.Log("built", "SUCCESS")
	sink.Approval("task-42", "Architect", "Design the module")
	sink.BoardChanged()

	if gotMessage != "built" || gotLevel != "SUCCESS" {
		t.Errorf("Log forwarded %q/%q", gotMessage, gotLevel)
	}
	// The task id is the whole reason this is three arguments and not two:
	// resolving an approval without one releases every waiting task.
	if gotTask != "task-42" || gotAgent != "Architect" || gotTitle != "Design the module" {
		t.Errorf("Approval forwarded %q/%q/%q", gotTask, gotAgent, gotTitle)
	}
	if !boardChanged {
		t.Error("BoardChanged did not reach the callback")
	}
}

// Attaching nothing must not panic either — a read-only frontend has no sink.
func TestAttachEventSinkIgnoresNils(t *testing.T) {
	AttachEventSink(nil, EventFuncs{})
	AttachEventSink(nil, nil)
}
