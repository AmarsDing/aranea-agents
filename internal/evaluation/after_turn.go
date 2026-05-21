package evaluation

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/safego"
)

const triggerAfterTurn = "after_turn"

// AfterTurnTrigger schedules dataset evaluation after successful chat turns (US-5).
type AfterTurnTrigger struct {
	uc     *biz.EvalUsecase
	runner *Runner
	mu     sync.Mutex
	last   map[string]time.Time // agentID -> last trigger time
}

// NewAfterTurnTrigger constructs an AfterTurnTrigger.
func NewAfterTurnTrigger(uc *biz.EvalUsecase, runner *Runner) *AfterTurnTrigger {
	return &AfterTurnTrigger{uc: uc, runner: runner, last: make(map[string]time.Time)}
}

var _ biz.NativeTurnAfterHook = (*AfterTurnTrigger)(nil)

// AfterNativeTurn enqueues an async eval run when agent config enables auto_after_turn.
func (t *AfterTurnTrigger) AfterNativeTurn(ctx context.Context, ev biz.NativeTurnEvent) {
	if t == nil || t.uc == nil || t.runner == nil {
		return
	}
	cfg := biz.ParseAgentEvalAutoConfig(ev.AgentConfigJSON)
	if !cfg.Enabled || cfg.DatasetID == "" {
		return
	}
	agentID := strings.TrimSpace(ev.AgentID)
	if agentID == "" {
		return
	}
	if !t.allowTrigger(agentID, cfg.MinIntervalSec) {
		return
	}
	datasetID := cfg.DatasetID
	metrics := cfg.Metrics
	numRuns := cfg.NumRuns
	safego.Go(ctx, "eval-after-turn", func() {
		bgCtx := context.Background()
		run, err := t.uc.CreateRun(bgCtx, biz.EvalRun{
			DatasetID:     datasetID,
			AgentID:       agentID,
			Status:        "pending",
			TriggerSource: triggerAfterTurn,
			NumRuns:       numRuns,
		})
		if err != nil {
			return
		}
		t.runner.Start(bgCtx, run, metrics, numRuns, false)
	})
}

func (t *AfterTurnTrigger) allowTrigger(agentID string, minIntervalSec int) bool {
	if minIntervalSec <= 0 {
		return true
	}
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	if last, ok := t.last[agentID]; ok && now.Sub(last) < time.Duration(minIntervalSec)*time.Second {
		return false
	}
	t.last[agentID] = now
	return true
}
