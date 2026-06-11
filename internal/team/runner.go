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
	"aranea-agents/pkg/loggateway"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// StreamOptsFactory creates StreamConsumeOptions for a team turn.
// Implemented by internal/chatactivity; injected via RunnerConfig.StreamOptsFactory.
type StreamOptsFactory interface {
	NewStreamConsumeOptions() *agent.StreamConsumeOptions
}

type Runner struct {
	teamReader      biz.TeamReader
	runReader       biz.TeamRunReader
	runWriter       biz.TeamRunWriter
	stepRepo        biz.OrchestrationStepRepo
	deadLetter      biz.TaskDeadLetterRepo
	usage           biz.TeamUsageQuerier
	td              rt.TurnDeps
	skillDBRepo     trpcskill.Repository
	codeExecFactory *localexec.Factory
	cfg             RunnerConfig
	mediator        *TeamRunMediator
	lg              loggateway.Logger
}

// SetMediator wires the TeamRunMediator that breaks the circular dependency
// between Runner and TeamGraphRunCoordinator. Runner depends on
// TeamGraphCoordAccess (via Mediator); Coordinator depends on
// TeamGraphRunFinisher (via Mediator). Construction order:
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

func NewRunner(
	teamReader biz.TeamReader,
	runReader biz.TeamRunReader,
	runWriter biz.TeamRunWriter,
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
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.InternalServer("CHAT_TEAM_NATIVE", "team runner not configured")
	}
	if !strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team") {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_TEAM_NATIVE", "session is not a team session")
	}
	tid := strings.TrimSpace(sess.TeamID)
	if tid == "" {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_TEAM_NATIVE", "session has no team_id")
	}
	if rtid := strings.TrimSpace(input.TeamID); rtid != "" && !strings.EqualFold(rtid, tid) {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.Forbidden("CHAT_TEAM_NATIVE", "team_id does not match session")
	}

	teamRow, err := r.teamReader.GetTeamByID(ctx, tid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.NotFound("TEAM", "team not found")
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	def, err := ParseDefinition(teamRow.DefinitionJSON)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("TEAM", "invalid team definition_json")
	}

	mode := strings.ToLower(strings.TrimSpace(def.Mode))
	return r.runTeamTRPCFromInput(ctx, sess, input, teamRow, def, mode)
}
