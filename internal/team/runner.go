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
	graphadapter "aranea-agents/internal/graph/adapter"
	"aranea-agents/internal/knowledge"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	rt "aranea-agents/internal/runtime"
	tooltrpc "aranea-agents/internal/tools/trpc"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// StreamOptsFactory creates StreamConsumeOptions for a team turn.
// Implemented by internal/chatactivity; injected via SetStreamOptsFactory.
type StreamOptsFactory interface {
	NewStreamConsumeOptions() *agent.StreamConsumeOptions
}

type Runner struct {
	teams              biz.TeamRepository
	usage              *biz.UsageUsecase
	td                 rt.TurnDeps
	pluginRT           *plugintrpc.Runtime
	pluginManager      *plugintrpc.Manager
	skillDBRepo        trpcskill.Repository
	runs               *rt.RunRegistry
	awaitHookProvider  func(runCtx context.Context, sessionID, runID string) tooltrpc.ReplyFunc
	knowledgeRetriever *knowledge.Retriever
	streamOptsFactory  StreamOptsFactory
	codeExecFactory    *localexec.Factory
	graphRoot          graphadapter.TeamGraphRootBuilder
	graphLoader        GraphBuildConfigLoader
	teamGraphTasks     TeamGraphTaskCreator
	teamGraphCoord     *TeamGraphRunCoordinator
}

// SetGraphBuildConfigLoader wires linked_graph_id resolution for GraphAgent runtime (M53 P2).
func (r *Runner) SetGraphBuildConfigLoader(l GraphBuildConfigLoader) {
	if r == nil {
		return
	}
	r.graphLoader = l
}

// SetTeamGraphTaskCreator wires Kanban task creation for team Graph task/review nodes (M53 TG-RT-TASK).
func (r *Runner) SetTeamGraphTaskCreator(c TeamGraphTaskCreator) {
	if r == nil {
		return
	}
	r.teamGraphTasks = c
}

// SetTeamGraphRunCoordinator wires team graph execution lifecycle (register / HITL / task resume).
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
	toolsCatalog biz.ToolRepo,
	toolUC *biz.ToolUsecase,
	catalog *biz.LlmProviderModelUsecase,
	eventBus event.Bus,
	skillUC *biz.SkillUsecase,
	sys biz.SystemSettingRepo,
	persist rt.PersistenceSet,
	compress biz.NativeTurnCompressor,
	pluginRT *plugintrpc.Runtime,
	pluginManager *plugintrpc.Manager,
	skillDBRepo trpcskill.Repository,
	codeExecFactory *localexec.Factory,
) *Runner {
	return &Runner{
		teams:           teams,
		usage:           usage,
		pluginRT:        pluginRT,
		pluginManager:   pluginManager,
		skillDBRepo:     skillDBRepo,
		codeExecFactory: codeExecFactory,
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
			RunnerMgr: rt.NewRunnerManagerFromPersist(persist),
		},
	}
}

func (r *Runner) SetAwaitHookProvider(fn func(runCtx context.Context, sessionID, runID string) tooltrpc.ReplyFunc) {
	r.awaitHookProvider = fn
}

func (r *Runner) SetKnowledgeRetriever(ret *knowledge.Retriever) {
	r.knowledgeRetriever = ret
}

// SetStreamOptsFactory wires the StreamConsumeOptions factory, eliminating
// direct chatactivity import from the team package.
func (r *Runner) SetStreamOptsFactory(f StreamOptsFactory) {
	r.streamOptsFactory = f
}

// SetRunRegistry shares the chat gateway run registry for cancel/status/enqueue.
func (r *Runner) SetRunRegistry(reg *rt.RunRegistry) {
	r.runs = reg
}

// SetGraphRootBuilder wires GraphAgent team runtime (Phase 3). Nil disables graph path.
func (r *Runner) SetGraphRootBuilder(b graphadapter.TeamGraphRootBuilder) {
	if r == nil {
		return
	}
	r.graphRoot = b
}

func (r *Runner) catalogAgent(ctx context.Context, id string) (biz.Agent, error) {
	if r.td.Catalog.AgentsUC != nil {
		return r.td.Catalog.AgentsUC.Get(ctx, id)
	}
	if r.td.Catalog.Agents != nil && r.td.Catalog.Tools != nil {
		return biz.NewAgentUsecase(r.td.Catalog.Agents, r.td.Catalog.Tools, nil).Get(ctx, id)
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
