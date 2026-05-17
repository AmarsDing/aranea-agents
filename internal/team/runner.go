package team

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	rt "aranea-agents/internal/runtime"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type Runner struct {
	teams       biz.TeamRepository
	td          rt.TurnDeps
	pluginRT    *plugintrpc.Runtime
	skillDBRepo trpcskill.Repository
}

func NewRunner(
	teams biz.TeamRepository,
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
	skillDBRepo trpcskill.Repository,
) *Runner {
	return &Runner{
		teams:       teams,
		pluginRT:    pluginRT,
		skillDBRepo: skillDBRepo,
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
			Persist:  persist,
			Pipeline: rt.EventPipeline{Bus: eventBus},
			LLMHTTP:  &http.Client{Timeout: 0},
			Sessions: sessions,
			Compress: compress,
		},
	}
}

func (r *Runner) catalogAgent(ctx context.Context, id string) (biz.Agent, error) {
	if r.td.Catalog.AgentsUC != nil {
		return r.td.Catalog.AgentsUC.Get(ctx, id)
	}
	if r.td.Catalog.Agents != nil && r.td.Catalog.Tools != nil {
		return biz.NewAgentUsecase(r.td.Catalog.Agents, r.td.Catalog.Tools).Get(ctx, id)
	}
	return r.td.Catalog.Agents.GetAgentByID(ctx, id)
}

// RunTurn executes one user turn for a team session.
func (r *Runner) RunTurn(ctx context.Context, sess biz.Session, req *chatv1.SendChatMessageRequest) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
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
	if rtid := strings.TrimSpace(req.GetTeamId()); rtid != "" && !strings.EqualFold(rtid, tid) {
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
	return r.runTeamTRPC(ctx, sess, req, teamRow, def, mode)
}
