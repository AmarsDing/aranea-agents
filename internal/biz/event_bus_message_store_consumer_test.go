package biz

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTeamMemberMessageIDDeterministic(t *testing.T) {
	a := teamMemberMessageID("agent_b", "env-1", "hello", "2026-05-21T00:00:00Z")
	b := teamMemberMessageID("agent_b", "env-1", "hello", "2026-05-21T00:00:00Z")
	if a != b {
		t.Fatalf("expected stable id, got %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "msg-team-agent_b-env-1") {
		t.Fatalf("unexpected id %q", a)
	}
	fallback := teamMemberMessageID("agent_b", "", "hello", "2026-05-21T00:00:00Z")
	if fallback == a {
		t.Fatal("expected different id when envelope id empty")
	}
}

func TestTeamMemberOptionsJSONShape(t *testing.T) {
	raw := map[string]any{
		"schema":           teamMemberOptionsSchema,
		"team_member":      map[string]any{"agent_key": "agent_b", "name": "agent_b", "role": ""},
		"member_agent_key": "agent_b",
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["schema"] != teamMemberOptionsSchema {
		t.Fatalf("schema %v", parsed["schema"])
	}
	tm, ok := parsed["team_member"].(map[string]any)
	if !ok || tm["agent_key"] != "agent_b" {
		t.Fatalf("team_member %#v", parsed["team_member"])
	}
}
