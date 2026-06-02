package team

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"aranea-agents/internal/agent"
	localexec "aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
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
	teams           biz.TeamRepository
	usage           *biz.UsageUsecase
	td              rt.TurnDeps
	skillDBRepo     trpcskill.Repository
	codeExecFactory *localexec.Factory
	cfg             RunnerConfig
	teamGraphCoord  *TeamGraphRunCoordinator
	lg              loggateway.Logger
}

// SetTeamGraphRunCoordinator wires team graph execution lifecycle (register / HITL / task resume).
// This is the only remaining Setter because Runner and TeamGraphRunCoordinator have a circular
// dependency: Runner needs Coordinator, and Coordinator needs Runner via TeamGraphRunFinisher.
// The circular dependency is resolved at construction time: Runner is created first (without
// Coordinator), then Coordinator is created, then this Setter links them.
func (r *Runner) SetTeamGraphRunCoordinator(c *TeamGraphRunCoordinator) {
	if r == nil {
		return
	}
	r.teamGraphCoord = c
}

func NewRunner(
	teams biz.TeamRepository,
	usage *biz.UsageUsecase,
	sessions *biz.SessionUsecase,
	agents biz.AgentRepository,
	agentsUC *biz.AgentUsecase,
	toolsCatalog biz.ToolCatalogReader,
	toolUC *biz.ToolUsecase,
	catalog *biz.LlmProviderModelUsecase,
	eventBus event.Bus,
	skillUC *biz.SkillUsecase,
	sys biz.SystemSettingRepo,
	persist rt.PersistenceSet,
	compress biz.NativeTurnCompressor,
	skillDBRepo trpcskill.Repository,
	codeExecFactory *localexec.Factory,
	lg loggateway.Logger,
	cfg RunnerConfig,
) *Runner {
	return &Runner{
		teams:           teams,
		usage:           usage,
		skillDBRepo:     skillDBRepo,
		codeExecFactory: codeExecFactory,
		cfg:             cfg,
		lg:              lg,
		td: rt.TurnDeps{
			Catalog: rt.Catalog{
				Agents:   agents,
				AgentsUC: agentsUC,
				Tools:    toolsCatalog,
				ToolUC:   toolUC,
				LLM:      catalog,
				SkillUC:  skillUC,
				Settings: sys,
			},
			Persist:   persist,
			Pipeline:  rt.EventPipeline{Bus: eventBus, Buffer: event.NewBuffer()},
			LLMHTTP:   &http.Client{Timeout: 0},
			Sessions:  sessions,
			Compress:  compress,
			RunnerMgr: rt.NewRunnerManagerFromPersist(persist, lg),
			Lg:        lg,
		},
	}
}

func (r *Runner) catalogAgent(ctx context.Context, id string) (biz.Agent, error) {
	if r.td.Catalog.AgentsUC != nil {
		return r.td.Catalog.AgentsUC.Get(ctx, id)
	}
	if r.td.Catalog.Agents != nil && r.td.Catalog.Tools != nil {
		return biz.NewAgentUsecase(r.td.Catalog.Agents, r.td.Catalog.Tools, nil, nil).Get(ctx, id)
	}
	return r.td.Catalog.Agents.GetAgentByID(ctx, id)
}

// RunTurnFromInput executes one user turn for a team session using biz-level TurnInput.
func (r *Runner) RunTurnFromInput(ctx context.Context, sess biz.Session, input biz.TurnInput) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	if r == nil || r.td.Sessions == nil || r.teams == nil || r.td.Catalog.Agents == nil || r.td.Catalog.LLM == nil {
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

	teamRow, err := r.teams.GetTeamByID(ctx, tid)
	if err != nil {
		if err == sql.ErrNoRows {
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
