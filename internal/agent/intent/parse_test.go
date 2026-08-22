package intent

import (
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestParseArtifactJSON_ToolHints(t *testing.T) {
	text := `{"refined_goal":"patch login","intent_kind":"code_change","tool_hints":["diff_edit","search_content"]}`
	art, _ := parseArtifactJSON(text)
	if art == nil {
		t.Fatal("expected artifact")
	}
	if len(art.ToolHints) != 2 || art.ToolHints[0] != "diff_edit" {
		t.Fatalf("tool_hints = %v", art.ToolHints)
	}
}

func TestParseArtifactJSON_Valid(t *testing.T) {
	text := `{"refined_goal":"fix the login bug","intent_kind":"debug"}`
	art, raw := parseArtifactJSON(text)
	if art == nil {
		t.Fatal("expected non-nil Artifact")
	}
	if art.RefinedGoal != "fix the login bug" {
		t.Errorf("RefinedGoal = %q, want %q", art.RefinedGoal, "fix the login bug")
	}
	if art.IntentKind != "debug" {
		t.Errorf("IntentKind = %q, want %q", art.IntentKind, "debug")
	}
	if raw == "" {
		t.Error("expected non-empty raw JSON")
	}
}

func TestParseArtifactJSON_Empty(t *testing.T) {
	art, raw := parseArtifactJSON("")
	if art != nil {
		t.Errorf("expected nil, got %+v", art)
	}
	if raw != "" {
		t.Errorf("raw = %q, want empty", raw)
	}
}

func TestParseArtifactJSON_NoRefinedGoal(t *testing.T) {
	art, _ := parseArtifactJSON(`{"intent_kind":"debug"}`)
	if art != nil {
		t.Errorf("expected nil when refined_goal is empty, got %+v", art)
	}
}

func TestParseArtifactJSON_WithExtraText(t *testing.T) {
	text := "Some text {\"refined_goal\":\"fix bug\",\"intent_kind\":\"debug\"} more text"
	art, _ := parseArtifactJSON(text)
	if art == nil {
		t.Fatal("expected non-nil Artifact")
	}
	if art.RefinedGoal != "fix bug" {
		t.Errorf("RefinedGoal = %q, want %q", art.RefinedGoal, "fix bug")
	}
}

func TestParseArtifactJSON_InvalidJSON(t *testing.T) {
	art, _ := parseArtifactJSON("not json at all")
	if art != nil {
		t.Errorf("expected nil for invalid JSON, got %+v", art)
	}
}

func TestStripFences_NoFence(t *testing.T) {
	got := stripFences("hello")
	if got != "hello" {
		t.Errorf("stripFences(%q) = %q, want %q", "hello", got, "hello")
	}
}

func TestStripFences_JSONFence(t *testing.T) {
	input := "```json\n{\"a\":1}\n```"
	got := stripFences(input)
	want := `{"a":1}`
	if got != want {
		t.Errorf("stripFences(%q) = %q, want %q", input, got, want)
	}
}

func TestStripFences_PlainFence(t *testing.T) {
	input := "```\ncode\n```"
	got := stripFences(input)
	if got != "code" {
		t.Errorf("stripFences(%q) = %q, want %q", input, got, "code")
	}
}

func TestWrapUserMessage_NilArtifact(t *testing.T) {
	got := WrapUserMessage("hello world", nil)
	if got != "hello world" {
		t.Errorf("WrapUserMessage with nil artifact = %q, want %q", got, "hello world")
	}
}

func TestWrapUserMessage_WithArtifact(t *testing.T) {
	art := &Artifact{RefinedGoal: "fix bug", IntentKind: "debug"}
	got := WrapUserMessage("hello", art)
	if !strings.Contains(got, "hello") {
		t.Error("result should contain original message")
	}
	artJSON, _ := json.Marshal(art)
	if !strings.Contains(got, string(artJSON)) {
		t.Error("result should contain artifact JSON")
	}
}

func TestMergeIntoUserOptionsJSON_EmptyArtifact(t *testing.T) {
	opts := `{"key":"value"}`
	got, err := MergeIntoUserOptionsJSON(opts, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != opts {
		t.Errorf("MergeIntoUserOptionsJSON with empty artifact = %q, want %q", got, opts)
	}
}

func TestMergeIntoUserOptionsJSON_Valid(t *testing.T) {
	opts := `{"key":"value"}`
	artifact := `{"refined_goal":"fix bug"}`
	got, err := MergeIntoUserOptionsJSON(opts, artifact)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if _, ok := m["intent_artifact"]; !ok {
		t.Error("expected intent_artifact key in merged result")
	}
	if _, ok := m["key"]; !ok {
		t.Error("expected original key to be preserved")
	}
}

func TestMergeIntoUserOptionsJSON_InvalidOptionsJSON(t *testing.T) {
	_, err := MergeIntoUserOptionsJSON("not json", `{"refined_goal":"x"}`)
	if err == nil {
		t.Error("expected error for invalid options JSON")
	}
}

func TestMergeIntoUserOptionsJSON_EmptyOptions(t *testing.T) {
	artifact := `{"refined_goal":"fix bug"}`
	got, err := MergeIntoUserOptionsJSON("", artifact)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if _, ok := m["intent_artifact"]; !ok {
		t.Error("expected intent_artifact key in result")
	}
}

func TestIntentPassFromAgent_NilSettings(t *testing.T) {
	// P1-1: intent pass defaults to ON when Settings is nil.
	ag := biz.Agent{}
	if !IntentPassFromAgent(ag) {
		t.Error("expected true (default ON) when Settings is nil")
	}
}

func TestIntentPassFromAgent_Enabled(t *testing.T) {
	ag := biz.Agent{
		Settings: &biz.AgentRuntimeSettings{IntentPassEnabled: true},
	}
	if !IntentPassFromAgent(ag) {
		t.Error("expected true when IntentPassEnabled is true")
	}
}

func TestMonitorLogEntry_Completed(t *testing.T) {
	r := RunResult{Outcome: "completed"}
	level, _ := MonitorLogEntry(r, "test", RunMeta{})
	if level != "INFO" {
		t.Errorf("level = %q, want %q", level, "INFO")
	}
}

func TestMonitorLogEntry_SkippedLLM(t *testing.T) {
	r := RunResult{Outcome: "skipped_llm"}
	level, _ := MonitorLogEntry(r, "test", RunMeta{})
	if level != "WARN" {
		t.Errorf("level = %q, want %q", level, "WARN")
	}
}

func TestMonitorLogEntry_SkippedParse(t *testing.T) {
	r := RunResult{Outcome: "skipped_parse"}
	level, _ := MonitorLogEntry(r, "test", RunMeta{})
	if level != "WARN" {
		t.Errorf("level = %q, want %q", level, "WARN")
	}
}

func TestMonitorLogEntry_WithMeta(t *testing.T) {
	r := RunResult{Outcome: "completed"}
	meta := RunMeta{SessionID: "sess1", TeamID: "team1", AgentID: "ag1"}
	_, msg := MonitorLogEntry(r, "test", meta)
	if !strings.Contains(msg, "session_id=sess1") {
		t.Error("msg should contain session_id")
	}
	if !strings.Contains(msg, "team_id=team1") {
		t.Error("msg should contain team_id")
	}
	if !strings.Contains(msg, "agent_id=ag1") {
		t.Error("msg should contain agent_id")
	}
}
