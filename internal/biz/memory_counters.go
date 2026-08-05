package biz

import (
	"context"
	"time"
)

// ── FR-12.6: three-stage fact counters (report §6.5) ─────────────────────
//
// use_count was semantically wrong: it incremented on recall regardless of
// whether the fact was ever written into a prompt. The three-stage counters
// separate the funnel precisely:
//
//	recalled_count  — entered a recall result set (data layer, recall path)
//	injected_count  — passed filters+budget and was written into the prompt
//	                  (before-model hook; the only "usage" count for users)
//	cited_count     — explicitly referenced by the assistant reply
//	                  (citation backfill worker, heuristic + dedup ledger)

// MemoryFactInjectCounter bumps injected_count for facts actually written
// into the LLM prompt. Implemented by the data layer (l3FactRepo); consumed
// by the agent before-model hook once per turn.
// Stability:evolving
type MemoryFactInjectCounter interface {
	IncrementFactInjectedCount(ctx context.Context, factIDs []string) error
}

// FactCitation is one (fact, turn) citation pair detected by the citation
// backfill worker.
type FactCitation struct {
	FactID string
	TurnID string
}

// MemoryFactCitationRecorder records citations into the dedup ledger and
// increments cited_count for first-seen (fact, turn) pairs only.
// Stability:evolving
type MemoryFactCitationRecorder interface {
	RecordFactCitations(ctx context.Context, citations []FactCitation) error
}

// CitationFactRef is a candidate fact from one memory_recalled notice,
// carrying the full statement for heuristic citation matching.
type CitationFactRef struct {
	FactID    string
	Statement string
}

// CitationCandidate bundles one turn's injected facts and the assistant
// reply text for citation detection.
type CitationCandidate struct {
	TurnID    string
	ReplyText string
	Facts     []CitationFactRef
}

// MemoryCitationTraceReader loads recent memory_recalled notices joined
// with their turn's assistant reply and full fact statements. Implemented
// by the data layer over steps_v2 + memory_facts.
// Stability:evolving
type MemoryCitationTraceReader interface {
	// ListCitationCandidates returns candidates from memory_recalled notices
	// created at or after since, newest first, capped at limit notices.
	// Candidates with no parseable facts or no reply text are skipped.
	ListCitationCandidates(ctx context.Context, since time.Time, limit int) ([]CitationCandidate, error)
}
