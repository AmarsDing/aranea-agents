package jobs

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/memory"
	"aranea-agents/pkg/loggateway"
)

const (
	memoryEbbinghausDecayDefaultInterval = 24 * time.Hour
	// memoryEbbinghausDecayBatchSize is the max number of fact rows scanned
	// per agent per tick. Keeps each tick bounded; large agents will be
	// processed across multiple ticks.
	memoryEbbinghausDecayBatchSize = 500
	// memoryEbbinghausDecayThreshold is the R_t value below which a fact is
	// considered "heavily decayed" (low reachability). Facts below this
	// threshold are counted in the decayed_facts statistic.
	memoryEbbinghausDecayThreshold = 0.3
)

// MemoryEbbinghausDecayWorker periodically computes Ebbinghaus exponential
// decay (R_t = exp(-n_t / S_t)) for stored memories and persists the score
// so fused recall can down-weight forgotten memories without recomputing R_t
// on every recall.
//
// This worker is the persistence companion to MemoryL3DecayWorker. The L3
// decay worker owns importance updates; this worker owns the decay_score
// column (the Ebbinghaus reachability R_t). On each tick it:
//   - scans memories (per-agent, via L3FactReader),
//   - computes Ebbinghaus reachability R_t for each fact,
//   - writes R_t back to the DB via DecayScoreWriter (batch, transactional),
//   - logs how many memories have decayed below a threshold.
//
// When no memory reader is wired (reader == nil), the worker logs startup and
// ticks at the configured interval as a no-op. When no writer is wired
// (writer == nil), the worker computes statistics but skips the writeback
// (statistics-only mode, for backward compatibility).
//
// The worker integrates with the unified Job framework (JobRunner) for retry,
// panic recovery, and dead-letter metrics.
type MemoryEbbinghausDecayWorker struct {
	interval   time.Duration
	calculator *memory.EbbinghausDecayCalculator
	reader     biz.L3FactReader   // for scanning fact rows (DB read)
	writer     biz.DecayScoreWriter // for persisting R_t back to DB (optional)
	agents     *biz.AgentUsecase  // for ListMemoryMaintenanceTargets
	runner     *JobRunner         // unified Job framework (retry + dead letter)
	lg         loggateway.Logger
}

// NewMemoryEbbinghausDecayWorker creates a MemoryEbbinghausDecayWorker.
//
// Parameters:
//   - interval:   tick interval. Falls back to 24h when <= 0.
//   - calculator: the Ebbinghaus decay calculator. When nil, a default one is created.
//   - reader:     L3 fact reader for scanning memories. When nil, the worker
//     runs as a no-op (statistics-only mode disabled).
//   - writer:     Decay score writer for persisting R_t. When nil, the worker
//     computes statistics but skips the writeback (statistics-only mode).
//   - agents:     AgentUsecase for listing memory maintenance targets. When nil,
//     the worker falls back to a global scan (if reader supports it).
//   - lg:         logger. Falls back to a no-op logger when nil.
func NewMemoryEbbinghausDecayWorker(
	interval time.Duration,
	calculator *memory.EbbinghausDecayCalculator,
	reader biz.L3FactReader,
	writer biz.DecayScoreWriter,
	agents *biz.AgentUsecase,
	lg loggateway.Logger,
) *MemoryEbbinghausDecayWorker {
	if interval <= 0 {
		interval = memoryEbbinghausDecayDefaultInterval
	}
	if calculator == nil {
		calculator = memory.NewEbbinghausDecayCalculator()
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &MemoryEbbinghausDecayWorker{
		interval:   interval,
		calculator: calculator,
		reader:     reader,
		writer:     writer,
		agents:     agents,
		runner:     NewJobRunner(lg),
		lg:         lg,
	}
}

// Start blocks until ctx is cancelled. It logs startup, then ticks at the
// configured interval. Each tick scans memories (when reader is wired) and
// computes Ebbinghaus decay statistics.
func (w *MemoryEbbinghausDecayWorker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	w.lg.Info("ebbinghaus decay worker started",
		loggateway.Any("interval", w.interval.String()),
		loggateway.Bool("reader_wired", w.reader != nil),
		loggateway.Bool("agents_wired", w.agents != nil))

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.lg.Info("ebbinghaus decay worker stopped")
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

// runOnce executes a single tick via the unified Job framework.
// When reader is not wired, the tick is a no-op.
//
// The error from JobRunner.Run is intentionally discarded: JobRunner already
// logs failures and increments dead-letter metrics internally, and this is a
// background worker with no caller to propagate errors to.
func (w *MemoryEbbinghausDecayWorker) runOnce(ctx context.Context) {
	if w.reader == nil {
		w.lg.Debug("ebbinghaus decay tick skipped: reader not wired")
		return
	}

	_ = w.runner.Run(ctx, JobConfig{
		JobID:      "memory_ebbinghaus_decay",
		MaxRetries: DefaultJobMaxRetries,
	}, func(ctx context.Context) error {
		return w.scanAndCompute(ctx)
	})
}

// scanAndCompute scans memories and computes Ebbinghaus decay statistics.
//
// When agents is wired, scans per-agent (only agents with WriteL3Facts=true).
// When agents is nil, falls back to a global scan via ListFactRows with empty
// scope (all active facts).
//
// Statistics logged per tick:
//   - total_facts: total facts scanned
//   - decayed_facts: facts with R_t < memoryEbbinghausDecayThreshold
//   - avg_decay: average R_t across all scanned facts
//   - agents: number of agents scanned (per-agent mode only)
func (w *MemoryEbbinghausDecayWorker) scanAndCompute(ctx context.Context) error {
	now := time.Now().UTC()

	if w.agents != nil {
		return w.scanPerAgent(ctx, now)
	}
	return w.scanGlobal(ctx, now)
}

// scanPerAgent scans memories for each agent with WriteL3Facts=true.
func (w *MemoryEbbinghausDecayWorker) scanPerAgent(ctx context.Context, now time.Time) error {
	targets, err := w.agents.ListMemoryMaintenanceTargets(ctx)
	if err != nil {
		w.lg.Warn("ebbinghaus decay: list maintenance targets failed", loggateway.Err(err))
		return err
	}

	var totalFacts, decayedFacts int
	var sumDecay float64
	// Collect fact ID → R_t for batch writeback at the end of the tick.
	scores := make(map[string]float64)

	for _, t := range targets {
		if !t.WriteL3Facts {
			continue
		}
		rows, _, _, _, err := w.reader.ListFactRows(ctx, "agent", t.AgentID, "", "active", "",
			int32(memoryEbbinghausDecayBatchSize), 0)
		if err != nil {
			w.lg.Warn("ebbinghaus decay: list facts failed",
				loggateway.Str("agent_id", t.AgentID),
				loggateway.Err(err))
			continue
		}

		for _, row := range rows {
			factID, decay, parseErr := w.computeDecayForRow(row, now)
			if parseErr != nil {
				continue
			}
			totalFacts++
			sumDecay += decay
			if decay < memoryEbbinghausDecayThreshold {
				decayedFacts++
			}
			if factID != "" {
				scores[factID] = decay
			}
		}
	}

	w.logStatistics(totalFacts, decayedFacts, sumDecay, len(targets))
	w.writebackDecayScores(ctx, scores)
	return nil
}

// scanGlobal scans all active memories in a single batch (fallback when
// agents is not wired).
func (w *MemoryEbbinghausDecayWorker) scanGlobal(ctx context.Context, now time.Time) error {
	rows, _, _, _, err := w.reader.ListFactRows(ctx, "", "", "", "active", "",
		int32(memoryEbbinghausDecayBatchSize), 0)
	if err != nil {
		w.lg.Warn("ebbinghaus decay: global list facts failed", loggateway.Err(err))
		return err
	}

	var totalFacts, decayedFacts int
	var sumDecay float64
	scores := make(map[string]float64)

	for _, row := range rows {
		factID, decay, parseErr := w.computeDecayForRow(row, now)
		if parseErr != nil {
			continue
		}
		totalFacts++
		sumDecay += decay
		if decay < memoryEbbinghausDecayThreshold {
			decayedFacts++
		}
		if factID != "" {
			scores[factID] = decay
		}
	}

	w.logStatistics(totalFacts, decayedFacts, sumDecay, 0)
	w.writebackDecayScores(ctx, scores)
	return nil
}

// computeDecayForRow parses a fact row JSON and computes the Ebbinghaus decay.
//
// The fact row JSON is produced by scanFactRowJSON in internal/data/memory_helpers.go
// and includes: id, created_at, updated_at, last_used_at, use_count.
//
// DecayInput uses last_used_at (falls back to updated_at, then created_at)
// as LastUsedAt, and use_count as AccessCount.
//
// Returns (factID, R_t, error). factID may be empty if the row JSON does not
// contain an "id" field; the caller should skip writeback for empty IDs.
func (w *MemoryEbbinghausDecayWorker) computeDecayForRow(row []byte, now time.Time) (string, float64, error) {
	var fact struct {
		ID         string `json:"id"`
		CreatedAt  string `json:"created_at"`
		UpdatedAt  string `json:"updated_at"`
		LastUsedAt string `json:"last_used_at"`
		UseCount   int32  `json:"use_count"`
	}
	if err := json.Unmarshal(row, &fact); err != nil {
		return "", 0, err
	}

	createdAt, _ := time.Parse(time.RFC3339, fact.CreatedAt)
	lastUsedAt, _ := time.Parse(time.RFC3339, fact.LastUsedAt)
	if lastUsedAt.IsZero() {
		// Fall back to updated_at when last_used_at is empty.
		lastUsedAt, _ = time.Parse(time.RFC3339, fact.UpdatedAt)
	}

	decay := w.calculator.ComputeDecay(memory.DecayInput{
		LastUsedAt:  lastUsedAt,
		CreatedAt:   createdAt,
		AccessCount: int(fact.UseCount),
		Now:         now,
	})
	return fact.ID, decay, nil
}

// writebackDecayScores persists the computed R_t scores via DecayScoreWriter.
// When the writer is nil or the scores map is empty, this is a no-op.
// Writeback failures are logged but do not fail the tick — the decay score
// is a best-effort optimization for fused recall ranking, not a correctness
// requirement.
func (w *MemoryEbbinghausDecayWorker) writebackDecayScores(ctx context.Context, scores map[string]float64) {
	if w.writer == nil || len(scores) == 0 {
		return
	}
	if err := w.writer.UpdateDecayScores(ctx, scores); err != nil {
		w.lg.Warn("ebbinghaus decay: writeback failed",
			loggateway.Int("fact_count", len(scores)),
			loggateway.Err(err))
		return
	}
	w.lg.Debug("ebbinghaus decay: writeback ok",
		loggateway.Int("fact_count", len(scores)))
}

// logStatistics logs the decay statistics when there are facts to report.
func (w *MemoryEbbinghausDecayWorker) logStatistics(totalFacts, decayedFacts int, sumDecay float64, agentCount int) {
	if totalFacts == 0 {
		return
	}
	avgDecay := sumDecay / float64(totalFacts)
	w.lg.Info("ebbinghaus decay statistics",
		loggateway.Int("total_facts", totalFacts),
		loggateway.Int("decayed_facts", decayedFacts),
		loggateway.Float64("avg_decay", avgDecay),
		loggateway.Float64("threshold", memoryEbbinghausDecayThreshold),
		loggateway.Int("agents", agentCount))
}

// MemoryEbbinghausDecayDisabled reports whether the Ebbinghaus decay worker
// is disabled via the MEMORY_EBBINGHAUS_DECAY_DISABLED environment variable.
func MemoryEbbinghausDecayDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_EBBINGHAUS_DECAY_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
