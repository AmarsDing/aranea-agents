package intent

import (
	"testing"
	"time"

	"aranea-agents/internal/biz"
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

func TestShouldRun(t *testing.T) {
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{IntentPassEnabled: true}}
	if ShouldRun(ag, "hi") {
		t.Fatal("short message should skip")
	}
	if !ShouldRun(ag, "please refactor the auth middleware tests") {
		t.Fatal("long message should run when enabled")
	}
	ag.Settings.IntentPassEnabled = false
	if ShouldRun(ag, "please refactor the auth middleware tests") {
		t.Fatal("disabled agent should skip")
	}
	// P1-1: Agent without settings defaults to intent pass ON.
	noSettingsAg := biz.Agent{Settings: nil}
	if !ShouldRun(noSettingsAg, "please refactor the auth middleware tests") {
		t.Fatal("agent without settings should run intent pass by default")
	}
}

// TestIntentPassFromAgent_DefaultOn verifies P1-1: intent pass defaults to ON
// when an agent has no settings, while still respecting an explicit OFF.
func TestIntentPassFromAgent_DefaultOn(t *testing.T) {
	if !IntentPassFromAgent(biz.Agent{Settings: nil}) {
		t.Fatal("agent without settings should default to intent pass ON")
	}
	if !IntentPassFromAgent(biz.Agent{Settings: &biz.AgentRuntimeSettings{IntentPassEnabled: true}}) {
		t.Fatal("agent with IntentPassEnabled=true should be ON")
	}
	if IntentPassFromAgent(biz.Agent{Settings: &biz.AgentRuntimeSettings{IntentPassEnabled: false}}) {
		t.Fatal("agent with explicit IntentPassEnabled=false should be OFF")
	}
}

func TestIntentSystemForAgent(t *testing.T) {
	coding := biz.Agent{
		SystemPromptMode: "complete",
		Settings:         &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "full"},
	}
	if IntentSystemForAgent(coding) != intentSystemCoding {
		t.Fatal("expected coding template")
	}
	general := biz.Agent{
		SystemPromptMode: "complete",
		Settings:         &biz.AgentRuntimeSettings{ToolsEnabled: false},
	}
	if IntentSystemForAgent(general) != intentSystemGeneral {
		t.Fatal("expected general template")
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
