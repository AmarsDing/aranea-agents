package team

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEnabledMembers_sortOrder(t *testing.T) {
	d := Definition{
		Members: []MemberDef{
			{AgentID: "c", Role: "worker", SortOrder: 30},
			{AgentID: "a", Role: "worker", SortOrder: 10},
			{AgentID: "b", Role: "worker", SortOrder: 20},
		},
	}
	got := EnabledMembers(d)
	if len(got) != 3 || got[0].AgentID != "a" || got[1].AgentID != "b" || got[2].AgentID != "c" {
		t.Fatalf("unexpected order: %#v", got)
	}
}

func TestEnabledMembers_sortOrderZeroPreservesSliceOrder(t *testing.T) {
	d := Definition{
		Members: []MemberDef{
			{AgentID: "first", Role: "worker"},
			{AgentID: "second", Role: "worker"},
		},
	}
	got := EnabledMembers(d)
	if len(got) != 2 || got[0].AgentID != "first" || got[1].AgentID != "second" {
		t.Fatalf("unexpected order: %#v", got)
	}
}

func TestLoopMaxIterations_criticLoopFromJSON(t *testing.T) {
	raw := `{"version":1,"mode":"critic_loop","members":[],"critic_loop":{"max_iterations":5}}`
	var def Definition
	if err := json.Unmarshal([]byte(raw), &def); err != nil {
		t.Fatal(err)
	}
	if got := loopMaxIterations("critic_loop", def); got != 5 {
		t.Fatalf("got %d want 5", got)
	}
}

func TestLoopMaxIterations_criticLoopCap(t *testing.T) {
	def := Definition{CriticLoop: &CriticLoopConfig{MaxIterations: 99}}
	if got := loopMaxIterations("critic_loop", def); got != 32 {
		t.Fatalf("got %d want 32", got)
	}
}

func TestLoopMaxIterations_coordinatorCap(t *testing.T) {
	def := Definition{TimeoutSeconds: 180}
	if got := loopMaxIterations("coordinator", def); got != 3 {
		t.Fatalf("got %d want 3 (default outer iterations when loop_max_iterations unset)", got)
	}
	if got := loopMaxIterations("adaptive", def); got != 3 {
		t.Fatalf("adaptive default: got %d want 3", got)
	}
}

func TestLoopMaxIterations_coordinatorOverride(t *testing.T) {
	def := Definition{LoopMaxIterations: 7}
	if got := loopMaxIterations("coordinator", def); got != 7 {
		t.Fatalf("got %d want 7", got)
	}
}

func TestTurnDeadlineDuration_clamped(t *testing.T) {
	if got := TurnDeadlineDuration(Definition{TimeoutSeconds: 30}); got != 120*time.Second {
		t.Fatalf("got %v want 120s", got)
	}
	if got := TurnDeadlineDuration(Definition{TimeoutSeconds: 9000}); got != 7200*time.Second {
		t.Fatalf("got %v want 7200s", got)
	}
	if got := TurnDeadlineDuration(Definition{TimeoutSeconds: 0}); got != 0 {
		t.Fatalf("got %v want 0", got)
	}
}

func TestParseDefinition_intentAnchorAgentID(t *testing.T) {
	raw := `{"version":1,"mode":"sequential","intent_anchor_agent_id":"agent-uuid-1","members":[]}`
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	if def.IntentAnchorAgentID != "agent-uuid-1" {
		t.Fatalf("got %q", def.IntentAnchorAgentID)
	}
}

func TestChunkParallelWorkers(t *testing.T) {
	w := []MemberDef{{AgentID: "a"}, {AgentID: "b"}, {AgentID: "c"}, {AgentID: "d"}}
	ch := chunkParallelWorkers(w, 2)
	if len(ch) != 2 || len(ch[0]) != 2 || len(ch[1]) != 2 {
		t.Fatalf("got %#v", ch)
	}
	ch = chunkParallelWorkers(w, 0)
	if len(ch) != 1 || len(ch[0]) != 4 {
		t.Fatalf("want single batch, got %#v", ch)
	}
}
