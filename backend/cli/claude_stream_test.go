package cli

import "testing"

// Verifies the stream-json parser extracts real token usage, cost, session id,
// the final result text, and surfaces tool calls.
func TestParseStreamEvent_ResultUsage(t *testing.T) {
	s := &streamParse{}

	var tools []string
	onLog := func(msg, level string) {
		if level == "TOOL" {
			tools = append(tools, msg)
		}
	}

	lines := []string{
		`{"type":"assistant","session_id":"abc-123","message":{"content":[{"type":"text","text":"Working on it"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"main.go"}}]}}`,
		`{"type":"result","subtype":"success","result":"All done","total_cost_usd":0.0123,"usage":{"input_tokens":100,"output_tokens":50}}`,
	}
	for _, l := range lines {
		parseStreamEvent(l, s, onLog)
	}

	if s.sessionID != "abc-123" {
		t.Errorf("sessionID = %q, want abc-123", s.sessionID)
	}
	if s.result != "All done" {
		t.Errorf("result = %q, want 'All done'", s.result)
	}
	if s.totalTokens() != 150 {
		t.Errorf("totalTokens = %d, want 150", s.totalTokens())
	}
	if s.costUSD != 0.0123 {
		t.Errorf("costUSD = %v, want 0.0123", s.costUSD)
	}
	if s.isError {
		t.Error("isError should be false for a success result")
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool log, got %d", len(tools))
	}
}

func TestParseStreamEvent_ErrorResult(t *testing.T) {
	s := &streamParse{}
	parseStreamEvent(`{"type":"result","subtype":"error_max_turns","is_error":true,"result":"boom"}`, s, nil)
	if !s.isError {
		t.Error("expected isError=true for error subtype")
	}
}
