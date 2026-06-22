package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// newTestEventProjector creates an EventProjector backed by a syncCaptureBus
// and a Noop logger. The bus is returned for tests that need to assert
// published envelopes, though most EventProjector methods return envelopes
// directly without publishing.
func newTestEventProjector() (*EventProjector, *syncCaptureBus) {
	bus := newSyncCaptureBus()
	p := NewEventProjector(bus, loggateway.NewNoop())
	return p, bus
}

// --- Project dispatcher branches ---

func TestEventProjector_Project_NilEvent_ReturnsNil(t *testing.T) {
	p, _ := newTestEventProjector()
	envs := p.Project(context.Background(), nil, ProjectMeta{SessionID: "sess-1"})
	if envs != nil {
		t.Errorf("expected nil envelopes for nil event, got %d envelopes", len(envs))
	}
}

func TestEventProjector_Project_RunnerCompletion_ReturnsRunnerCompletionEnvelope(t *testing.T) {
	p, _ := newTestEventProjector()
	meta := ProjectMeta{SessionID: "sess-1"}
	ev := &trpcevent.Event{
		Author: "agent",
		Response: &trpcmodel.Response{
			Object: trpcmodel.ObjectTypeRunnerCompletion,
			Done:   true,
		},
	}
	envs := p.Project(context.Background(), ev, meta)
	if len(envs) != 1 {
		t.Fatalf("expected 1 envelope, got %d (types=%v)", len(envs), envelopeTypes(envs))
	}
	if envs[0].Type != event.EnvelopeTypeRunnerCompletion {
		t.Errorf("type=%q want %q", envs[0].Type, event.EnvelopeTypeRunnerCompletion)
	}
	if envs[0].SessionID != "sess-1" {
		t.Errorf("sessionID=%q want %q", envs[0].SessionID, "sess-1")
	}
}

func TestEventProjector_Project_ResponseError_ReturnsErrorEnvelope(t *testing.T) {
	p, _ := newTestEventProjector()
	meta := ProjectMeta{SessionID: "sess-1"}
	ev := &trpcevent.Event{
		Author: "agent",
		Response: &trpcmodel.Response{
			Object: trpcmodel.ObjectTypeError,
			Error: &trpcmodel.ResponseError{
				Type:    "api_error",
				Message: "rate limit exceeded",
			},
		},
	}
	envs := p.Project(context.Background(), ev, meta)
	if len(envs) != 1 {
		t.Fatalf("expected 1 envelope, got %d (types=%v)", len(envs), envelopeTypes(envs))
	}
	if envs[0].Type != event.EnvelopeTypeError {
		t.Errorf("type=%q want %q", envs[0].Type, event.EnvelopeTypeError)
	}
	if envs[0].Error == nil {
		t.Fatal("expected error to be non-nil")
	}
	if envs[0].Error.Message != "rate limit exceeded" {
		t.Errorf("error message=%q want %q", envs[0].Error.Message, "rate limit exceeded")
	}
}

func TestEventProjector_Project_StateDelta_ReturnsStateDeltaEnvelope(t *testing.T) {
	p, _ := newTestEventProjector()
	meta := ProjectMeta{SessionID: "sess-1"}
	ev := &trpcevent.Event{
		Author: "agent",
		Response: &trpcmodel.Response{
			Object: trpcmodel.ObjectTypeStateUpdate,
		},
		StateDelta: map[string][]byte{
			"key1": []byte(`"value1"`),
		},
	}
	envs := p.Project(context.Background(), ev, meta)
	if len(envs) != 1 {
		t.Fatalf("expected 1 envelope, got %d (types=%v)", len(envs), envelopeTypes(envs))
	}
	if envs[0].Type != event.EnvelopeTypeStateDelta {
		t.Errorf("type=%q want %q", envs[0].Type, event.EnvelopeTypeStateDelta)
	}
	if envs[0].StateDelta == nil {
		t.Fatal("expected state delta to be non-nil")
	}
}

func TestEventProjector_Project_ChatCompletion_ReturnsTextDone(t *testing.T) {
	p, _ := newTestEventProjector()
	meta := ProjectMeta{SessionID: "sess-1"}
	ev := &trpcevent.Event{
		Author: "agent",
		Response: &trpcmodel.Response{
			Object: trpcmodel.ObjectTypeChatCompletion,
			Choices: []trpcmodel.Choice{
				{
					Message: trpcmodel.Message{
						Content: "final reply",
					},
				},
			},
		},
	}
	envs := p.Project(context.Background(), ev, meta)
	if len(envs) != 1 {
		t.Fatalf("expected 1 envelope, got %d (types=%v)", len(envs), envelopeTypes(envs))
	}
	if envs[0].Type != event.EnvelopeTypeTextDone {
		t.Errorf("type=%q want %q", envs[0].Type, event.EnvelopeTypeTextDone)
	}
	if envs[0].Content == nil {
		t.Fatal("expected content to be non-nil")
	}
	if envs[0].Content.Text != "final reply" {
		t.Errorf("content text=%q want %q", envs[0].Content.Text, "final reply")
	}
	if envs[0].Content.IsPartial {
		t.Error("expected IsPartial=false for non-chunk completion")
	}
}

// --- buildErrorEnvelope ---

func TestEventProjector_buildErrorEnvelope_WithTypeAndMessage(t *testing.T) {
	p, _ := newTestEventProjector()
	meta := ProjectMeta{SessionID: "sess-1"}
	ev := &trpcevent.Event{
		Author: "agent",
		Response: &trpcmodel.Response{
			Error: &trpcmodel.ResponseError{
				Type:    "api_error",
				Message: "rate limit exceeded",
			},
		},
	}
	env := p.buildErrorEnvelope(ev, meta)
	if env.Type != event.EnvelopeTypeError {
		t.Errorf("type=%q want %q", env.Type, event.EnvelopeTypeError)
	}
	if env.Error == nil {
		t.Fatal("expected error to be non-nil")
	}
	if env.Error.Type != "api_error" {
		t.Errorf("error type=%q want %q", env.Error.Type, "api_error")
	}
	if env.Error.Code != "api_error" {
		t.Errorf("error code=%q want %q", env.Error.Code, "api_error")
	}
	if env.Error.Message != "rate limit exceeded" {
		t.Errorf("error message=%q want %q", env.Error.Message, "rate limit exceeded")
	}
}

func TestEventProjector_buildErrorEnvelope_EmptyTypeDefaultsToRunError(t *testing.T) {
	p, _ := newTestEventProjector()
	meta := ProjectMeta{SessionID: "sess-1"}
	ev := &trpcevent.Event{
		Author: "agent",
		Response: &trpcmodel.Response{
			Error: &trpcmodel.ResponseError{
				Message: "something failed",
			},
		},
	}
	env := p.buildErrorEnvelope(ev, meta)
	if env.Error == nil {
		t.Fatal("expected error to be non-nil")
	}
	if env.Error.Type != "run_error" {
		t.Errorf("error type=%q want %q (default)", env.Error.Type, "run_error")
	}
	if env.Error.Code != "run_error" {
		t.Errorf("error code=%q want %q (default)", env.Error.Code, "run_error")
	}
	if env.Error.Message != "something failed" {
		t.Errorf("error message=%q want %q", env.Error.Message, "something failed")
	}
}

// --- buildRunnerCompletionEnvelope ---

func TestEventProjector_buildRunnerCompletionEnvelope_WithUsage(t *testing.T) {
	p, _ := newTestEventProjector()
	meta := ProjectMeta{
		SessionID:     "sess-1",
		ContextWindow: 8192,
	}
	ev := &trpcevent.Event{
		Author: "agent",
		Response: &trpcmodel.Response{
			Usage: &trpcmodel.Usage{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
		},
	}
	env := p.buildRunnerCompletionEnvelope(ev, meta)
	if env.Type != event.EnvelopeTypeRunnerCompletion {
		t.Errorf("type=%q want %q", env.Type, event.EnvelopeTypeRunnerCompletion)
	}
	if env.Usage == nil {
		t.Fatal("expected usage to be non-nil")
	}
	if env.Usage.PromptTokens != 100 {
		t.Errorf("prompt tokens=%d want 100", env.Usage.PromptTokens)
	}
	if env.Usage.CompletionTokens != 50 {
		t.Errorf("completion tokens=%d want 50", env.Usage.CompletionTokens)
	}
	if env.Usage.TotalTokens != 150 {
		t.Errorf("total tokens=%d want 150", env.Usage.TotalTokens)
	}
	if env.Usage.MaxTokens != 8192 {
		t.Errorf("max tokens=%d want 8192 (from ContextWindow)", env.Usage.MaxTokens)
	}
	// ContextPromptTokens should mirror PromptTokens when TurnPromptTokens is unset.
	if env.Usage.ContextPromptTokens != 100 {
		t.Errorf("context prompt tokens=%d want 100", env.Usage.ContextPromptTokens)
	}
}

func TestEventProjector_buildRunnerCompletionEnvelope_WithTeamAndRunMetadata(t *testing.T) {
	p, _ := newTestEventProjector()
	meta := ProjectMeta{
		SessionID:        "sess-1",
		TeamID:           "team-1",
		RunID:            "run-1",
		TraceID:          "trace-1",
		AgentID:          "agent-1",
		AgentDisplayName: "Agent One",
	}
	ev := &trpcevent.Event{
		Author:   "agent",
		Response: &trpcmodel.Response{},
	}
	env := p.buildRunnerCompletionEnvelope(ev, meta)
	if env.Type != event.EnvelopeTypeRunnerCompletion {
		t.Errorf("type=%q want %q", env.Type, event.EnvelopeTypeRunnerCompletion)
	}
	// TeamID is set by baseEnvelope via FromFrameworkEvent.
	if env.TeamID != "team-1" {
		t.Errorf("teamID=%q want %q", env.TeamID, "team-1")
	}
	// run_kind should be "team" when TeamID is non-empty.
	runKind, _ := env.Metadata["run_kind"].(string)
	if runKind != "team" {
		t.Errorf("metadata run_kind=%q want %q", runKind, "team")
	}
	runID, _ := env.Metadata["run_id"].(string)
	if runID != "run-1" {
		t.Errorf("metadata run_id=%q want %q", runID, "run-1")
	}
	traceID, _ := env.Metadata["trace_id"].(string)
	if traceID != "trace-1" {
		t.Errorf("metadata trace_id=%q want %q", traceID, "trace-1")
	}
	agentID, _ := env.Metadata["agent_id"].(string)
	if agentID != "agent-1" {
		t.Errorf("metadata agent_id=%q want %q", agentID, "agent-1")
	}
	agentDisplayName, _ := env.Metadata["agent_display_name"].(string)
	if agentDisplayName != "Agent One" {
		t.Errorf("metadata agent_display_name=%q want %q", agentDisplayName, "Agent One")
	}
}

func TestEventProjector_buildRunnerCompletionEnvelope_ChatRunKind(t *testing.T) {
	p, _ := newTestEventProjector()
	// No TeamID → run_kind should be "chat".
	meta := ProjectMeta{SessionID: "sess-1"}
	ev := &trpcevent.Event{
		Author:   "agent",
		Response: &trpcmodel.Response{},
	}
	env := p.buildRunnerCompletionEnvelope(ev, meta)
	runKind, _ := env.Metadata["run_kind"].(string)
	if runKind != "chat" {
		t.Errorf("run_kind=%q want %q (no TeamID)", runKind, "chat")
	}
}

// --- buildToolResultEnvelope ---

func TestEventProjector_buildToolResultEnvelope_Success(t *testing.T) {
	p, _ := newTestEventProjector()
	meta := ProjectMeta{SessionID: "sess-1"}
	ev := &trpcevent.Event{
		Author: "agent",
		Response: &trpcmodel.Response{
			Object: trpcmodel.ObjectTypeToolResponse,
			Choices: []trpcmodel.Choice{
				{
					Message: trpcmodel.Message{
						ToolID:   "tc-1",
						ToolName: "read_file",
						Content:  `{"content":"file content"}`,
					},
				},
			},
		},
	}
	env := p.buildToolResultEnvelope(context.Background(), ev, meta)
	if env.Type != event.EnvelopeTypeToolResult {
		t.Errorf("type=%q want %q", env.Type, event.EnvelopeTypeToolResult)
	}
	if env.ToolCall == nil {
		t.Fatal("expected tool call to be non-nil")
	}
	if env.ToolCall.ID != "tc-1" {
		t.Errorf("tool call id=%q want %q", env.ToolCall.ID, "tc-1")
	}
	if env.ToolCall.Name != "read_file" {
		t.Errorf("tool call name=%q want %q", env.ToolCall.Name, "read_file")
	}
	if env.ToolCall.Status != "success" {
		t.Errorf("tool call status=%q want %q", env.ToolCall.Status, "success")
	}
	if env.ToolCall.ResultJSON == "" {
		t.Error("expected non-empty result JSON for successful tool call")
	}
}

func TestEventProjector_buildToolResultEnvelope_Error(t *testing.T) {
	p, _ := newTestEventProjector()
	meta := ProjectMeta{SessionID: "sess-1"}
	ev := &trpcevent.Event{
		Author: "agent",
		Response: &trpcmodel.Response{
			Object: trpcmodel.ObjectTypeToolResponse,
			Error: &trpcmodel.ResponseError{
				Type:    "tool_timeout",
				Message: "tool execution timed out",
			},
			Choices: []trpcmodel.Choice{
				{
					Message: trpcmodel.Message{
						ToolID:   "tc-1",
						ToolName: "read_file",
						Content:  "",
					},
				},
			},
		},
	}
	env := p.buildToolResultEnvelope(context.Background(), ev, meta)
	if env.ToolCall == nil {
		t.Fatal("expected tool call to be non-nil")
	}
	if env.ToolCall.Status != "failed" {
		t.Errorf("tool call status=%q want %q", env.ToolCall.Status, "failed")
	}
	// "tool_timeout" is a valid error code and should be preserved.
	if env.ToolCall.ErrorCode != "tool_timeout" {
		t.Errorf("tool call error code=%q want %q", env.ToolCall.ErrorCode, "tool_timeout")
	}
	// Result JSON should contain the error message (merged in by mergeToolErrorResult).
	if !strings.Contains(env.ToolCall.ResultJSON, "tool execution timed out") {
		t.Errorf("result JSON=%q should contain error message", env.ToolCall.ResultJSON)
	}
}

func TestEventProjector_buildToolResultEnvelope_EmptyResponseReturnsBaseEnvelope(t *testing.T) {
	p, _ := newTestEventProjector()
	meta := ProjectMeta{SessionID: "sess-1"}
	ev := &trpcevent.Event{
		Author:   "agent",
		Response: &trpcmodel.Response{Object: trpcmodel.ObjectTypeToolResponse},
	}
	env := p.buildToolResultEnvelope(context.Background(), ev, meta)
	if env.Type != event.EnvelopeTypeToolResult {
		t.Errorf("type=%q want %q", env.Type, event.EnvelopeTypeToolResult)
	}
	// No choices → ToolCall should be nil (base envelope only).
	if env.ToolCall != nil {
		t.Errorf("expected nil ToolCall for empty choices, got %+v", env.ToolCall)
	}
}

// --- buildStateDeltaEnvelope ---

func TestEventProjector_buildStateDeltaEnvelope_WithDelta(t *testing.T) {
	p, _ := newTestEventProjector()
	meta := ProjectMeta{SessionID: "sess-1"}
	ev := &trpcevent.Event{
		Author: "agent",
		StateDelta: map[string][]byte{
			"key1": []byte(`"value1"`),
			"key2": []byte(`"value2"`),
		},
	}
	env := p.buildStateDeltaEnvelope(ev, meta)
	if env.Type != event.EnvelopeTypeStateDelta {
		t.Errorf("type=%q want %q", env.Type, event.EnvelopeTypeStateDelta)
	}
	if env.StateDelta == nil {
		t.Fatal("expected state delta to be non-nil")
	}
	if env.StateDelta.Operation != "set" {
		t.Errorf("operation=%q want %q", env.StateDelta.Operation, "set")
	}
	if env.StateDelta.Path != "__state__" {
		t.Errorf("path=%q want %q", env.StateDelta.Path, "__state__")
	}
	if env.StateDelta.ValueJSON == "" {
		t.Error("expected non-empty value JSON")
	}
	// ValueJSON is the JSON-marshaled StateDelta map ([]byte values are base64-encoded).
	var parsed map[string]any
	if err := json.Unmarshal([]byte(env.StateDelta.ValueJSON), &parsed); err != nil {
		t.Fatalf("value JSON is not valid JSON: %v", err)
	}
	if len(parsed) != 2 {
		t.Errorf("expected 2 keys in value JSON, got %d", len(parsed))
	}
}

// --- Build*Envelope series ---

func TestEventProjector_BuildLogEnvelope(t *testing.T) {
	p, _ := newTestEventProjector()
	env := p.BuildLogEnvelope("info", "hello world", "monitor", "sess-1")
	if env.Type != event.EnvelopeTypeLog {
		t.Errorf("type=%q want %q", env.Type, event.EnvelopeTypeLog)
	}
	if env.Author != "monitor" {
		t.Errorf("author=%q want %q", env.Author, "monitor")
	}
	if env.SessionID != "sess-1" {
		t.Errorf("sessionID=%q want %q", env.SessionID, "sess-1")
	}
	level, _ := env.Metadata["level"].(string)
	if level != "info" {
		t.Errorf("metadata level=%q want %q", level, "info")
	}
	source, _ := env.Metadata["source"].(string)
	if source != "monitor" {
		t.Errorf("metadata source=%q want %q", source, "monitor")
	}
	if env.Content == nil {
		t.Fatal("expected content to be non-nil")
	}
	if env.Content.Text != "hello world" {
		t.Errorf("content text=%q want %q", env.Content.Text, "hello world")
	}
	if env.Content.IsPartial {
		t.Error("expected IsPartial=false for log envelope")
	}
}

func TestEventProjector_BuildIntentPassEnvelope(t *testing.T) {
	p, _ := newTestEventProjector()
	payload := map[string]any{"intent": "search", "confidence": 0.95}
	env := p.BuildIntentPassEnvelope(payload, "sess-1", "team-1")
	if env.Type != event.EnvelopeTypeIntentPass {
		t.Errorf("type=%q want %q", env.Type, event.EnvelopeTypeIntentPass)
	}
	if env.Author != "system" {
		t.Errorf("author=%q want %q", env.Author, "system")
	}
	if env.SessionID != "sess-1" {
		t.Errorf("sessionID=%q want %q", env.SessionID, "sess-1")
	}
	if env.TeamID != "team-1" {
		t.Errorf("teamID=%q want %q", env.TeamID, "team-1")
	}
	intent, _ := env.Metadata["intent"].(string)
	if intent != "search" {
		t.Errorf("metadata intent=%q want %q", intent, "search")
	}
	confidence, _ := env.Metadata["confidence"].(float64)
	if confidence != 0.95 {
		t.Errorf("metadata confidence=%v want 0.95", confidence)
	}
}

func TestEventProjector_BuildMemberMessageStartEnvelope(t *testing.T) {
	p, _ := newTestEventProjector()
	env := p.BuildMemberMessageStartEnvelope("worker-a", "sess-1", "team-1", "branch-1")
	if env.Type != event.EnvelopeTypeMemberMessageStart {
		t.Errorf("type=%q want %q", env.Type, event.EnvelopeTypeMemberMessageStart)
	}
	if env.Author != "worker-a" {
		t.Errorf("author=%q want %q", env.Author, "worker-a")
	}
	if env.SessionID != "sess-1" {
		t.Errorf("sessionID=%q want %q", env.SessionID, "sess-1")
	}
	if env.TeamID != "team-1" {
		t.Errorf("teamID=%q want %q", env.TeamID, "team-1")
	}
	if env.Branch != "branch-1" {
		t.Errorf("branch=%q want %q", env.Branch, "branch-1")
	}
}

func TestEventProjector_BuildMemberDeltaEnvelope(t *testing.T) {
	p, _ := newTestEventProjector()
	env := p.BuildMemberDeltaEnvelope("worker-a", "sess-1", "team-1", "Hello ")
	if env.Type != event.EnvelopeTypeMemberDelta {
		t.Errorf("type=%q want %q", env.Type, event.EnvelopeTypeMemberDelta)
	}
	if env.Author != "worker-a" {
		t.Errorf("author=%q want %q", env.Author, "worker-a")
	}
	if env.SessionID != "sess-1" {
		t.Errorf("sessionID=%q want %q", env.SessionID, "sess-1")
	}
	if env.TeamID != "team-1" {
		t.Errorf("teamID=%q want %q", env.TeamID, "team-1")
	}
	if env.Content == nil {
		t.Fatal("expected content to be non-nil")
	}
	if env.Content.Text != "Hello " {
		t.Errorf("content text=%q want %q", env.Content.Text, "Hello ")
	}
	if !env.Content.IsPartial {
		t.Error("expected IsPartial=true for member delta envelope")
	}
}

func TestEventProjector_BuildMemberMessageDoneEnvelope(t *testing.T) {
	p, _ := newTestEventProjector()
	env := p.BuildMemberMessageDoneEnvelope("worker-a", "sess-1", "team-1", "Hello world")
	if env.Type != event.EnvelopeTypeMemberMessageDone {
		t.Errorf("type=%q want %q", env.Type, event.EnvelopeTypeMemberMessageDone)
	}
	if env.Author != "worker-a" {
		t.Errorf("author=%q want %q", env.Author, "worker-a")
	}
	if env.SessionID != "sess-1" {
		t.Errorf("sessionID=%q want %q", env.SessionID, "sess-1")
	}
	if env.TeamID != "team-1" {
		t.Errorf("teamID=%q want %q", env.TeamID, "team-1")
	}
	if env.Content == nil {
		t.Fatal("expected content to be non-nil")
	}
	if env.Content.Text != "Hello world" {
		t.Errorf("content text=%q want %q", env.Content.Text, "Hello world")
	}
	if env.Content.IsPartial {
		t.Error("expected IsPartial=false for member message done envelope")
	}
}

// --- projectMemberText ---

func TestEventProjector_projectMemberText_Partial(t *testing.T) {
	p, _ := newTestEventProjector()
	meta := ProjectMeta{
		SessionID: "sess-1",
		TeamID:    "team-1",
	}
	ev := &trpcevent.Event{Author: "worker-a"}

	// First partial call: should produce MemberMessageStart + MemberDelta.
	envs := p.projectMemberText(ev, meta, "Hello", "", true)
	if len(envs) != 2 {
		t.Fatalf("expected 2 envelopes (start + delta), got %d (types=%v)",
			len(envs), envelopeTypes(envs))
	}
	if envs[0].Type != event.EnvelopeTypeMemberMessageStart {
		t.Errorf("first envelope type=%q want %q",
			envs[0].Type, event.EnvelopeTypeMemberMessageStart)
	}
	if envs[1].Type != event.EnvelopeTypeMemberDelta {
		t.Errorf("second envelope type=%q want %q",
			envs[1].Type, event.EnvelopeTypeMemberDelta)
	}
	if envs[1].Content == nil {
		t.Fatal("expected delta content to be non-nil")
	}
	if envs[1].Content.Text != "Hello" {
		t.Errorf("delta content text=%q want %q", envs[1].Content.Text, "Hello")
	}
	if !envs[1].Content.IsPartial {
		t.Error("expected IsPartial=true for member delta")
	}

	// Second partial call: should produce only MemberDelta (no new start).
	envs2 := p.projectMemberText(ev, meta, "Hello world", "", true)
	if len(envs2) != 1 {
		t.Fatalf("expected 1 envelope on second delta (delta only), got %d (types=%v)",
			len(envs2), envelopeTypes(envs2))
	}
	if envs2[0].Type != event.EnvelopeTypeMemberDelta {
		t.Errorf("second-call envelope type=%q want %q",
			envs2[0].Type, event.EnvelopeTypeMemberDelta)
	}
	// visibleStreamDelta returns only the suffix delta (" world"), but
	// projectMemberText applies strings.TrimSpace before building the envelope.
	if envs2[0].Content.Text != "world" {
		t.Errorf("second delta content text=%q want %q", envs2[0].Content.Text, "world")
	}
}

func TestEventProjector_projectMemberText_Done(t *testing.T) {
	p, _ := newTestEventProjector()
	meta := ProjectMeta{
		SessionID: "sess-1",
		TeamID:    "team-1",
	}
	ev := &trpcevent.Event{Author: "worker-a"}

	// Done call on first message: should produce MemberMessageStart + MemberMessageDone.
	envs := p.projectMemberText(ev, meta, "Hello world", "", false)
	if len(envs) != 2 {
		t.Fatalf("expected 2 envelopes (start + done), got %d (types=%v)",
			len(envs), envelopeTypes(envs))
	}
	if envs[0].Type != event.EnvelopeTypeMemberMessageStart {
		t.Errorf("first envelope type=%q want %q",
			envs[0].Type, event.EnvelopeTypeMemberMessageStart)
	}
	if envs[1].Type != event.EnvelopeTypeMemberMessageDone {
		t.Errorf("second envelope type=%q want %q",
			envs[1].Type, event.EnvelopeTypeMemberMessageDone)
	}
	if envs[1].Content == nil {
		t.Fatal("expected done content to be non-nil")
	}
	if envs[1].Content.Text != "Hello world" {
		t.Errorf("done content text=%q want %q", envs[1].Content.Text, "Hello world")
	}
	if envs[1].Content.IsPartial {
		t.Error("expected IsPartial=false for member message done")
	}
}

func TestEventProjector_projectMemberText_EmptyAuthorReturnsNil(t *testing.T) {
	p, _ := newTestEventProjector()
	meta := ProjectMeta{SessionID: "sess-1"}
	ev := &trpcevent.Event{Author: ""}
	envs := p.projectMemberText(ev, meta, "Hello", "", true)
	if envs != nil {
		t.Errorf("expected nil for empty author, got %d envelopes", len(envs))
	}
}
