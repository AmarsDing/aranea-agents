package biz

import "context"

// ContextCompressor is the port for L0 context-window compression. It
// encapsulates the decision of whether the context window is approaching its
// limit and the recursive-summarisation logic that produces a compact summary
// of evicted messages.
//
// Implementation lives in internal/memory (LLM-driven). The interface itself
// stays in biz so that hooks and tooling can depend on the contract without
// pulling in trpc-agent-go model types.
//
// Stability: evolving
type ContextCompressor interface {
	// ShouldCompress reports whether the context window usage ratio has
	// crossed the compression threshold (default 0.80 — see
	// llmcontext.ContextStatusCriticalThreshold).
	ShouldCompress(usedRatio float64) bool

	// Compress produces a recursive summary by feeding the existing summary
	// (if any) together with the evicted messages to an LLM. Callers are
	// responsible for selecting which messages to evict; Compress only
	// turns them into a summary.
	//
	// Behaviour contract:
	//   - empty evictedMessages → returns an empty ContextCompressionResult, no LLM call
	//   - nil LLM (implementation not wired) → returns an error
	//   - LLM failure → returns the error (caller decides graceful degradation)
	//   - empty LLM response → returns an empty Summary with metrics populated
	Compress(ctx context.Context, existingSummary string, evictedMessages []ConsolidateMessage) (ContextCompressionResult, error)
}

// ContextCompressionResult is the output of an L0 compression pass. It carries
// the freshly generated recursive summary plus char-level metrics so callers
// can persist an L0 Assembly Snapshot row for observability.
//
// Note: this is distinct from CompactResult (used by ManualCompressor for the
// /compact session-compaction command). The two serve different layers:
//   - ContextCompressionResult: L0 automatic summarisation (this file)
//   - CompactResult:             session-level manual compaction (manual_compressor.go)
type ContextCompressionResult struct {
	// Summary is the freshly generated (or merged) recursive summary.
	Summary string
	// EvictedCount is the number of messages that were folded into the summary.
	EvictedCount int
	// BeforeChars is the total character count of the evicted messages plus
	// the existing summary (if any). Useful for compression-ratio observability.
	BeforeChars int
	// AfterChars is len(Summary). Convenience field so callers don't have to
	// re-measure.
	AfterChars int
}
