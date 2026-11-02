package data

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestSplitEvalMessagesByAtomBudget(t *testing.T) {
	// Newlines are atom breakers: each message yields a known atom count.
	msgs := []biz.EvalMessage{
		{Content: "A one\nA two\nA three"},                    // 3 atoms
		{Content: "B one\nB two"},                             // 2 atoms
		{Content: "C one\nC two\nC three\nC four"},            // 4 atoms
		{Content: "D one"},                                    // 1 atom
		{Content: "E one\nE two\nE three\nE four\nE five"},    // 5 atoms
	}
	t.Run("budget zero keeps single chunk", func(t *testing.T) {
		chunks := splitEvalMessagesByAtomBudget(msgs, 0)
		if len(chunks) != 1 || len(chunks[0]) != len(msgs) {
			t.Fatalf("expected single chunk, got %d", len(chunks))
		}
	})
	t.Run("messages never split across chunks", func(t *testing.T) {
		chunks := splitEvalMessagesByAtomBudget(msgs, 4)
		// Greedy by atom count 3,2,4,1,5 with budget 4: each message overflows
		// the previous accumulation, so every message lands in its own chunk.
		want := [][]int{{0}, {1}, {2}, {3}, {4}}
		if len(chunks) != len(want) {
			t.Fatalf("chunk count = %d, want %d", len(chunks), len(want))
		}
		for ci, w := range want {
			if len(chunks[ci]) != len(w) || chunks[ci][0].Content != msgs[w[0]].Content {
				t.Fatalf("chunk %d = %+v, want messages %v", ci, chunks[ci], w)
			}
		}
	})
	t.Run("all fit in one chunk", func(t *testing.T) {
		chunks := splitEvalMessagesByAtomBudget(msgs, 100)
		if len(chunks) != 1 {
			t.Fatalf("expected 1 chunk, got %d", len(chunks))
		}
	})
	t.Run("empty input", func(t *testing.T) {
		if chunks := splitEvalMessagesByAtomBudget(nil, 5); len(chunks) != 0 {
			t.Fatalf("expected no chunks, got %d", len(chunks))
		}
	})
}

// Chunked commits must be outcome-equivalent to the legacy single
// transaction: same stored count, same live atom facts, and a
// platform-style retry (re-POST of the same batch) stays deduplicated.
// Derived projection rows (digests/rollups) churn more under chunking
// (one rotation per chunk) — that is expected and excluded here.
func TestEvalStore_ChunkedAddEquivalentAndRetrySafe(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	s := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	var msgs []biz.EvalMessage
	for i := 0; i < 12; i++ {
		msgs = append(msgs, biz.EvalMessage{
			Role: "user", Content: fmt.Sprintf("My test attribute %02d has value %02d. It stayed stable.", i, i),
			MessageID: fmt.Sprint(i), Timestamp: fmt.Sprintf("2024-03-%02dT09:00:00Z", i+1),
		})
	}
	liveAtoms := func(userID string) map[string]int {
		out := map[string]int{}
		for _, f := range evalC3AtomFacts(t, r, userID) {
			if v, _ := f["valid_until"].(string); v != "" {
				continue
			}
			stmt, _ := f["statement"].(string)
			out[stmt]++
		}
		return out
	}
	t.Setenv("EVAL_ADD_CHUNK_ATOMS", "3")
	stored, err := s.AddMessages(ctx, "chunked", "s", msgs)
	if err != nil {
		t.Fatal(err)
	}
	if stored != len(msgs) {
		t.Fatalf("stored = %d, want %d", stored, len(msgs))
	}
	chunkedLive := liveAtoms("chunked")
	if len(chunkedLive) == 0 {
		t.Fatal("chunked add wrote nothing")
	}
	// Platform retry: same batch re-POSTed; fingerprints dedupe everything.
	stored, err = s.AddMessages(ctx, "chunked", "s", msgs)
	if err != nil {
		t.Fatal(err)
	}
	if stored != len(msgs) {
		t.Fatalf("retry stored = %d, want %d", stored, len(msgs))
	}
	if got := liveAtoms("chunked"); !maps.Equal(got, chunkedLive) {
		t.Fatalf("retry changed live atoms: %v -> %v", chunkedLive, got)
	}
	// Whole-transaction path (knob off) agrees on live atom outcomes.
	t.Setenv("EVAL_ADD_CHUNK_ATOMS", "0")
	if _, err := s.AddMessages(ctx, "whole", "s", msgs); err != nil {
		t.Fatal(err)
	}
	if wholeLive := liveAtoms("whole"); !maps.Equal(wholeLive, chunkedLive) {
		t.Fatalf("chunked live atoms != whole-transaction:\nchunked=%v\nwhole=%v", chunkedLive, wholeLive)
	}
}

// Coding-track seeds embed raw file bytes (NUL included) in message content.
// PostgreSQL text columns reject 0x00 (pq 22021); the ingest must strip them
// instead of failing the whole transaction deterministically on retry.
func TestEvalStore_NULByteContentSanitized(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	s := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	msgs := []biz.EvalMessage{
		{Role: "user", Content: "Binary header bytes PK\x00\x00\x03\x04 seen in archive.\nTrailing NUL here\x00", MessageID: "n1", Timestamp: "2024-03-01T09:00:00Z"},
	}
	if _, err := s.AddMessages(ctx, "nuluser", "s", msgs); err != nil {
		t.Fatalf("AddMessages with NUL content: %v", err)
	}
	facts := evalC3AtomFacts(t, r, "nuluser")
	if len(facts) == 0 {
		t.Fatal("expected facts from NUL-bearing content")
	}
	for _, f := range facts {
		stmt, _ := f["statement"].(string)
		if strings.ContainsRune(stmt, 0) {
			t.Fatalf("statement retains NUL: %q", stmt)
		}
	}
	// Retry stays deduplicated on the sanitized text.
	if _, err := s.AddMessages(ctx, "nuluser", "s", msgs); err != nil {
		t.Fatal(err)
	}
	if got := len(evalC3AtomFacts(t, r, "nuluser")); got != len(facts) {
		t.Fatalf("retry changed fact count: %d -> %d", len(facts), got)
	}
}
