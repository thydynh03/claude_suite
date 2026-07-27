package core

import "agent_center/backend/orchestrator"

// EventSink is how a frontend receives what the orchestrator is doing.
//
// The orchestrator already accepts these as three positional function values.
// That works, and it is also three same-shaped arguments in a row: passing the
// board callback where the log callback belongs compiles cleanly and produces a
// UI that quietly stops updating. Naming them removes that.
type EventSink interface {
	// Log is a line for the global stream.
	Log(message, level string)

	// Approval means a task is waiting for a person.
	//
	// taskID is not optional. Resolving an approval without one releases every
	// waiting task at once, so the prompt on screen would also decide the ones
	// the user cannot see.
	Approval(taskID, agentName, taskTitle string)

	// BoardChanged means the task board should be refetched.
	BoardChanged()
}

// AttachEventSink wires a sink into an orchestrator.
//
// One call instead of three arguments whose order the compiler cannot check.
func AttachEventSink(o *orchestrator.Orchestrator, sink EventSink) {
	if o == nil || sink == nil {
		return
	}
	o.SetEventHandlers(sink.Log, sink.Approval, sink.BoardChanged)
}

// EventFuncs adapts plain functions to an EventSink, for a caller that has
// closures rather than a type.
type EventFuncs struct {
	OnLog      func(message, level string)
	OnApproval func(taskID, agentName, taskTitle string)
	OnBoard    func()
}

func (e EventFuncs) Log(message, level string) {
	if e.OnLog != nil {
		e.OnLog(message, level)
	}
}

func (e EventFuncs) Approval(taskID, agentName, taskTitle string) {
	if e.OnApproval != nil {
		e.OnApproval(taskID, agentName, taskTitle)
	}
}

func (e EventFuncs) BoardChanged() {
	if e.OnBoard != nil {
		e.OnBoard()
	}
}

// A missing callback must be a no-op rather than a panic: a frontend that does
// not render a board still wants logs.
var _ EventSink = EventFuncs{}
