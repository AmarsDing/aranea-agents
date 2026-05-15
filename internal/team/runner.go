package team

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/runtimedeps"
	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type Runner struct {
	teams biz.TeamRepository
	td    runtimedeps.TurnDeps
}

func NewRunner(
	teams biz.TeamRepository,
	sessions *biz.SessionUsecase,
	agents biz.AgentRepository,
	agentsUC *biz.AgentUsecase,
	toolsCatalog biz.ToolRepo,
	toolUC *biz.ToolUsecase,
	catalog *biz.LlmProviderModelUsecase,
	broker *biz.TeamRunEventBroker,
	skillUC *biz.SkillUsecase,
	sys biz.SystemSettingRepo,
	rt *runtimedeps.Runtime,
	compress biz.NativeTurnCompressor,
	monitorLogs *biz.MonitorLogBroker,
) *Runner {
	return &Runner{
		teams: teams,
		td: runtimedeps.TurnDeps{
			Agents:       agents,
			AgentsUC:     agentsUC,
			ToolsCatalog: toolsCatalog,
			ToolUC:       toolUC,
			LLMCatalog:   catalog,
			SkillUC:      skillUC,
			Sys:          sys,
			RT:           rt,
			LLMHTTP:      &http.Client{Timeout: 0},
			Sessions:     sessions,
			Compress:     compress,
			MonitorLogs:  monitorLogs,
			TeamSSE:      broker,
		},
	}
}

func (r *Runner) catalogAgent(ctx context.Context, id string) (biz.Agent, error) {
	if r.td.AgentsUC != nil {
		return r.td.AgentsUC.Get(ctx, id)
	}
	if r.td.Agents != nil && r.td.ToolsCatalog != nil {
		return biz.NewAgentUsecase(r.td.Agents, r.td.ToolsCatalog).Get(ctx, id)
	}
	return r.td.Agents.GetAgentByID(ctx, id)
}

// RunTurn executes one user turn for a team session.
func (r *Runner) RunTurn(ctx context.Context, sess biz.Session, req *chatv1.SendChatMessageRequest, stream agent.StreamEmitter) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	if r == nil || r.td.Sessions == nil || r.teams == nil || r.td.Agents == nil || r.td.LLMCatalog == nil {
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
	return r.runTeamTRPC(ctx, sess, req, teamRow, def, mode, stream)
}
