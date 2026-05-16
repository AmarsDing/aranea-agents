package intent

import (
	"testing"
	"time"
)

func TestPassEffective(t *testing.T) {
	cases := []struct {
		env    string
		agent  bool
		expect bool
	}{
		{"", true, true},
		{"", false, false},
		{"1", false, true},
		{"0", true, false},
		{"false", true, false},
		{"off", true, false},
		{"yes", false, true},
		{"maybe", true, true},
		{"maybe", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.env+"_"+boolStr(tc.agent), func(t *testing.T) {
			t.Setenv("ARANEA_INTENT_PASS", tc.env)
			if g := PassEffective(tc.agent); g != tc.expect {
				t.Fatalf("PassEffective(agent=%v) with env %q: got %v want %v", tc.agent, tc.env, g, tc.expect)
			}
		})
	}
}

func boolStr(b bool) string {
	if b {
		return "t"
	}
	return "f"
}

func TestBuildIntentPassPayload(t *testing.T) {
	r := RunResult{
		Artifact: &Artifact{IntentKind: "debug", RefinedGoal: "fix bug", SearchHints: []string{"a", "b"}},
		Outcome:  "completed",
		Duration: 42 * time.Millisecond,
	}
	p := BuildIntentPassPayload(r, RunMeta{AgentID: "ag1", SessionID: "s1", RunID: "r1", TeamID: "t1"})
	if p["outcome"] != "completed" {
		t.Fatalf("outcome: %v", p["outcome"])
	}
	if p["duration_ms"] != int64(42) {
		t.Fatalf("duration_ms: %v", p["duration_ms"])
	}
	if p["intent_kind"] != "debug" {
		t.Fatalf("intent_kind: %v", p["intent_kind"])
	}
	if p["search_hints_count"] != 2 {
		t.Fatalf("search_hints_count: %v", p["search_hints_count"])
	}
	if p["session_id"] != "s1" || p["team_id"] != "t1" {
		t.Fatalf("ids: %#v", p)
	}
}
