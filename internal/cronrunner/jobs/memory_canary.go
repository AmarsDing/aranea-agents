package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

const (
	// MemoryCanaryDefaultInterval is the default canary loop period (30m).
	MemoryCanaryDefaultInterval = 30 * time.Minute
	// MemoryCanaryMinScore is the production default L3 recall threshold
	// (agent_runtime_settings.l3_recall_min_score default). The canary recalls
	// with exactly this value so a regression in score computation (Bug A:
	// Scores.Total = 0) is caught as a recall-stage failure.
	MemoryCanaryMinScore = 0.55

	memoryCanaryScopeType = "canary"
	memoryCanaryScopeID   = "memory-canary"

	canaryStageWrite   = "write"
	canaryStageRecall  = "recall"
	canaryStageArchive = "archive"
)

// MemoryCanaryWorker periodically proves the memory closed loop works
// end-to-end: it writes a synthetic fact through the production consolidation
// upsert path, recalls it through the production L3 recall path at the
// production default minScore, then invalidates it (bi-temporal archive) and
// asserts it disappears. Any stage failure records the failure in
// biz.MemoryCanaryStatus (alert metric) and emits a flow-log alarm.
//
// Background: the 2026-07 memory outage (6 bugs across write/store/recall/
// inject) survived because no code asserted "a written fact must come back".
// This worker is that assertion.
type MemoryCanaryWorker struct {
	interval time.Duration
	writer   biz.MemoryConsolidationWriter
	reader   biz.L3FactReader
	facts    biz.L3FactWriter
	status   *biz.MemoryCanaryStatus
	flowLog  biz.FlowLogWriter
	lg       loggateway.Logger
}

func NewMemoryCanaryWorker(
	interval time.Duration,
	writer biz.MemoryConsolidationWriter,
	reader biz.L3FactReader,
	facts biz.L3FactWriter,
	status *biz.MemoryCanaryStatus,
	flowLog biz.FlowLogWriter,
	lg loggateway.Logger,
) *MemoryCanaryWorker {
	if interval <= 0 {
		interval = MemoryCanaryDefaultInterval
	}
	return &MemoryCanaryWorker{
		interval: interval,
		writer:   writer,
		reader:   reader,
		facts:    facts,
		status:   status,
		flowLog:  flowLog,
		lg:       lg,
	}
}

func (w *MemoryCanaryWorker) Start(ctx context.Context) {
	if w == nil || w.writer == nil || w.reader == nil || w.facts == nil {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.RunOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.RunOnce(ctx)
		}
	}
}

// RunOnce executes one write → recall → archive loop synchronously (the
// ticker goroutine already provides the async context). Panics are recovered
// locally so a broken probe can never kill the worker loop, and are recorded
// as failures — a panicking canary must look exactly like a broken pipeline.
func (w *MemoryCanaryWorker) RunOnce(ctx context.Context) {
	if w == nil || w.writer == nil || w.reader == nil || w.facts == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			w.lg.Error("memory canary panic recovered",
				loggateway.StepID("memory.canary"),
				loggateway.Any("panic", r))
			w.status.RecordFail("panic", fmt.Sprintf("%v", r))
		}
	}()
	w.runLoop(ctx)
}

func (w *MemoryCanaryWorker) runLoop(ctx context.Context) {
	start := time.Now()
	token := "FALCON-" + randomHex(4)

	// Stage 1 — write through the production consolidation upsert path.
	factID, err := w.writeCanaryFact(ctx, token)
	if err != nil {
		w.fail(canaryStageWrite, err, time.Since(start))
		return
	}

	// Stage 2 — recall through the production L3 path at production minScore.
	if err := w.assertRecall(ctx, token); err != nil {
		// Best-effort cleanup so failed probes don't accumulate residue.
		w.cleanupFact(ctx, factID)
		w.fail(canaryStageRecall, err, time.Since(start))
		return
	}

	// Stage 3 — invalidate (bi-temporal archive) and assert disappearance.
	if err := w.assertArchived(ctx, factID, token); err != nil {
		w.fail(canaryStageArchive, err, time.Since(start))
		return
	}

	w.status.RecordOK()
	w.lg.Info("memory canary passed",
		loggateway.StepID("memory.canary"),
		loggateway.Str("fact_id", factID),
		loggateway.Int64("duration_ms", time.Since(start).Milliseconds()))
}

// writeCanaryFact upserts the synthetic fact via the same batch path used by
// auto-memory consolidation (the path that carried Bug F's 42702 ambiguity).
func (w *MemoryCanaryWorker) writeCanaryFact(ctx context.Context, token string) (string, error) {
	res, err := w.writer.UpsertFactsAndEpisodeBatch(ctx, []biz.MemoryFactWrite{{
		ScopeType:  memoryCanaryScopeType,
		ScopeID:    memoryCanaryScopeID,
		Statement:  "金丝雀记忆测试：我的代号是 " + token,
		FactKind:   "general",
		Confidence: 0.99,
		Importance: 0.9,
		SourceKind: "memory_canary",
		Status:     "active",
	}}, nil)
	if err != nil {
		return "", err
	}
	if res == nil || len(res.FactRows) == 0 {
		return "", fmt.Errorf("upsert returned no fact row")
	}
	var row map[string]any
	if err := json.Unmarshal(res.FactRows[0], &row); err != nil {
		return "", fmt.Errorf("decode fact row: %w", err)
	}
	id, _ := row["id"].(string)
	if id == "" {
		return "", fmt.Errorf("fact row has no id")
	}
	return id, nil
}

// assertRecall proves the written fact survives the production recall filter:
// brute-force/vector scoring must produce scores.total >= minScore or the
// fused recall drops it before prompt injection (Bug A chain).
func (w *MemoryCanaryWorker) assertRecall(ctx context.Context, token string) error {
	rows, err := w.reader.RecallL3Facts(ctx, memoryCanaryScopeType, memoryCanaryScopeID, "", token, nil, 5, MemoryCanaryMinScore)
	if err != nil {
		return err
	}
	for _, raw := range rows {
		var row map[string]any
		if json.Unmarshal(raw, &row) != nil {
			continue
		}
		stmt, _ := row["statement"].(string)
		if !strings.Contains(stmt, token) {
			continue
		}
		total := scoresTotal(row)
		if total < MemoryCanaryMinScore {
			return fmt.Errorf("canary fact recalled but scores.total=%.3f < %.2f (score pipeline broken)", total, MemoryCanaryMinScore)
		}
		return nil
	}
	return fmt.Errorf("canary fact not recalled at minScore=%.2f (%d rows)", MemoryCanaryMinScore, len(rows))
}

// assertArchived invalidates the fact and proves the bi-temporal filter
// (valid_until = '') removes it from recall — doubling as residue cleanup.
func (w *MemoryCanaryWorker) assertArchived(ctx context.Context, factID, token string) error {
	if _, err := w.facts.InvalidateFact(ctx, factID); err != nil {
		return fmt.Errorf("invalidate: %w", err)
	}
	rows, err := w.reader.RecallL3Facts(ctx, memoryCanaryScopeType, memoryCanaryScopeID, "", token, nil, 5, 0)
	if err != nil {
		return fmt.Errorf("post-archive recall: %w", err)
	}
	for _, raw := range rows {
		var row map[string]any
		if json.Unmarshal(raw, &row) != nil {
			continue
		}
		if id, _ := row["id"].(string); id == factID {
			return fmt.Errorf("canary fact still recallable after invalidation (valid_until filter broken)")
		}
	}
	return nil
}

func (w *MemoryCanaryWorker) cleanupFact(ctx context.Context, factID string) {
	if factID == "" {
		return
	}
	if _, err := w.facts.InvalidateFact(ctx, factID); err != nil {
		w.lg.Warn("memory canary cleanup invalidate failed",
			loggateway.StepID("memory.canary"),
			loggateway.Str("fact_id", factID),
			loggateway.Err(err))
	}
}

func (w *MemoryCanaryWorker) fail(stage string, err error, dur time.Duration) {
	reason := err.Error()
	w.status.RecordFail(stage, reason)
	w.lg.Error("memory canary failed",
		loggateway.StepID("memory.canary"),
		loggateway.Str("stage", stage),
		loggateway.Err(err),
		loggateway.Int64("duration_ms", dur.Milliseconds()))
	if w.flowLog != nil {
		w.flowLog.LogFlowError(context.Background(), "", "system.memory_canary.failed",
			"记忆闭环金丝雀告警："+stage+" 阶段失败",
			biz.LogPair{Key: "stage", Value: stage},
			biz.LogPair{Key: "reason", Value: reason})
	}
}

func scoresTotal(row map[string]any) float64 {
	sc, ok := row["scores"].(map[string]any)
	if !ok {
		return 0
	}
	switch v := sc["total"].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	}
	return 0
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failure is practically unreachable; fall back to time
		// so the probe still gets a unique token.
		return hex.EncodeToString([]byte(fmt.Sprintf("%x", time.Now().UnixNano())))[: n*2]
	}
	return hex.EncodeToString(buf)
}

// MemoryCanaryDisabled reports whether the canary is disabled via env.
func MemoryCanaryDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_CANARY_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
