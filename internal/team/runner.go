package team

import (
	"context"
	"database/sql"
	"errors"
	"strings"

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
	lg              loggateway.Logger
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

func (r *Runner) SetAwaitHookProvider(fn func(runCtx context.Context, sessionID, runID string) tooltrpc.ReplyFunc) {
	if r == nil {
		return
	}
	r.cfg.AwaitHookProvider = fn
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

func NewRunner(
	teamReader biz.TeamReader,
	runReader biz.TeamRunReader,
	runWriter biz.TeamRunWriter,
	runTransitioner biz.TeamRunStatusTransitioner,
	stepRepo biz.OrchestrationStepRepo,
	deadLetter biz.TaskDeadLetterRepo,
	usage biz.TeamUsageQuerier,
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

	teamRow, err := r.teamReader.GetTeamByID(ctx, tid)
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
