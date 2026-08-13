package session

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// P0 review fix: tryMemoryCompact used to accept ANY non-empty fact set,
// letting a near-empty memory summary replace the whole conversation body in
// the persisted snapshot (effectively unrecoverable context loss). The ICS
// coverage score was already computed but never gated on. L2 must now meet a
// minimum coverage (memoryCompactMinICS) or the cascade escalates to the LLM
// compressor.
func TestTryMemoryCompact_icsGate(t *testing.T) {
	ctx := context.Background()
	lg := loggateway.NewNoop()
	body := []biz.ChatMessage{
		makeMsg("user", 1, "hello"),
		makeMsg("assistant", 1, "hi"),
	}

	t.Run("sparse_facts_rejected", func(t *testing.T) {
		// Single intent fact → ICS = 0.25 < 0.5 → must NOT compact.
		facts := []biz.MemoryFactEntry{
			{Statement: "build the API", Scope: "intent", Confidence: 0.9},
		}
		r := tryMemoryCompact(ctx, body, &stubMemoryFactReader{facts: facts}, nil, "s1", lg)
		if r.didCompact {
			t.Fatal("sparse facts (ICS < minICS) should not compact")
		}
	})

	t.Run("unscoped_facts_rejected", func(t *testing.T) {
		// Unknown scopes only → FactCount=2 → ICS = 0.05 < 0.5.
		facts := []biz.MemoryFactEntry{
			{Statement: "user prefers Go", Scope: "static", Confidence: 0.9},
			{Statement: "project uses Kratos", Scope: "dynamic", Confidence: 0.8},
		}
		r := tryMemoryCompact(ctx, body, &stubMemoryFactReader{facts: facts}, nil, "s1", lg)
		if r.didCompact {
			t.Fatal("unscoped sparse facts (ICS < minICS) should not compact")
		}
	})

	t.Run("covered_facts_accepted", func(t *testing.T) {
		// intent(0.25) + state(0.20) + pending(0.10) + 2 decisions(0.20) = 0.75 ≥ 0.5.
		facts := []biz.MemoryFactEntry{
			{Statement: "build the API", Scope: "intent"},
			{Statement: "schema drafted", Scope: "state"},
			{Statement: "write tests", Scope: "pending"},
			{Statement: "chose Kratos", Scope: "decision"},
			{Statement: "chose Postgres", Scope: "decision"},
		}
		r := tryMemoryCompact(ctx, body, &stubMemoryFactReader{facts: facts}, nil, "s1", lg)
		if !r.didCompact {
			t.Fatal("covered facts (ICS >= minICS) should compact")
		}
		if r.summaryMarkdown == "" {
			t.Fatal("summary should not be empty")
		}
	})

	t.Run("boundary_exactly_min_accepted", func(t *testing.T) {
		// intent(0.25) + state(0.20) + fact(0.05, count1) = 0.50 → accept (>=).
		facts := []biz.MemoryFactEntry{
			{Statement: "build the API", Scope: "intent"},
			{Statement: "schema drafted", Scope: "state"},
			{Statement: "misc note", Scope: "other"},
		}
		r := tryMemoryCompact(ctx, body, &stubMemoryFactReader{facts: facts}, nil, "s1", lg)
		if !r.didCompact {
			t.Fatal("ICS exactly at minICS should compact (>= comparison)")
		}
	})
}
