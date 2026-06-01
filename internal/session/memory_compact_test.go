package session

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type stubMemoryFactReader struct {
	facts []biz.MemoryFactEntry
	err   error
}

func (s *stubMemoryFactReader) ReadSessionMemoryFacts(_ context.Context, _ string) ([]biz.MemoryFactEntry, error) {
	return s.facts, s.err
}

func TestTryMemoryCompact_emptyBody(t *testing.T) {
	r := tryMemoryCompact(context.Background(), nil, &stubMemoryFactReader{}, "s1", loggateway.NewNoop())
	if r.didCompact {
		t.Fatal("empty body should not compact")
	}
}

func TestTryMemoryCompact_nilReader(t *testing.T) {
	body := []biz.ChatMessage{makeMsg("user", 1, "hello")}
	r := tryMemoryCompact(context.Background(), body, nil, "s1", loggateway.NewNoop())
	if r.didCompact {
		t.Fatal("nil reader should not compact")
	}
}

func TestTryMemoryCompact_readerError(t *testing.T) {
	body := []biz.ChatMessage{makeMsg("user", 1, "hello")}
	r := tryMemoryCompact(context.Background(), body, &stubMemoryFactReader{err: errors.New("db down")}, "s1", loggateway.NewNoop())
	if r.didCompact {
		t.Fatal("reader error should not compact")
	}
}

func TestTryMemoryCompact_noFacts(t *testing.T) {
	body := []biz.ChatMessage{makeMsg("user", 1, "hello")}
	r := tryMemoryCompact(context.Background(), body, &stubMemoryFactReader{facts: nil}, "s1", loggateway.NewNoop())
	if r.didCompact {
		t.Fatal("no facts should not compact")
	}
}

func TestTryMemoryCompact_withFacts(t *testing.T) {
	body := []biz.ChatMessage{
		makeMsg("user", 1, "hello"),
		makeMsg("assistant", 2, "hi"),
	}
	facts := []biz.MemoryFactEntry{
		{Statement: "user prefers Go", Scope: "static", Confidence: 0.9},
		{Statement: "project uses Kratos", Scope: "dynamic", Confidence: 0.8},
	}
	r := tryMemoryCompact(context.Background(), body, &stubMemoryFactReader{facts: facts}, "s1", loggateway.NewNoop())
	if !r.didCompact {
		t.Fatal("facts present should compact")
	}
	if r.fromTurn != 1 || r.toTurn != 2 {
		t.Fatalf("turns = %d–%d, want 1–2", r.fromTurn, r.toTurn)
	}
	if r.summaryMarkdown == "" {
		t.Fatal("summary should not be empty")
	}
}

func TestTryMemoryCompact_factWithScope(t *testing.T) {
	body := []biz.ChatMessage{makeMsg("user", 1, "hello")}
	facts := []biz.MemoryFactEntry{
		{Statement: "fact A", Scope: "static", Confidence: 0.9},
		{Statement: "fact B", Scope: "", Confidence: 0.7},
	}
	r := tryMemoryCompact(context.Background(), body, &stubMemoryFactReader{facts: facts}, "s1", loggateway.NewNoop())
	if !r.didCompact {
		t.Fatal("should compact")
	}
}

func TestMemoryCompactEnabled_default(t *testing.T) {
	if !memoryCompactEnabled(biz.Agent{}) {
		t.Fatal("default should enable memory compact")
	}
}

func TestMemoryCompactEnabled_withSettings(t *testing.T) {
	if !memoryCompactEnabled(biz.Agent{Settings: &biz.AgentRuntimeSettings{MemoryCompactEnabled: true}}) {
		t.Fatal("explicitly enabled should return true")
	}
}

func TestMemoryCompactEnabled_disabled(t *testing.T) {
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{MemoryCompactEnabled: false}}
	if memoryCompactEnabled(ag) {
		t.Fatal("explicitly disabled should return false")
	}
}

func TestMemoryCompactEnabled_compressOff(t *testing.T) {
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{L0SnapshotMode: "off", MemoryCompactEnabled: true}}
	if memoryCompactEnabled(ag) {
		t.Fatal("compress off should disable memory compact")
	}
}
