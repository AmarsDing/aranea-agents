package contract

import (
	"testing"
)

func TestNewEnvelope(t *testing.T) {
	env := NewEnvelope(EnvelopeTypeTextDelta, "agent-1", "sess-1")
	if env.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if env.Type != EnvelopeTypeTextDelta {
		t.Fatalf("expected text_delta, got %q", env.Type)
	}
	if env.Author != "agent-1" {
		t.Fatalf("expected agent-1, got %q", env.Author)
	}
	if env.SessionID != "sess-1" {
		t.Fatalf("expected sess-1, got %q", env.SessionID)
	}
	if env.Version != 1 {
		t.Fatalf("expected version 1, got %d", env.Version)
	}
	if env.Timestamp == "" {
		t.Fatal("expected non-empty timestamp")
	}
}

func TestNewEnvelope_UniqueIDs(t *testing.T) {
	a := NewEnvelope(EnvelopeTypeTextDelta, "a", "s")
	b := NewEnvelope(EnvelopeTypeTextDelta, "a", "s")
	if a.ID == b.ID {
		t.Fatal("each envelope should have unique ID")
	}
}

func TestRouteChannel_Monitor(t *testing.T) {
	for _, typ := range []EnvelopeType{EnvelopeTypeLog, EnvelopeTypeFlowLog} {
		if ch := RouteChannel(Envelope{Type: typ}); ch != "monitor" {
			t.Fatalf("expected monitor for %q, got %q", typ, ch)
		}
	}
}

func TestRouteChannel_Team(t *testing.T) {
	for _, typ := range []EnvelopeType{
		EnvelopeTypeMemberMessageStart, EnvelopeTypeMemberDelta, EnvelopeTypeMemberMessageDone,
		EnvelopeTypeTeamRunStarted, EnvelopeTypeTeamRunFinished, EnvelopeTypeTeamStepStarted,
		EnvelopeTypeTeamStepFinished, EnvelopeTypeTeamRunFailed, EnvelopeTypeTeamSummary,
		EnvelopeTypeOrchestrationAgentStatus,
	} {
		if ch := RouteChannel(Envelope{Type: typ}); ch != "team" {
			t.Fatalf("expected team for %q, got %q", typ, ch)
		}
	}
}

func TestRouteChannel_Graph(t *testing.T) {
	for _, typ := range []EnvelopeType{
		EnvelopeTypeGraphNodeStart, EnvelopeTypeGraphNodeEnd, EnvelopeTypeCheckpoint,
		EnvelopeTypeGraphStep, EnvelopeTypeGraphExecutionDone, EnvelopeTypeGraphNodeError,
		EnvelopeTypeGraphNodeCustom, EnvelopeTypeGraphTaskStatus,
	} {
		if ch := RouteChannel(Envelope{Type: typ}); ch != "graph" {
			t.Fatalf("expected graph for %q, got %q", typ, ch)
		}
	}
}

func TestRouteChannel_Knowledge(t *testing.T) {
	if ch := RouteChannel(Envelope{Type: EnvelopeTypeKnowledgeIngest}); ch != "knowledge" {
		t.Fatalf("expected knowledge, got %q", ch)
	}
}

func TestRouteChannel_MCP(t *testing.T) {
	for _, typ := range []EnvelopeType{EnvelopeTypeMCPSessionReconnect, EnvelopeTypeMCPHealthAlert, EnvelopeTypeAlertNotify} {
		if ch := RouteChannel(Envelope{Type: typ}); ch != "monitor" {
			t.Fatalf("expected monitor for %q, got %q", typ, ch)
		}
	}
}

func TestRouteChannel_DefaultWithTeamID(t *testing.T) {
	if ch := RouteChannel(Envelope{Type: EnvelopeTypeTextDelta, TeamID: "team-1"}); ch != "team" {
		t.Fatalf("expected team, got %q", ch)
	}
}

func TestRouteChannel_DefaultChat(t *testing.T) {
	if ch := RouteChannel(Envelope{Type: EnvelopeTypeTextDelta}); ch != "chat" {
		t.Fatalf("expected chat, got %q", ch)
	}
}

func TestMatchFilterKey_EmptyKeys(t *testing.T) {
	if !MatchFilterKey("", "abc") {
		t.Fatal("empty subscriber key should match")
	}
	if !MatchFilterKey("abc", "") {
		t.Fatal("empty event key should match")
	}
	if !MatchFilterKey("", "") {
		t.Fatal("both empty should match")
	}
}

func TestMatchFilterKey_SameKey(t *testing.T) {
	if !MatchFilterKey("sess-1", "sess-1") {
		t.Fatal("same key should match")
	}
}

func TestMatchFilterKey_ParentPrefix(t *testing.T) {
	if !MatchFilterKey("sess-1/turn-1", "sess-1") {
		t.Fatal("child should match parent")
	}
	if !MatchFilterKey("sess-1", "sess-1/turn-1") {
		t.Fatal("parent should match child")
	}
}

func TestMatchFilterKey_NoMatch(t *testing.T) {
	if MatchFilterKey("sess-1", "sess-2") {
		t.Fatal("different keys should not match")
	}
}

func TestEnvelope_Clone(t *testing.T) {
	orig := Envelope{
		ID:        "e-1",
		Type:      EnvelopeTypeTextDelta,
		Author:    "agent-1",
		SessionID: "sess-1",
		Content:   &EnvelopeContent{Text: "hello"},
		ToolCall:  &EnvelopeToolCall{ID: "tc-1", Name: "search"},
		Error:     &EnvelopeError{Type: "runtime", Message: "oops"},
		Extensions: map[string]string{"key": "val"},
		Metadata:   map[string]any{"count": 42},
	}
	clone := orig.Clone()
	if clone.ID != orig.ID {
		t.Fatal("ID should be copied")
	}
	clone.Content.Text = "changed"
	if orig.Content.Text == "changed" {
		t.Fatal("Content should be deep copied")
	}
	clone.Extensions["key"] = "changed"
	if orig.Extensions["key"] == "changed" {
		t.Fatal("Extensions should be deep copied")
	}
	clone.Metadata["count"] = 99
	if orig.Metadata["count"] == 99 {
		t.Fatal("Metadata should be deep copied")
	}
}

func TestEnvelope_Clone_NilFields(t *testing.T) {
	orig := Envelope{ID: "e-1", Type: EnvelopeTypeTextDelta}
	clone := orig.Clone()
	if clone.Content != nil {
		t.Fatal("nil Content should remain nil")
	}
	if clone.Extensions != nil {
		t.Fatal("nil Extensions should remain nil")
	}
}

func TestEnvelope_ContainsTag_Exact(t *testing.T) {
	env := Envelope{Tag: "phase_start"}
	if !env.ContainsTag("phase_start") {
		t.Fatal("exact tag should match")
	}
}

func TestEnvelope_ContainsTag_CommaList(t *testing.T) {
	env := Envelope{Tag: "phase_start,phase_succeeded"}
	if !env.ContainsTag("phase_succeeded") {
		t.Fatal("should find tag in comma list")
	}
	if env.ContainsTag("phase_failed") {
		t.Fatal("should not find absent tag")
	}
}

func TestEnvelope_ContainsTag_Empty(t *testing.T) {
	env := Envelope{Tag: "phase_start"}
	if env.ContainsTag("") {
		t.Fatal("empty tag should not match non-empty Tag")
	}
	env2 := Envelope{Tag: ""}
	if env2.ContainsTag("phase_start") {
		t.Fatal("empty Tag should not match non-empty tag")
	}
}

func TestRouteChannel_AllTypes(t *testing.T) {
	allTypes := []EnvelopeType{
		EnvelopeTypeTextDelta, EnvelopeTypeTextDone, EnvelopeTypeToolCall,
		EnvelopeTypeToolResult, EnvelopeTypeStateDelta, EnvelopeTypeTransfer,
		EnvelopeTypeRunnerCompletion, EnvelopeTypeContextUsage, EnvelopeTypeRunStatus,
		EnvelopeTypeError, EnvelopeTypeLog, EnvelopeTypeFlowLog,
		EnvelopeTypeGraphNodeStart, EnvelopeTypeGraphNodeEnd, EnvelopeTypeCheckpoint,
		EnvelopeTypeIntentPass, EnvelopeTypeMemberMessageStart, EnvelopeTypeMemberDelta,
		EnvelopeTypeMemberMessageDone, EnvelopeTypeTeamRunStarted, EnvelopeTypeTeamRunFinished,
		EnvelopeTypeTeamStepStarted, EnvelopeTypeTeamStepFinished, EnvelopeTypeTeamRunFailed,
		EnvelopeTypeTeamSummary, EnvelopeTypeGraphStep, EnvelopeTypeGraphExecutionDone,
		EnvelopeTypeGraphNodeError, EnvelopeTypeGraphNodeCustom, EnvelopeTypeGraphTaskStatus,
		EnvelopeTypeKnowledgeIngest, EnvelopeTypeMCPSessionReconnect, EnvelopeTypeMCPHealthAlert,
		EnvelopeTypeAlertNotify, EnvelopeTypeOrchestrationAgentStatus, EnvelopeTypeUserFeedback,
		EnvelopeTypeSessionStatusChanged, EnvelopeTypeExecutionProgress,
	}
	validChannels := map[string]bool{"chat": true, "monitor": true, "team": true, "graph": true, "knowledge": true}
	for _, typ := range allTypes {
		ch := RouteChannel(Envelope{Type: typ})
		if !validChannels[ch] {
			t.Fatalf("unexpected channel %q for type %q", ch, typ)
		}
	}
}

func TestMatchFilterKey_PrefixNotPartial(t *testing.T) {
	if MatchFilterKey("sess-12", "sess-1") {
		t.Fatal("sess-12 should not match sess-1 (not prefix with slash)")
	}
}

func TestMatchFilterKey_SlashPrefix(t *testing.T) {
	if !MatchFilterKey("sess-1/turn-2/step-3", "sess-1/turn-2") {
		t.Fatal("sess-1/turn-2/step-3 should match sess-1/turn-2")
	}
	if !MatchFilterKey("sess-1/turn-2", "sess-1/turn-2/step-3") {
		t.Fatal("sess-1/turn-2 should match sess-1/turn-2/step-3")
	}
}
