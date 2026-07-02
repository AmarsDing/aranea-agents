package biz

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSpiritSessionStatus_Constants(t *testing.T) {
	if SpiritSessionStatusActive != "active" {
		t.Fatalf("expected active, got %s", SpiritSessionStatusActive)
	}
	if SpiritSessionStatusCompleted != "completed" {
		t.Fatalf("expected completed, got %s", SpiritSessionStatusCompleted)
	}
}

func TestTaskStatus_Constants(t *testing.T) {
	cases := []TaskStatus{
		TaskStatusPending, TaskStatusRunning, TaskStatusCompleted,
		TaskStatusFailed, TaskStatusCancelled,
	}
	expected := []string{"pending", "running", "completed", "failed", "cancelled"}
	for i, c := range cases {
		if string(c) != expected[i] {
			t.Fatalf("TaskStatus[%d]: expected %s, got %s", i, expected[i], c)
		}
	}
}

func TestStepKind_Constants(t *testing.T) {
	if StepKindThinking != "thinking" {
		t.Fatalf("expected thinking, got %s", StepKindThinking)
	}
}

func TestPlanStep_Fields(t *testing.T) {
	now := time.Now().UTC()
	ps := PlanStep{
		ID:        "ps-1",
		PlanID:    "pb-1",
		TaskID:    "t-1",
		Label:     "Step 1",
		DependsOn: []string{"ps-0"},
		Status:    PlanStepStatusPending,
		Seq:       1,
	}
	if ps.ID != "ps-1" || ps.PlanID != "pb-1" || ps.TaskID != "t-1" {
		t.Fatalf("PlanStep field mismatch: %+v", ps)
	}
	if len(ps.DependsOn) != 1 || ps.DependsOn[0] != "ps-0" {
		t.Fatalf("DependsOn mismatch: %v", ps.DependsOn)
	}
	_ = now // 验证 time.Time 类型可用
}

func TestStepResult_JSONRoundtrip(t *testing.T) {
	r := StepResult{
		Output: "team output",
		MemberReports: []MemberReport{
			{AgentKey: "k1", AgentName: "Agent 1", Output: "out1"},
		},
		TokensUsed: TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
		DurationMs: 5000,
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got StepResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Output != "team output" || got.TokensUsed.TotalTokens != 150 {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}

func TestMemberInfo_Fields(t *testing.T) {
	mi := MemberInfo{
		AgentKey:       "agent-1",
		AgentName:      "Worker",
		AvatarURL:      "https://example.com/a.png",
		ChildSessionID: "sess-child-1",
		Status:         "pending",
	}
	if mi.AgentKey != "agent-1" || mi.AvatarURL == "" {
		t.Fatalf("MemberInfo mismatch: %+v", mi)
	}
}
