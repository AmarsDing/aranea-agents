package server

import (
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// ws_v2_wire_test.go — Wire/Domain 类型分离的安全网测试。
//
// 两类断言：
//  1. Golden 字节一致：wire payload 序列化结果必须 ≡ 领域事件默认序列化结果
//     （当前线上行为），保证重构零协议变更。
//  2. Key 契约：payload 的 JSON key 集合显式锁定，防止未来字段漂移静默
//     改变线上协议（前端 v2Types.ts 依赖 PascalCase key）。

// mustMarshalJSON is a test helper that marshals v or fails.
func mustMarshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// topLevelKeys unmarshals b into a map and returns its top-level keys.
func topLevelKeys(t *testing.T, b []byte) map[string]bool {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

var wireTestTime = time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

func wireTestTask() biz.Task {
	completed := wireTestTime.Add(time.Hour)
	return biz.Task{
		ID:          "task-1",
		SessionID:   "sess-1",
		UserMessage: "hello",
		Status:      biz.TaskStatusCompleted,
		Seq:         3,
		Version:     7,
		CreatedAt:   wireTestTime,
		UpdatedAt:   wireTestTime.Add(time.Minute),
		CompletedAt: &completed,
	}
}

func wireTestTurn() biz.Turn {
	completed := wireTestTime.Add(time.Hour)
	return biz.Turn{
		ID:              "turn-1",
		TaskID:          "task-1",
		SessionID:       "sess-1",
		SpiritSessionID: "sess-1",
		ParentTurnID:    "turn-0",
		AgentKey:        "agent-a",
		TeamID:          "team-1",
		TeamStageID:     "ts-1",
		Seq:             2,
		Version:         5,
		Status:          biz.TurnStatusCompleted,
		StartedAt:       wireTestTime,
		CompletedAt:     &completed,
	}
}

func wireTestStep() biz.Step {
	completed := wireTestTime.Add(time.Hour)
	return biz.Step{
		ID:              "step-1",
		TurnID:          "turn-1",
		TaskID:          "task-1",
		SessionID:       "sess-1",
		SpiritSessionID: "sess-1",
		Kind:            biz.StepKindAction,
		AuthorAgentKey:  "agent-a",
		Seq:             4,
		Version:         9,
		Content:         "content",
		Reasoning:       "reasoning",
		ToolName:        "read_file",
		ToolCallID:      "tc-1",
		ToolArgs:        json.RawMessage(`{"path":"/a.go"}`),
		ToolResult:      json.RawMessage(`{"ok":true}`),
		ToolDurationMs:  42,
		ToolErrorCode:   "E_X",
		NoticeType:      "model_router",
		Status:          biz.StepStatusCompleted,
		IsFinal:         true,
		StartedAt:       wireTestTime,
		CompletedAt:     &completed,
	}
}

func wireTestTeamStage() biz.TeamStage {
	completed := wireTestTime.Add(time.Hour)
	return biz.TeamStage{
		ID:        "ts-1",
		TaskID:    "task-1",
		TurnID:    "turn-1",
		SessionID: "sess-1",
		TeamID:    "team-1",
		TeamName:  "调研团队",
		DagNodeID: "ps-1",
		DependsOn: []string{"ts-0"},
		Status:    biz.TeamStageStatusRunning,
		Stage:     biz.TeamStageStageExecuting,
		Members: []biz.MemberInfo{{
			AgentKey:       "agent-a",
			AgentName:      "Agent A",
			AvatarURL:      "https://x/a.png",
			ChildSessionID: "sess-child-1",
			Status:         "running",
		}},
		Strategy:    "dag",
		StartedAt:   wireTestTime,
		CompletedAt: &completed,
		Seq:         1,
		Version:     2,
	}
}

func wireTestTeamRun() biz.TeamRun {
	completed := wireTestTime.Add(time.Hour)
	return biz.TeamRun{
		ID:              "tr-1",
		TeamStageID:     "ts-1",
		TaskID:          "task-1",
		SessionID:       "sess-team-1",
		SpiritSessionID: "sess-1",
		DagNodeID:       "ps-1",
		DependsOn:       []string{"tr-0"},
		Status:          biz.TeamRunV2StatusRunning,
		StartedAt:       wireTestTime,
		CompletedAt:     &completed,
		Seq:             1,
		Version:         1,
		Error:           "",
	}
}

func wireTestMemberSession() biz.MemberSession {
	finished := wireTestTime.Add(time.Hour)
	return biz.MemberSession{
		ID:              "ms-1",
		TeamRunID:       "tr-1",
		TeamStageID:     "ts-1",
		TaskID:          "task-1",
		SessionID:       "sess-child-1",
		SpiritSessionID: "sess-1",
		AgentKey:        "agent-a",
		AgentName:       "Agent A",
		AvatarURL:       "https://x/a.png",
		Status:          biz.MemberSessionStatusCompleted,
		Seq:             1,
		Version:         3,
		StartedAt:       wireTestTime,
		FinishedAt:      &finished,
		Error:           "",
	}
}

func wireTestPlanStep() biz.PlanStep {
	completed := wireTestTime.Add(time.Hour)
	return biz.PlanStep{
		ID:                "ps-1",
		PlanID:            "pb-1",
		TaskID:            "task-1",
		Label:             "步骤一",
		Description:       "desc",
		DependsOn:         []string{"ps-0"},
		MappedTeamStageID: "ts-1",
		Status:            biz.PlanStepStatusCompleted,
		AutoSynthesis:     false,
		StartedAt:         wireTestTime,
		CompletedAt:       &completed,
		Seq:               1,
		Version:           2,
		Result: &biz.StepResult{
			Output: "out",
			MemberReports: []biz.MemberReport{{
				AgentKey:   "agent-a",
				AgentName:  "Agent A",
				Output:     "member out",
				TokensUsed: biz.TokenUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
				DurationMs: 100,
				Error:      "",
			}},
			TokensUsed: biz.TokenUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
			DurationMs: 100,
		},
		Error: &biz.StepError{
			Code:      "tool_failed",
			Message:   "boom",
			Retryable: true,
			FailedMember: &biz.MemberReport{
				AgentKey:   "agent-a",
				AgentName:  "Agent A",
				Output:     "",
				TokensUsed: biz.TokenUsage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3},
				DurationMs: 50,
				Error:      "boom",
			},
		},
		AgentKeys: []string{"agent-a"},
	}
}

func wireTestPlanBoard() biz.PlanBoard {
	completed := wireTestTime.Add(time.Hour)
	return biz.PlanBoard{
		ID:          "pb-1",
		TaskID:      "task-1",
		TurnID:      "turn-1",
		SessionID:   "sess-1",
		Strategy:    biz.PlanStrategyDAG,
		Status:      biz.PlanStatusExecuting,
		Steps:       []biz.PlanStep{wireTestPlanStep()},
		StartedAt:   wireTestTime,
		CompletedAt: &completed,
		Seq:         1,
		Version:     4,
	}
}

func wireTestGraphStage() biz.GraphStage {
	completed := wireTestTime.Add(time.Hour)
	return biz.GraphStage{
		ID:          "gs-1",
		TaskID:      "task-1",
		TurnID:      "turn-1",
		SessionID:   "sess-1",
		PlanBoardID: "pb-1",
		Nodes: []biz.GraphNode{{
			ID:           "ps-1",
			GraphStageID: "gs-1",
			Label:        "步骤一",
			DagNodeID:    "ps-1",
			TeamStageID:  "ts-1",
			Status:       biz.GraphNodeStatusRunning,
			DependsOn:    []string{"ps-0"},
		}},
		Status:      biz.GraphStageStatusRunning,
		StartedAt:   wireTestTime,
		CompletedAt: &completed,
		Seq:         1,
		Version:     2,
	}
}

// wireGoldenCase pairs a domain event with the expected top-level payload keys.
type wireGoldenCase struct {
	name string
	// buildDomain constructs the domain event (current wire behavior baseline).
	buildDomain func() biz.Event
	// wantKeys is the explicit top-level key contract of the payload.
	wantKeys []string
}

func wireGoldenCases() []wireGoldenCase {
	return []wireGoldenCase{
		{"task.created", func() biz.Event { return biz.NewTaskCreatedEvent(wireTestTask()) }, []string{"Task"}},
		{"task.updated", func() biz.Event { return biz.NewTaskUpdatedEvent(wireTestTask()) }, []string{"Task"}},
		{"task.completed", func() biz.Event { return biz.NewTaskCompletedEvent(wireTestTask()) }, []string{"Task"}},
		{"task.failed", func() biz.Event { return biz.NewTaskFailedEvent(wireTestTask()) }, []string{"Task"}},
		{"turn.started", func() biz.Event { return biz.NewTurnStartedEvent(wireTestTurn()) }, []string{"TurnID", "Turn"}},
		{"turn.completed", func() biz.Event { return biz.NewTurnCompletedEvent(wireTestTurn()) }, []string{"TurnID", "Turn"}},
		{"turn.failed", func() biz.Event { return biz.NewTurnFailedEvent(wireTestTurn()) }, []string{"TurnID", "Turn"}},
		{"step.created", func() biz.Event { return biz.NewStepCreatedEvent(wireTestStep()) }, []string{"Step"}},
		{"step.streaming", func() biz.Event {
			return biz.NewStepStreamingEvent("sess-1", "task-1", "step-1", "content", "chunk")
		}, []string{"StepID", "DeltaField", "DeltaChunk"}},
		{"step.updated", func() biz.Event { return biz.NewStepUpdatedEvent(wireTestStep()) }, []string{"Step"}},
		{"step.completed", func() biz.Event { return biz.NewStepCompletedEvent(wireTestStep()) }, []string{"Step"}},
		{"step.failed", func() biz.Event { return biz.NewStepFailedEvent(wireTestStep()) }, []string{"Step"}},
		{"team_stage.created", func() biz.Event { return biz.NewTeamStageCreatedEvent(wireTestTeamStage()) }, []string{"TeamStage"}},
		{"team_stage.updated", func() biz.Event { return biz.NewTeamStageUpdatedEvent(wireTestTeamStage()) }, []string{"TeamStage"}},
		{"team_stage.completed", func() biz.Event { return biz.NewTeamStageCompletedEvent(wireTestTeamStage()) }, []string{"TeamStage"}},
		{"team_stage.failed", func() biz.Event { return biz.NewTeamStageFailedEvent(wireTestTeamStage()) }, []string{"TeamStage"}},
		{"team_run.started", func() biz.Event { return biz.NewTeamRunStartedEvent(wireTestTeamRun()) }, []string{"TeamRun"}},
		{"team_run.completed", func() biz.Event { return biz.NewTeamRunCompletedEvent(wireTestTeamRun()) }, []string{"TeamRun"}},
		{"team_run.failed", func() biz.Event { return biz.NewTeamRunFailedEvent(wireTestTeamRun()) }, []string{"TeamRun"}},
		{"member_session.created", func() biz.Event {
			return biz.NewMemberSessionCreatedEvent(wireTestMemberSession())
		}, []string{"MemberSession"}},
		{"member_session.updated", func() biz.Event {
			return biz.NewMemberSessionUpdatedEvent(wireTestMemberSession())
		}, []string{"MemberSession"}},
		{"plan_board.created", func() biz.Event { return biz.NewPlanBoardCreatedEvent(wireTestPlanBoard()) }, []string{"PlanBoard"}},
		{"plan_board.updated", func() biz.Event { return biz.NewPlanBoardUpdatedEvent(wireTestPlanBoard()) }, []string{"PlanBoard"}},
		{"plan_step.started", func() biz.Event { return biz.NewPlanStepStartedEvent(wireTestPlanStep(), "sess-1") }, []string{"PlanStep"}},
		{"plan_step.completed", func() biz.Event { return biz.NewPlanStepCompletedEvent(wireTestPlanStep(), "sess-1") }, []string{"PlanStep"}},
		{"plan_step.failed", func() biz.Event { return biz.NewPlanStepFailedEvent(wireTestPlanStep(), "sess-1") }, []string{"PlanStep"}},
		{"plan_step.skipped", func() biz.Event {
			return biz.NewPlanStepSkippedEvent(wireTestPlanStep(), "sess-1", "dependency_failed")
		}, []string{"PlanStep", "Reason"}},
		{"plan_step.updated", func() biz.Event { return biz.NewPlanStepUpdatedEvent(wireTestPlanStep(), "sess-1") }, []string{"PlanStep"}},
		{"graph_stage.created", func() biz.Event { return biz.NewGraphStageCreatedEvent(wireTestGraphStage()) }, []string{"GraphStage"}},
		{"graph_stage.updated", func() biz.Event { return biz.NewGraphStageUpdatedEvent(wireTestGraphStage()) }, []string{"GraphStage"}},
		{"graph_stage.completed", func() biz.Event { return biz.NewGraphStageCompletedEvent(wireTestGraphStage()) }, []string{"GraphStage"}},
		{"graph_stage.failed", func() biz.Event { return biz.NewGraphStageFailedEvent(wireTestGraphStage()) }, []string{"GraphStage"}},
		{"graph_stage.interrupted", func() biz.Event {
			return biz.NewGraphStageInterruptedEvent(wireTestGraphStage())
		}, []string{"GraphStage"}},
		{"graph_node.updated", func() biz.Event {
			return biz.NewGraphNodeUpdatedEvent(wireTestGraphStage().Nodes[0], "task-1", "sess-1")
		}, []string{"GraphNode"}},
		{"system.run_status", func() biz.Event {
			return biz.NewRunStatusEvent("sess-1", "run-1", "running", map[string]any{"k": "v"})
		}, []string{"RunID", "Status", "Meta"}},
		{"system.heartbeat", func() biz.Event {
			return biz.NewHeartbeatEventWithMeta("sess-1", "hb", wireTestTime, map[string]any{"k": "v"})
		}, []string{"Message", "Meta"}},
		{"system.notice", func() biz.Event {
			return biz.NewSystemNoticeEvent("sess-1", "cost_guard", "msg", map[string]any{"k": "v"})
		}, []string{"NoticeType", "Message", "Meta", "Seq"}},
		{"activity.bridge", func() biz.Event {
			return biz.NewActivityBridgeEvent(biz.ActivityEvent{
				Event:    biz.ActivityEventType("activity.updated"),
				Activity: biz.Activity{ID: "act-1", SessionID: "sess-1"},
			})
		}, []string{"Event"}},
	}
}

// TestV2EventPayloadToWire_GoldenParity locks byte-level parity between the
// wire serialization path and the legacy default-marshal path (the current
// production behavior the frontend depends on).
func TestV2EventPayloadToWire_GoldenParity(t *testing.T) {
	t.Parallel()
	for _, tc := range wireGoldenCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			domain := tc.buildDomain()
			want := mustMarshalJSON(t, domain)

			wire, err := v2EventPayloadToWire(domain)
			if err != nil {
				t.Fatalf("v2EventPayloadToWire: %v", err)
			}
			got := mustMarshalJSON(t, wire)

			if string(got) != string(want) {
				t.Errorf("wire payload mismatch\nwant: %s\ngot:  %s", want, got)
			}
		})
	}
}

// TestV2EventPayloadToWire_KeyContract explicitly locks the top-level payload
// key set per event kind, so future field drift fails loudly instead of
// silently changing the protocol consumed by the frontend.
func TestV2EventPayloadToWire_KeyContract(t *testing.T) {
	t.Parallel()
	for _, tc := range wireGoldenCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wire, err := v2EventPayloadToWire(tc.buildDomain())
			if err != nil {
				t.Fatalf("v2EventPayloadToWire: %v", err)
			}
			keys := topLevelKeys(t, mustMarshalJSON(t, wire))
			if len(keys) != len(tc.wantKeys) {
				t.Errorf("payload key count = %d, want %d (keys: %v)", len(keys), len(tc.wantKeys), keys)
			}
			for _, k := range tc.wantKeys {
				if !keys[k] {
					t.Errorf("payload missing key %q (keys: %v)", k, keys)
				}
			}
		})
	}
}

// TestV2EventPayloadToWire_UnknownEventFailsClosed verifies that an event
// type without an explicit wire mapping is rejected (fail-closed), instead of
// silently leaking domain internals via default marshaling.
func TestV2EventPayloadToWire_UnknownEventFailsClosed(t *testing.T) {
	t.Parallel()
	if _, err := v2EventPayloadToWire(&fakeUnmappedEvent{}); err == nil {
		t.Fatal("expected error for unmapped event type, got nil")
	}
}

// TestSanitizeRawJSON verifies that invalid JSON in ToolArgs/ToolResult is
// replaced with nil (serializes as null) instead of causing a marshal error
// that silently drops the step.updated event.
func TestSanitizeRawJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input json.RawMessage
		want  json.RawMessage
	}{
		{"nil", nil, nil},
		{"empty", json.RawMessage{}, nil},
		{"valid object", json.RawMessage(`{"a":1}`), json.RawMessage(`{"a":1}`)},
		{"valid string", json.RawMessage(`"hello"`), json.RawMessage(`"hello"`)},
		{"valid null", json.RawMessage(`null`), json.RawMessage(`null`)},
		{"invalid newline", json.RawMessage("line1\nline2"), nil},
		{"invalid truncated", json.RawMessage(`{"a":`), nil},
		{"invalid plain text", json.RawMessage(`not json`), nil},
		{"invalid trailing comma", json.RawMessage(`{"a":1,}`), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeRawJSON(tc.input)
			if string(got) != string(tc.want) {
				t.Errorf("sanitizeRawJSON(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestStepToWire_InvalidToolJSON verifies that a step with invalid JSON in
// ToolArgs/ToolResult can be marshaled to wire without error.
func TestStepToWire_InvalidToolJSON(t *testing.T) {
	t.Parallel()
	step := wireTestStep()
	step.ToolArgs = json.RawMessage("plain text\nwith newline")
	step.ToolResult = json.RawMessage(`{"truncated`)
	wire := stepToWire(step)
	// Must not panic; the result must be valid JSON when marshaled.
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal stepWire with invalid tool JSON: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal marshaled wire: %v", err)
	}
	if m["ToolArgs"] != nil {
		t.Errorf("ToolArgs should be null, got %v", m["ToolArgs"])
	}
	if m["ToolResult"] != nil {
		t.Errorf("ToolResult should be null, got %v", m["ToolResult"])
	}
}

// fakeUnmappedEvent is a biz.Event implementation with no wire mapping.
type fakeUnmappedEvent struct{}

func (e *fakeUnmappedEvent) EventKind() biz.EventKind { return "fake.unmapped" }
func (e *fakeUnmappedEvent) SpiritSessionID() string  { return "sess-1" }
func (e *fakeUnmappedEvent) TaskID() string           { return "task-1" }
func (e *fakeUnmappedEvent) EntityID() string         { return "x" }
func (e *fakeUnmappedEvent) OccurredAt() time.Time    { return wireTestTime }
