package team

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"

	"aranea-agents/internal/agent"
	localexec "aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	tooltrpc "aranea-agents/internal/tools/trpc"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// StreamOptsFactory creates StreamConsumeOptions for a team turn.
// Implemented by internal/chatactivity; injected via RunnerConfig.StreamOptsFactory.
// Stability:internal
type StreamOptsFactory interface {
	NewStreamConsumeOptions() *agent.StreamConsumeOptions
}

type Runner struct {
	teamReader      biz.TeamReader
	runReader       biz.TeamRunReader
	runWriter       biz.TeamRunWriter
	runTransitioner biz.TeamRunStatusTransitioner
	stepRepo        biz.OrchestrationStepRepo
	deadLetter      biz.TaskDeadLetterRepo
	usage           biz.TeamUsageQuerier
	td              rt.TurnDeps
	skillDBRepo     trpcskill.Repository
	codeExecFactory *localexec.Factory
	cfg             RunnerConfig
	mediator        *TeamRunMediator
	// deliverableGate vetoes run-success finalization for DAG teams that
	// produced no real deliverable (set_deliverable never called). Optional:
	// nil keeps legacy behavior (always success). Wired in production to
	// biz.SpiritTeamController.HasRealDeliverable.
	deliverableGate func(ctx context.Context, team biz.Team) (bool, error)
	// qualityGate evaluates deliverable CONTENT quality after the binary gate
	// passes (G3/ADR-G: verdict pass/revise/fail with bounded revision loop).
	// Optional: nil keeps binary-only behavior.
	qualityGate func(ctx context.Context, team biz.Team) (biz.QualityGateResult, error)
	// revisionEnqueuer delivers the judge feedback as a followup to the team
	// session (P2-3 roadbed). Optional: nil degrades revise to fail-open pass.
	revisionEnqueuer func(ctx context.Context, sessionID, content string) error
	// qualityReviseCount bounds quality-gate revisions per team+session
	// (maxQualityRevisions). In-memory: process restart resets the budget,
	// worst case one extra revision chain — acceptable.
	qualityReviseMu    sync.Mutex
	qualityReviseCount map[string]int
	// upstreamSeedFn resolves the cross-team deliverable seed injected into
	// the graph initial state at DAG downstream team turn start (2026-08-08
	// 问题3c). Optional: nil skips seeding. Wired in production to
	// biz.SpiritTeamUsecase.UpstreamDeliverableSeed.
	upstreamSeedFn func(ctx context.Context, team biz.Team) (map[string]any, error)
	// heartbeatWriter 是 P2-1 持久化心跳写入端口（run_heartbeat.go）。Optional:
	// nil 时流式事件不产生心跳，biz idle 探测回退 steps.started_at 旧语义。
	heartbeatWriter biz.TeamRunHeartbeatWriter
	// monitor receives runner.completion events on run terminal states
	// (Runner metrics + runner.error_rate alert data source). Optional:
	// nil skips monitor recording (tests).
	monitor *biz.MonitorUsecase
	// tokenBudget 是 run 级累计 input-token 预算闸（2026-08-24，见
	// token_budget.go）。成员行记账时累加，超闸取消 run ctx，防止
	// 多成员 ReAct 回灌的累计记账无上限（实测单 run 513 万 input tok）。
	budgetMu      sync.Mutex
	budgetUsed    map[string]int64
	budgetLimit   map[string]int64
	budgetTripped map[string]bool
	// noProgress 是 run 级无进展审计（79-runtime-governance R5，见
	// no_progress_auditor.go）：成员 step 记账点追踪连续同指纹状态，
	// 纠偏注记 + 单发 Cancel（reason=no_progress）。
	noProgMu           sync.Mutex
	noProgRuns         map[string]map[string]*noProgressMemberState
	noProgTripped      map[string]bool
	noProgressEnqueuer func(ctx context.Context, sessionID, content string) error
	lg                 loggateway.Logger
}

// SetMediator wires the TeamRunMediator that breaks the circular dependency
// between Runner and TeamGraphRunCoordinator. Runner depends on
// TeamGraphCoordAccess (via Mediator); Coordinator depends on
// Mediator's finisher methods. Construction order:
// Runner → Mediator → Coordinator → Mediator.SetCoordinator.
func (r *Runner) SetMediator(m *TeamRunMediator) {
	if r == nil {
		return
	}
	r.mediator = m
}

// SetNoProgressEnqueuer wires the correction-note injection channel for the
// no-progress auditor (79-runtime-governance R5): the note is enqueued as a
// followup to the team session so members read it in the next turn's history.
// Optional: nil skips injection (counting/cancel still work).
func (r *Runner) SetNoProgressEnqueuer(fn func(ctx context.Context, sessionID, content string) error) {
	if r == nil {
		return
	}
	r.noProgressEnqueuer = fn
}

func (r *Runner) SetAwaitHookProvider(fn func(runCtx context.Context, sessionID, runID string) tooltrpc.ReplyFunc) {
	if r == nil {
		return
	}
	r.cfg.AwaitHookProvider = fn
}

// SetTeamRunHeartbeatWriter wires the P2-1 persistent heartbeat writer.
// runTeamTRPCFromInput throttles stream events into team_runs_v2.heartbeat_at
// so the biz idle probe sees liveness during long single-step generations.
func (r *Runner) SetTeamRunHeartbeatWriter(w biz.TeamRunHeartbeatWriter) {
	if r == nil {
		return
	}
	r.heartbeatWriter = w
}

// SetDeliverableGate wires the real-deliverable gate consulted by
// finalizeTeamRun before marking a DAG team run success. The run FSM treats
// success as terminal, so the veto must happen BEFORE the success transition
// — a post-hoc service-layer flip cannot repair the run record. Mirrors the
// service gate in HandleTeamTurnResult as a second line of defense.
func (r *Runner) SetDeliverableGate(fn func(ctx context.Context, team biz.Team) (bool, error)) {
	if r == nil {
		return
	}
	r.deliverableGate = fn
}

// SetUpstreamDeliverableSeed wires the cross-team deliverable seed resolver.
// The seed is injected into the graph initial state's "deliverable" field at
// DAG downstream team turn start, so members read upstream topics via
// get_deliverable directly. Seed 回流不冒充本团队产出——biz 层
// HasRealDeliverable / WriteDeliverablesToSession 用同一解析结果减去种子。
func (r *Runner) SetUpstreamDeliverableSeed(fn func(ctx context.Context, team biz.Team) (map[string]any, error)) {
	if r == nil {
		return
	}
	r.upstreamSeedFn = fn
}

func NewRunner(
	teamReader biz.TeamReader,
	runReader biz.TeamRunReader,
	runWriter biz.TeamRunWriter,
	runTransitioner biz.TeamRunStatusTransitioner,
	stepRepo biz.OrchestrationStepRepo,
	deadLetter biz.TaskDeadLetterRepo,
	usage biz.TeamUsageQuerier,
	monitor *biz.MonitorUsecase,
	td rt.TurnDeps,
	skillDBRepo trpcskill.Repository,
	codeExecFactory *localexec.Factory,
	lg loggateway.Logger,
	cfg RunnerConfig,
) *Runner {
	return &Runner{
		teamReader:      teamReader,
		runReader:       runReader,
		runWriter:       runWriter,
		runTransitioner: runTransitioner,
		stepRepo:        stepRepo,
		deadLetter:      deadLetter,
		usage:           usage,
		monitor:         monitor,
		skillDBRepo:     skillDBRepo,
		codeExecFactory: codeExecFactory,
		cfg:             cfg,
		lg:              lg,
		td:              td,
	}
}

// publishEvent routes a v2 Event through the Sequencer when
// available (FIFO ordering + retry/dead-letter); falls back to EventBus.Publish
// when Sequencer is nil (test/legacy paths). Returns false when neither is set
// so callers can short-circuit downstream work.
func (r *Runner) publishEvent(ctx context.Context, e biz.Event) bool {
	if r == nil {
		return false
	}
	if r.td.Pipeline.Sequencer != nil {
		r.td.Pipeline.Sequencer.Publish(ctx, e)
		return true
	}
	if r.td.Pipeline.EventBus != nil {
		r.td.Pipeline.EventBus.Publish(ctx, e)
		return true
	}
	return false
}

// hasPublisher returns true when either Sequencer or EventBus is available.
func (r *Runner) hasPublisher() bool {
	return r != nil && (r.td.Pipeline.Sequencer != nil || r.td.Pipeline.EventBus != nil)
}

func (r *Runner) lookupAgent(ctx context.Context, id string) (biz.Agent, error) {
	if r.td.ReadDeps.AgentsUC != nil {
		return r.td.ReadDeps.AgentsUC.Get(ctx, id)
	}
	if r.td.ReadDeps.Agents != nil && r.td.ReadDeps.Tools != nil {
		return biz.NewAgentUsecase(biz.AgentUsecaseDeps{Reader: r.td.ReadDeps.Agents, Writer: r.td.ReadDeps.Agents, Settings: r.td.ReadDeps.Agents, Files: r.td.ReadDeps.Agents, Position: r.td.ReadDeps.Agents, Tx: r.td.ReadDeps.Agents, Tools: r.td.ReadDeps.Tools}).Get(ctx, id)
	}
	return r.td.ReadDeps.Agents.GetAgentByID(ctx, id)
}

// loadTeamForRun loads the team for a run, routing through the lazy graph
// materialization ensurer（B10）when configured so legacy teams whose
// linked_graph_id is still empty get materialized on first run. Falls back
// to the plain reader when the port is not wired (tests/offline tools).
func (r *Runner) loadTeamForRun(ctx context.Context, tid string) (biz.Team, error) {
	if r.cfg.GraphEnsurer != nil {
		return r.cfg.GraphEnsurer.EnsureTeamGraphAsset(ctx, tid)
	}
	return r.teamReader.GetTeamByID(ctx, tid)
}

// RunTurnFromInput executes one user turn for a team session using biz-level TurnInput.
func (r *Runner) RunTurnFromInput(ctx context.Context, sess biz.Session, input biz.TurnInput) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	if r == nil || r.td.Sessions == nil || r.teamReader == nil || r.runWriter == nil || r.td.ReadDeps.Agents == nil || r.td.ReadDeps.LLM == nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, apierror.Internal("CHAT_TEAM_NATIVE", "team runner not configured")
	}
	if !strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team") {
		return biz.ChatMessage{}, biz.ChatMessage{}, apierror.BadRequest("CHAT_TEAM_NATIVE", "session is not a team session")
	}
	tid := strings.TrimSpace(sess.TeamID)
	if tid == "" {
		return biz.ChatMessage{}, biz.ChatMessage{}, apierror.BadRequest("CHAT_TEAM_NATIVE", "session has no team_id")
	}
	if rtid := strings.TrimSpace(input.TeamID); rtid != "" && !strings.EqualFold(rtid, tid) {
		return biz.ChatMessage{}, biz.ChatMessage{}, apierror.Forbidden("CHAT_TEAM_NATIVE", "team_id does not match session")
	}

	teamRow, err := r.loadTeamForRun(ctx, tid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return biz.ChatMessage{}, biz.ChatMessage{}, apierror.NotFound(apierror.DomainTeam, "team not found")
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	def, err := ParseDefinition(teamRow.DefinitionJSON)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, apierror.BadRequest(apierror.DomainTeam, "invalid team definition_json")
	}

	mode := strings.ToLower(strings.TrimSpace(def.Mode))
	return r.runTeamTRPCFromInput(ctx, sess, input, teamRow, def, mode)
}
