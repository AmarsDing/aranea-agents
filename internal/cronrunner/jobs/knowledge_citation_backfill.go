package jobs

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

const (
	// KnowledgeCitationBackfillDefaultInterval is the default scan period (10m).
	KnowledgeCitationBackfillDefaultInterval = 10 * time.Minute
	// knowledgeCitationBackfillWindow is the lookback window scanned per pass.
	// Windows overlap across passes; the dedup ledger (knowledge_chunk_citations)
	// makes re-scanning idempotent.
	knowledgeCitationBackfillWindow = time.Hour
	// knowledgeCitationBackfillBatch caps the notices loaded per pass.
	knowledgeCitationBackfillBatch = 200
)

// KnowledgeCitationBackfillWorker detects knowledge chunks explicitly
// referenced by the assistant reply and increments cited_count (29-token P2-2:
// the knowledge-side counterpart of MemoryCitationBackfillWorker). It scans
// recent knowledge_recalled notices (chunks returned by knowledge_search /
// knowledge_reflect that turn), joins each to the turn's final reply, and
// applies the same two heuristics as the memory side:
//
//  1. ID reference — the reply contains the chunk ID's first 8 chars.
//  2. Text overlap — ≥50% of the content's 8-rune k-grams appear in the reply.
//
// Recording is idempotent via the knowledge_chunk_citations dedup ledger, so
// overlapping windows are safe. All failures are best-effort: a failed pass is
// logged and retried on the next tick.
type KnowledgeCitationBackfillWorker struct {
	interval time.Duration
	window   time.Duration
	reader   bizknowledge.KnowledgeCitationTraceReader
	recorder bizknowledge.KnowledgeChunkCitationRecorder
	lg       loggateway.Logger
}

// NewKnowledgeCitationBackfillWorker creates a KnowledgeCitationBackfillWorker.
// interval <= 0 falls back to the 10m default. Returns nil when reader or
// recorder is nil.
func NewKnowledgeCitationBackfillWorker(
	interval time.Duration,
	reader bizknowledge.KnowledgeCitationTraceReader,
	recorder bizknowledge.KnowledgeChunkCitationRecorder,
	lg loggateway.Logger,
) *KnowledgeCitationBackfillWorker {
	if reader == nil || recorder == nil {
		return nil
	}
	if interval <= 0 {
		interval = KnowledgeCitationBackfillDefaultInterval
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &KnowledgeCitationBackfillWorker{
		interval: interval,
		window:   knowledgeCitationBackfillWindow,
		reader:   reader,
		recorder: recorder,
		lg:       lg.With(loggateway.Domain("knowledge_citation_backfill")),
	}
}

// Start blocks until ctx is cancelled, scanning on every tick.
func (w *KnowledgeCitationBackfillWorker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	w.lg.Info("knowledge citation backfill worker started",
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
func (w *KnowledgeCitationBackfillWorker) RunOnce(ctx context.Context) {
	if w == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			w.lg.Error("knowledge citation backfill panic recovered",
				loggateway.StepID("knowledge.citation_backfill"),
				loggateway.Any("panic", r))
		}
	}()
	since := time.Now().Add(-w.window)
	candidates, err := w.reader.ListKnowledgeCitationCandidates(ctx, since, knowledgeCitationBackfillBatch)
	if err != nil {
		w.lg.Warn("citation candidates load failed",
			loggateway.StepID("knowledge.citation_backfill"),
			loggateway.Err(err))
		return
	}
	if len(candidates) == 0 {
		return
	}
	var citations []bizknowledge.ChunkCitation
	for _, cand := range candidates {
		replyNorm := normalizeCitationText(cand.ReplyText)
		if replyNorm == "" {
			continue
		}
		for _, chunk := range cand.Chunks {
			if chunkCited(replyNorm, chunk) {
				citations = append(citations, bizknowledge.ChunkCitation{ChunkID: chunk.ChunkID, TurnID: cand.TurnID})
			}
		}
	}
	if len(citations) == 0 {
		return
	}
	if err := w.recorder.RecordChunkCitations(ctx, citations); err != nil {
		w.lg.Warn("citation record failed",
			loggateway.StepID("knowledge.citation_backfill"),
			loggateway.Err(err))
		return
	}
	w.lg.Info("knowledge citation backfill pass completed",
		loggateway.StepID("knowledge.citation_backfill"),
		loggateway.Int("candidates", len(candidates)),
		loggateway.Int("citations_detected", len(citations)))
}

// chunkCited reports whether the reply explicitly references the chunk. The
// heuristics are identical to the memory side (ID reference + k-gram overlap),
// so it delegates to factCited rather than duplicating the matching logic.
func chunkCited(replyNorm string, chunk bizknowledge.CitationChunkRef) bool {
	if chunk.N > 0 {
		marker := "[" + strconv.Itoa(chunk.N) + "]"
		if strings.Contains(replyNorm, marker) {
			return true
		}
	}
	return factCited(replyNorm, biz.CitationFactRef{FactID: chunk.ChunkID, Statement: chunk.Content})
}

// KnowledgeCitationBackfillDisabled reports whether the citation backfill
// worker is disabled via the KNOWLEDGE_CITATION_BACKFILL_DISABLED env var.
func KnowledgeCitationBackfillDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("KNOWLEDGE_CITATION_BACKFILL_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
