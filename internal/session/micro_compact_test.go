package session

import (
	"testing"

	"aranea-agents/internal/biz"
)

func makeMsg(role string, turn int, content string) biz.ChatMessage {
	return biz.ChatMessage{Role: role, TurnNumber: turn, ContentMarkdown: content}
}

func TestTryMicroCompact_emptyBody(t *testing.T) {
	r := tryMicroCompact(nil, 10)
	if r.didCompact {
		t.Fatal("empty body should not compact")
	}
}

func TestTryMicroCompact_noToolMessages(t *testing.T) {
	body := []biz.ChatMessage{
		makeMsg("user", 1, "hello"),
		makeMsg("assistant", 1, "hi"),
	}
	r := tryMicroCompact(body, 10)
	if r.didCompact {
		t.Fatal("no tool messages should not compact")
	}
}

func TestTryMicroCompact_shortToolResult(t *testing.T) {
	body := []biz.ChatMessage{
		makeMsg("tool", 1, "short"),
	}
	r := tryMicroCompact(body, 10)
	if r.didCompact {
		t.Fatal("short tool result should not compact")
	}
}

func TestTryMicroCompact_recentToolResult(t *testing.T) {
	longContent := strings200()
	body := []biz.ChatMessage{
		makeMsg("tool", 8, longContent),
	}
	r := tryMicroCompact(body, 9)
	if r.didCompact {
		t.Fatal("tool result within minAgeTurns should not compact")
	}
}

func TestTryMicroCompact_oldLongToolResult(t *testing.T) {
	longContent := strings200()
	body := []biz.ChatMessage{
		makeMsg("user", 1, "hello"),
		makeMsg("tool", 1, longContent),
		makeMsg("assistant", 2, "done"),
	}
	r := tryMicroCompact(body, 10)
	if !r.didCompact {
		t.Fatal("old long tool result should compact")
	}
	if r.fromTurn != 1 {
		t.Fatalf("fromTurn = %d, want 1", r.fromTurn)
	}
	if r.toTurn != 2 {
		t.Fatalf("toTurn = %d, want 2", r.toTurn)
	}
}

func TestTryMicroCompact_mixedMessages(t *testing.T) {
	longContent := strings200()
	body := []biz.ChatMessage{
		makeMsg("user", 1, "hello"),
		makeMsg("tool", 1, longContent),
		makeMsg("assistant", 2, "ok"),
		makeMsg("tool", 3, "short"),
		makeMsg("tool", 4, longContent),
	}
	r := tryMicroCompact(body, 10)
	if !r.didCompact {
		t.Fatal("should compact old long tool results")
	}
}

func TestMicroCompactEnabled_default(t *testing.T) {
	if !microCompactEnabled(biz.Agent{}) {
		t.Fatal("default should enable micro compact")
	}
}

func TestMicroCompactEnabled_withSettings(t *testing.T) {
	if !microCompactEnabled(biz.Agent{Settings: &biz.AgentRuntimeSettings{MicroCompactEnabled: true}}) {
		t.Fatal("explicitly enabled should return true")
	}
}

func TestMicroCompactEnabled_disabled(t *testing.T) {
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{MicroCompactEnabled: false}}
	if microCompactEnabled(ag) {
		t.Fatal("explicitly disabled should return false")
	}
}

func TestMicroCompactEnabled_compressOff(t *testing.T) {
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{L0SnapshotMode: "off", MicroCompactEnabled: true}}
	if microCompactEnabled(ag) {
		t.Fatal("compress off should disable micro compact")
	}
}

func strings200() string {
	s := make([]byte, 201)
	for i := range s {
		s[i] = 'x'
	}
	return string(s)
}
