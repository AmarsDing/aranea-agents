package jobs

import (
	"context"
	"os"
	"strings"
	"time"
	"unicode"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

const (
	// MemoryCitationBackfillDefaultInterval is the default scan period (10m).
	MemoryCitationBackfillDefaultInterval = 10 * time.Minute
	// memoryCitationBackfillWindow is the lookback window scanned per pass.
	// Windows overlap across passes; the dedup ledger (memory_fact_citations)
	// makes re-scanning idempotent.
	memoryCitationBackfillWindow = time.Hour
	// memoryCitationBackfillBatch caps the notices loaded per pass.
	memoryCitationBackfillBatch = 200

	// citationGramRunes is the sliding-window size (runes) for statement↔reply
	// overlap matching. Chinese statements have no whitespace, so segment
	// splitting does not work — k-gram containment tolerates paraphrase.
	citationGramRunes = 8
	// citationGramMinRunes is the minimum statement length (runes) for a
	// citation signal; shorter statements are too ambiguous to match.
	citationGramMinRunes = 4
	// citationGramHitRatio is the fraction of statement k-grams that must
	// appear in the reply for the fact to count as cited.
	citationGramHitRatio = 0.5
)

// MemoryCitationBackfillWorker detects facts explicitly referenced by the
// assistant reply and increments cited_count (FR-12.6: the "cited" stage of
// the three-stage counters). It scans recent memory_recalled notices (the
// facts injected that turn), joins each to the turn's final reply, and
// applies two heuristics:
//
//  1. ID reference — the reply contains the fact's short provenance ID
//     (the prompt renders "[id:abc12345, ...]" when L3InjectProvenance is
//     on; an LLM echoing that reference is a certain citation).
//  2. Text overlap — ≥50% of the statement's 8-rune k-grams appear in the
//     reply (paraphrase-tolerant for CJK text without word boundaries).
//
// Recording is idempotent via the memory_fact_citations dedup ledger, so
// overlapping windows are safe. All failures are best-effort: a failed pass
// is logged and retried on the next tick.
type MemoryCitationBackfillWorker struct {
	interval time.Duration
	window   time.Duration
	reader   biz.MemoryCitationTraceReader
	recorder biz.MemoryFactCitationRecorder
	lg       loggateway.Logger
}

// NewMemoryCitationBackfillWorker creates a MemoryCitationBackfillWorker.
// interval <= 0 falls back to the 10m default. Returns nil when reader or
// recorder is nil.
func NewMemoryCitationBackfillWorker(
	interval time.Duration,
	reader biz.MemoryCitationTraceReader,
	recorder biz.MemoryFactCitationRecorder,
	lg loggateway.Logger,
) *MemoryCitationBackfillWorker {
	if reader == nil || recorder == nil {
		return nil
	}
	if interval <= 0 {
		interval = MemoryCitationBackfillDefaultInterval
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &MemoryCitationBackfillWorker{
		interval: interval,
		window:   memoryCitationBackfillWindow,
		reader:   reader,
		recorder: recorder,
		lg:       lg.With(loggateway.Domain("memory_citation_backfill")),
	}
}

// Start blocks until ctx is cancelled, scanning on every tick.
func (w *MemoryCitationBackfillWorker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	w.lg.Info("memory citation backfill worker started",
		loggateway.Str("interval", w.interval.String()))
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.RunOnce(ctx)
		}
	}
}

// RunOnce executes one scan pass synchronously. Panics are recovered locally
// so a broken pass can never kill the worker loop.
func (w *MemoryCitationBackfillWorker) RunOnce(ctx context.Context) {
	if w == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			w.lg.Error("memory citation backfill panic recovered",
				loggateway.StepID("memory.citation_backfill"),
				loggateway.Any("panic", r))
		}
	}()
	since := time.Now().Add(-w.window)
	candidates, err := w.reader.ListCitationCandidates(ctx, since, memoryCitationBackfillBatch)
	if err != nil {
		w.lg.Warn("citation candidates load failed",
			loggateway.StepID("memory.citation_backfill"),
			loggateway.Err(err))
		return
	}
	if len(candidates) == 0 {
		return
	}
	var citations []biz.FactCitation
	for _, cand := range candidates {
		replyNorm := normalizeCitationText(cand.ReplyText)
		if replyNorm == "" {
			continue
		}
		for _, fact := range cand.Facts {
			if factCited(replyNorm, fact) {
				citations = append(citations, biz.FactCitation{FactID: fact.FactID, TurnID: cand.TurnID})
			}
		}
	}
	if len(citations) == 0 {
		return
	}
	if err := w.recorder.RecordFactCitations(ctx, citations); err != nil {
		w.lg.Warn("citation record failed",
			loggateway.StepID("memory.citation_backfill"),
			loggateway.Err(err))
		return
	}
	w.lg.Info("memory citation backfill pass completed",
		loggateway.StepID("memory.citation_backfill"),
		loggateway.Int("candidates", len(candidates)),
		loggateway.Int("citations_detected", len(citations)))
}

// factCited reports whether the reply explicitly references the fact, per
// the ID-reference and text-overlap heuristics.
func factCited(replyNorm string, fact biz.CitationFactRef) bool {
	// Heuristic 1: provenance ID reference. The prompt renders the first 8
	// chars of the fact ID ("[id:abc12345, ...]"); a reply echoing that
	// short ID is a certain citation.
	id := strings.TrimSpace(fact.FactID)
	if len(id) >= 8 {
		if strings.Contains(replyNorm, strings.ToLower(id[:8])) {
			return true
		}
	}
	// Heuristic 2: k-gram overlap between statement and reply.
	stmtRunes := []rune(normalizeCitationText(fact.Statement))
	if len(stmtRunes) < citationGramMinRunes {
		return false
	}
	if len(stmtRunes) <= citationGramRunes {
		return strings.Contains(replyNorm, string(stmtRunes))
	}
	hits := 0
	grams := len(stmtRunes) - citationGramRunes + 1
	for i := 0; i < grams; i++ {
		if strings.Contains(replyNorm, string(stmtRunes[i:i+citationGramRunes])) {
			hits++
		}
	}
	return float64(hits)/float64(grams) >= citationGramHitRatio
}

// normalizeCitationText lowercases and collapses whitespace for matching.
func normalizeCitationText(s string) string {
	return strings.ToLower(strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r)
	}), " "))
}

// MemoryCitationBackfillDisabled reports whether the citation backfill
// worker is disabled via the MEMORY_CITATION_BACKFILL_DISABLED env var.
func MemoryCitationBackfillDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_CITATION_BACKFILL_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
