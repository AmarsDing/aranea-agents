package evaluation

import (
	"context"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const triggerAfterTurn = "after_turn"

// AfterTurnTrigger schedules dataset evaluation after successful chat turns (US-5).
type AfterTurnTrigger struct {
	uc        *biz.EvalUsecase
	runner    *Runner
	lg        loggateway.Logger
	mu        sync.Mutex
	last      map[string]time.Time
	lastClean time.Time
	// randFn supplies the sampling coin flip (P2-2 sample_rate); injectable
	// for deterministic tests. Defaults to rand.Float64.
	randFn func() float64
}

// NewAfterTurnTrigger constructs an AfterTurnTrigger.
func NewAfterTurnTrigger(uc *biz.EvalUsecase, runner *Runner, lg loggateway.Logger) *AfterTurnTrigger {
	return &AfterTurnTrigger{uc: uc, runner: runner, lg: lg, last: make(map[string]time.Time)}
}

var _ biz.NativeTurnAfterHook = (*AfterTurnTrigger)(nil)

// AfterNativeTurn enqueues an async eval run when agent config enables auto_after_turn.
func (t *AfterTurnTrigger) AfterNativeTurn(ctx context.Context, ev biz.NativeTurnEvent) {
	if t == nil || t.uc == nil || t.runner == nil {
		return
	}
	var cfg biz.AgentEvalAutoConfig
	if ev.AgentSettings != nil {
		cfg = ev.AgentSettings.EvalAutoConfig()
	}
	if !cfg.Enabled {
		cfg = biz.ParseAgentEvalAutoConfig(ev.AgentConfigJSON)
	}
	if !cfg.Enabled || cfg.DatasetID == "" {
		return
	}
	agentID := strings.TrimSpace(ev.AgentID)
	if agentID == "" {
		return
	}
	// P2-2: sample before rate-limit accounting so sample_rate approximates
	// the fraction of eligible turns that actually trigger an online run.
	if !t.samplePass(cfg.SampleRate) {
		return
	}
	if !t.allowTrigger(agentID, cfg.MinIntervalSec) {
		return
	}
	datasetID := cfg.DatasetID
	metrics := cfg.Metrics
	numRuns := cfg.NumRuns
	safego.Go(ctx, "eval-after-turn", func() {
		// EVAL-01: keep request values (workspace) while detaching from
		// request cancellation. context.Background() would strand the run in
		// the legacy workspace bucket ("") — invisible to the owning tenant
		// (strict workspace filter) yet readable by the default workspace
		// (cross-tenant leak). It also feeds the run execution and the
		// score-drop alerter's trend query, which must stay tenant-scoped.
		bgCtx := context.WithoutCancel(ctx)
		in := biz.EvalRun{
			DatasetID:     datasetID,
			AgentID:       agentID,
			Status:        "pending",
			TriggerSource: triggerAfterTurn,
			NumRuns:       numRuns,
		}
		if !workspace.IsSystem(bgCtx) {
			in.WorkspaceID = workspace.IDFromContext(bgCtx)
		}
		run, err := t.uc.CreateRun(bgCtx, in)
		if err != nil {
			// K2: a silently dropped trigger looks like "auto-eval never
			// fired" — log the real reason.
			t.lg.Warn("eval after-turn: create run failed",
				loggateway.StepID("evaluation.after_turn.create_fail"),
				loggateway.Str("agent_id", agentID),
				loggateway.Str("dataset_id", datasetID),
				loggateway.Err(err))
			return
		}
		t.runner.Start(bgCtx, run, metrics, numRuns, false)
	})
}

// samplePass reports whether this turn wins the sampling lottery. Rates
// outside (0,1) always pass (backward-compatible default = evaluate every
// eligible turn).
func (t *AfterTurnTrigger) samplePass(rate float64) bool {
	if rate <= 0 || rate >= 1 {
		return true
	}
	fn := t.randFn
	if fn == nil {
		fn = rand.Float64
	}
	return fn() < rate
}

func (t *AfterTurnTrigger) allowTrigger(agentID string, minIntervalSec int) bool {
	if minIntervalSec <= 0 {
		return true
	}
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	if now.Sub(t.lastClean) > time.Hour {
		cutoff := now.Add(-2 * time.Duration(minIntervalSec) * time.Second)
		for k, v := range t.last {
			if v.Before(cutoff) {
				delete(t.last, k)
			}
		}
		t.lastClean = now
	}
	if last, ok := t.last[agentID]; ok && now.Sub(last) < time.Duration(minIntervalSec)*time.Second {
		return false
	}
	t.last[agentID] = now
	return true
}
