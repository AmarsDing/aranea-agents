package team

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/adkdeps"
	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// Runner executes native team workflows via pkg/trpc-agent-go workflow agents + runner.Run.
type Runner struct {
	teams        biz.TeamRepository
	sessions     *biz.SessionUsecase
	agents       biz.AgentRepository
	agentsUC     *biz.AgentUsecase
	toolsCatalog biz.ToolRepo
	catalog      *biz.LlmProviderModelUsecase
	broker       *biz.TeamRunEventBroker
	llmHTTP      *http.Client
	skillUC      *biz.SkillUsecase
	sys          biz.SystemSettingRepo
	adk          *adkdeps.Runtime
	compress     biz.NativeTurnCompressor
	monitorLogs  *biz.MonitorLogBroker
}

// NewRunner wires a team runner. llmHTTP uses Timeout 0 so per-turn limits come from
// context.WithTimeout in runTeamADK (definition.timeout_seconds), not a fixed client cap.
func NewRunner(
	teams biz.TeamRepository,
	sessions *biz.SessionUsecase,
	agents biz.AgentRepository,
	agentsUC *biz.AgentUsecase,
	toolsCatalog biz.ToolRepo,
	catalog *biz.LlmProviderModelUsecase,
	broker *biz.TeamRunEventBroker,
	skillUC *biz.SkillUsecase,
	sys biz.SystemSettingRepo,
	adk *adkdeps.Runtime,
	compress biz.NativeTurnCompressor,
	monitorLogs *biz.MonitorLogBroker,
) *Runner {
	return &Runner{
		teams:        teams,
		sessions:     sessions,
		agents:       agents,
		agentsUC:     agentsUC,
		toolsCatalog: toolsCatalog,
		catalog:      catalog,
		broker:       broker,
		llmHTTP:      &http.Client{Timeout: 0},
		skillUC:      skillUC,
		sys:          sys,
		adk:          adk,
		compress:     compress,
		monitorLogs:  monitorLogs,
	}
}

func (r *Runner) catalogAgent(ctx context.Context, id string) (biz.Agent, error) {
	if r.agentsUC != nil {
		return r.agentsUC.Get(ctx, id)
	}
	if r.agents != nil && r.toolsCatalog != nil {
		return biz.NewAgentUsecase(r.agents, r.toolsCatalog).Get(ctx, id)
	}
	return r.agents.GetAgentByID(ctx, id)
}

// RunTurn executes one user turn for a team session.
func (r *Runner) RunTurn(ctx context.Context, sess biz.Session, req *chatv1.SendChatMessageRequest, stream agent.StreamEmitter) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	if r == nil || r.sessions == nil || r.teams == nil || r.agents == nil || r.catalog == nil {
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
	return r.runTeamADK(ctx, sess, req, teamRow, def, mode, stream)
}
