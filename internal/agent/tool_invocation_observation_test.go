package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type mockObservationRecorder struct {
	recorded []biz.Observation
	err      error
}

func (m *mockObservationRecorder) RecordObservation(ctx context.Context, obs biz.Observation) (biz.Observation, error) {
	if m.err != nil {
		return biz.Observation{}, m.err
	}
	m.recorded = append(m.recorded, obs)
	return obs, nil
}

// TestRecordToolCallObservation verifies that a completed tool invocation is
// converted into a tool_call Observation for the learning loop: correct kind,
// agent/session attribution, and metadata carrying tool_name/status/duration_ms
// (the fields DetectPatterns/describeBucket rely on).
func TestRecordToolCallObservation(t *testing.T) {
	t.Parallel()
	rec := &mockObservationRecorder{}
	write := biz.ToolInvocationWrite{
		ToolKey:    "read_file",
		Status:     "success",
		DurationMS: 42,
		SessionID:  "sess-1",
		Source:     biz.ToolInvocationSourceRuntime,
	}
	ag := biz.Agent{ID: "agent-1"}

	recordToolCallObservation(context.Background(), write, ag, rec, loggateway.NewNoop())

	if len(rec.recorded) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(rec.recorded))
	}
	obs := rec.recorded[0]
	if obs.Kind != biz.ObservationKindToolCall {
		t.Fatalf("kind = %q, want %q", obs.Kind, biz.ObservationKindToolCall)
	}
	if obs.AgentID != "agent-1" {
		t.Fatalf("agent_id = %q, want agent-1", obs.AgentID)
	}
	if obs.SessionID != "sess-1" {
		t.Fatalf("session_id = %q, want sess-1", obs.SessionID)
	}
	if obs.Content != "read_file" {
		t.Fatalf("content = %q, want read_file", obs.Content)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(obs.Metadata), &meta); err != nil {
		t.Fatalf("metadata not JSON: %v", err)
	}
	if meta["tool_name"] != "read_file" {
		t.Fatalf("metadata.tool_name = %v, want read_file", meta["tool_name"])
	}
	if meta["status"] != "success" {
		t.Fatalf("metadata.status = %v, want success", meta["status"])
	}
	if meta["duration_ms"] != float64(42) {
		t.Fatalf("metadata.duration_ms = %v, want 42", meta["duration_ms"])
	}
}

// TestRecordToolCallObservation_NilRecorder ensures the nil-dependency path is
// a safe no-op (observation recording is optional).
func TestRecordToolCallObservation_NilRecorder(t *testing.T) {
	t.Parallel()
	write := biz.ToolInvocationWrite{ToolKey: "read_file", Status: "success"}
	recordToolCallObservation(context.Background(), write, biz.Agent{ID: "a"}, nil, loggateway.NewNoop())
	// no panic = pass
}

// TestRecordToolCallObservation_FailureStatus verifies failed invocations are
// also recorded (failure patterns are learning signal too).
func TestRecordToolCallObservation_FailureStatus(t *testing.T) {
	t.Parallel()
	rec := &mockObservationRecorder{}
	write := biz.ToolInvocationWrite{
		ToolKey:      "bash",
		Status:       "failed",
		ErrorMessage: "exit 1",
		DurationMS:   7,
	}
	recordToolCallObservation(context.Background(), write, biz.Agent{ID: "agent-9"}, rec, loggateway.NewNoop())
	if len(rec.recorded) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(rec.recorded))
	}
	if !strings.Contains(rec.recorded[0].Metadata, `"status":"failed"`) {
		t.Fatalf("metadata should carry failed status, got %s", rec.recorded[0].Metadata)
	}
}
