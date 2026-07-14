package team

import (
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

func TestParseDefinition_enableStateDeliverable(t *testing.T) {
	// Explicitly enabled
	raw := `{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[]}`
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !def.EnableStateDeliverable {
		t.Fatal("expected EnableStateDeliverable=true when JSON sets it")
	}

	// Default false when omitted
	def2, err := ParseDefinition(`{"version":1,"mode":"sequential","members":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if def2.EnableStateDeliverable {
		t.Fatal("expected EnableStateDeliverable=false by default")
	}

	// Explicitly false
	def3, err := ParseDefinition(`{"version":1,"mode":"sequential","enable_state_deliverable":false,"members":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if def3.EnableStateDeliverable {
		t.Fatal("expected EnableStateDeliverable=false when JSON sets it false")
	}
}
