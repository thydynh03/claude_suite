package cli

import "testing"

// The CLI reports the model it actually runs — in system:init and on every
// assistant message. Parsing it is what turns "which model is this agent
// using" from a configured wish into a measured fact.
func TestParseStreamEventCapturesTheModelActuallyUsed(t *testing.T) {
	s := &streamParse{}

	parseStreamEvent(`{"type":"system","subtype":"init","session_id":"s1","model":"claude-opus-4-8-20260101"}`, s, nil)
	if s.modelUsed != "claude-opus-4-8-20260101" {
		t.Fatalf("init model not captured: %q", s.modelUsed)
	}

	// A later assistant message wins — it is what actually generated output
	// (the CLI can switch models mid-session).
	parseStreamEvent(`{"type":"assistant","message":{"model":"claude-opus-5-20260901","content":[{"type":"text","text":"xin chào"}]}}`, s, nil)
	if s.modelUsed != "claude-opus-5-20260901" {
		t.Fatalf("assistant model not captured: %q", s.modelUsed)
	}

	// Events without a model must not erase what is known.
	parseStreamEvent(`{"type":"result","result":"done","subtype":"success"}`, s, nil)
	if s.modelUsed != "claude-opus-5-20260901" {
		t.Fatalf("model erased by a model-less event: %q", s.modelUsed)
	}
}
