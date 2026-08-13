package jobs

import (
	"context"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// EvolutionAgentLister is the narrow agent dependency of
// EvolutionOrchestratorWorker: paged active-agent listing plus the runtime
// settings flag that gates skill-evolution opt-in. Defined at the consumer
// (jobs package convention) so tests mock 2 methods instead of the full
// biz.AgentRepository composite.
type EvolutionAgentLister interface {
	SearchAgents(ctx context.Context, q biz.AgentListQuery) (biz.AgentListResult, error)
	GetAgentRuntimeSettings(ctx context.Context, agentID string) (biz.AgentRuntimeSettings, error)
}

// EvolutionDrafterPort is the narrow post-pass dependency (EVO-20): after the
// trigger pass, notification-only L3 suggestions (persona/prompt without an
// apply payload) get an LLM-generated actionable draft. nil = feature off.
// *biz.EvolutionDrafter satisfies this interface.
type EvolutionDrafterPort interface {
	DraftPending(ctx context.Context, agentID string) error
}

// EvolutionOrchestratorWorker is the single unified entry point for automatic
// evolution triggering (A1). It replaces the trigger half of the legacy
// per-pipeline scanners:
//
//   - SkillEvolutionScanner (pattern → new-skill proposals) — superseded by PatternTrigger
//   - CuratorWorker trigger step (health → improvement suggestions) — superseded by HealthTrigger
//
// Each tick it:
//  1. Expires stale pending suggestions (orchestrator.ExpirePending)
//  2. Scans active skills → orchestrator.CheckAndCreate(skill) — HealthTrigger
//  3. Scans active agents (L1 EvolutionSkillEvolve or L3 EvolutionSuggestionsEnabled/
//     EvoEnabled opt-in) → orchestrator.CheckAndCreate(agent) — PatternTrigger +
//     AgentConfigTrigger; each trigger re-checks its own opt-in flag.
//  4. Post-pass (EVO-20): for each L3-opted-in agent, drafter.DraftPending
//     turns one pending notification-only persona/prompt suggestion into an
//     applicable draft (LLM failure degrades silently to notification state).
//
// Verification of triggered suggestions remains with CuratorWorker
// (ValidatePendingSuggestionsForSkill); the learning loop
// (LearningLoopScanner) and report generation (SkillIntelligenceWorker) are
// orthogonal and keep their own workers.
type EvolutionOrchestratorWorker struct {
	interval time.Duration
	orch     *biz.SkillEvolutionOrchestrator
	agents   EvolutionAgentLister
	skills   biz.SkillQueryReader
	drafter  EvolutionDrafterPort // nil = EVO-20 disabled
	lg       loggateway.Logger
	running  atomic.Bool // single-flight: skip a tick while the previous scan is still in flight
}

// NewEvolutionOrchestratorWorker creates the worker. If interval <= 0, defaults to 2 hours.
func NewEvolutionOrchestratorWorker(
	interval time.Duration,
	orch *biz.SkillEvolutionOrchestrator,
	agents EvolutionAgentLister,
	skills biz.SkillQueryReader,
	drafter EvolutionDrafterPort,
	lg loggateway.Logger,
) *EvolutionOrchestratorWorker {
	if interval <= 0 {
		interval = 2 * time.Hour
	}
	return &EvolutionOrchestratorWorker{
		interval: interval,
		orch:     orch,
		agents:   agents,
		skills:   skills,
		drafter:  drafter,
		lg:       lg,
	}
}

// Start begins the worker loop. Blocks until ctx is cancelled.
func (w *EvolutionOrchestratorWorker) Start(ctx context.Context) {
	if w == nil || w.orch == nil {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *EvolutionOrchestratorWorker) runOnce(ctx context.Context) {
	safego.Go(ctx, "evolution.orchestrator", func() {
		if !w.running.CompareAndSwap(false, true) {
			w.lg.Warn("orchestrator worker: previous scan still running, skipping tick",
				loggateway.StepID("evo_orchestrator_worker.skip"))
			return
		}
		defer w.running.Store(false)
		// 1. Expire stale pending suggestions first so cooldown windows open up.
		if _, err := w.orch.ExpirePending(ctx); err != nil {
			w.lg.Warn("orchestrator worker: expire pending failed",
				loggateway.StepID("evo_orchestrator_worker.expire"),
				loggateway.Err(err))
		}
		// 2. Skill-side triggers (health).
		if err := w.scanSkills(ctx); err != nil {
			w.lg.Warn("orchestrator worker: skill scan failed",
				loggateway.StepID("evo_orchestrator_worker.scan_skills"),
				loggateway.Err(err))
		}
		// 3. Agent-side triggers (pattern / agent_config).
		if err := w.scanAgents(ctx); err != nil {
			w.lg.Warn("orchestrator worker: agent scan failed",
				loggateway.StepID("evo_orchestrator_worker.scan_agents"),
				loggateway.Err(err))
		}
	})
}

// scanSkills iterates active skills in batches and runs orchestrator
// CheckAndCreate for each (HealthTrigger). The orchestrator internally
// dedups via pending checks + per-action cooldown, so this is idempotent.
func (w *EvolutionOrchestratorWorker) scanSkills(ctx context.Context) error {
	if w.skills == nil {
		return nil
	}
	const batchSize = 100
	offset := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		results, err := w.skills.SearchSkills(ctx, biz.SkillListQuery{
			Limit:  batchSize,
			Offset: offset,
			Status: "active",
		})
		if err != nil {
			return err
		}
		for _, skill := range results.Items {
			if _, err := w.orch.CheckAndCreate(ctx, biz.EvolutionTargetSkill, skill.ID); err != nil {
				w.lg.Warn("orchestrator worker: skill check failed",
					loggateway.StepID("evo_orchestrator_worker.scan_skills"),
					loggateway.Str("skill_id", skill.ID),
					loggateway.Err(err))
			}
		}
		if len(results.Items) < batchSize {
			return nil
		}
		offset += batchSize
	}
}

// scanAgents iterates active agents in batches and runs orchestrator
// CheckAndCreate for each agent that has opted into any evolution pipeline
// (L1 EvolutionSkillEvolve or L3 EvolutionSuggestionsEnabled/EvoEnabled).
// Each registered trigger re-checks its own opt-in flag, so the union gate
// here only limits unnecessary CheckAndCreate calls.
func (w *EvolutionOrchestratorWorker) scanAgents(ctx context.Context) error {
	if w.agents == nil {
		return nil
	}
	const batchSize = 100
	offset := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		page, err := w.agents.SearchAgents(ctx, biz.AgentListQuery{
			Limit:  batchSize,
			Offset: offset,
			Status: string(biz.AgentStatusActive),
		})
		if err != nil {
			return err
		}
		for _, a := range page.Items {
			settings, serr := w.agents.GetAgentRuntimeSettings(ctx, a.ID)
			if serr != nil {
				continue
			}
			if !settings.EvolutionSkillEvolve && !settings.EvolutionSuggestionsEnabled && !settings.EvoEnabled {
				continue
			}
			if _, err := w.orch.CheckAndCreate(ctx, biz.EvolutionTargetAgent, a.ID); err != nil {
				w.lg.Warn("orchestrator worker: agent check failed",
					loggateway.StepID("evo_orchestrator_worker.scan_agents"),
					loggateway.Str("agent_id", a.ID),
					loggateway.Err(err))
			}
			// EVO-20 post-pass: draft one notification-only L3 suggestion per
			// cycle. Only L3-opted-in agents can hold evolve_agent suggestions.
			if w.drafter != nil && (settings.EvolutionSuggestionsEnabled || settings.EvoEnabled) {
				if err := w.drafter.DraftPending(ctx, a.ID); err != nil {
					w.lg.Warn("orchestrator worker: draft pending failed",
						loggateway.StepID("evo_orchestrator_worker.draft"),
						loggateway.Str("agent_id", a.ID),
						loggateway.Err(err))
				}
			}
		}
		if len(page.Items) < batchSize {
			return nil
		}
		offset += batchSize
	}
}
