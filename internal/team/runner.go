package team

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/agent"
	"aranea-agents/internal/agent/adksvc"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/internal/tools/skillruntime"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/runner"

	"google.golang.org/genai"
)

// Runner executes native team workflows via pkg/adk-go workflow agents + runner.Run.
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
}

// NewRunner wires a team runner (llmHTTP should match chat transport timeouts).
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
) *Runner {
	return &Runner{
		teams:        teams,
		sessions:     sessions,
		agents:       agents,
		agentsUC:     agentsUC,
		toolsCatalog: toolsCatalog,
		catalog:      catalog,
		broker:       broker,
		llmHTTP:      &http.Client{Timeout: 300 * time.Second},
		skillUC:      skillUC,
		sys:          sys,
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

func preview(s string, max int) string {
	return strings.TrimSpace(runesTruncate(strings.TrimSpace(s), max))
}

func runesTruncate(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func topologyJSON(def Definition) string {
	ids := make([]string, 0, len(def.Members))
	for _, m := range EnabledMembers(def) {
		ids = append(ids, m.AgentID)
	}
	b, _ := json.Marshal(map[string]any{"member_order": ids, "mode": def.Mode})
	return string(b)
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

func (r *Runner) runTeamADK(ctx context.Context, sess biz.Session, req *chatv1.SendChatMessageRequest, teamRow biz.Team, def Definition, mode string, stream agent.StreamEmitter) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	content := strings.TrimSpace(req.GetContent())
	if content == "" {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "content is required")
	}
	dialogMode, provOpt, modOpt, attN := extractOpts(req)
	dialogMode = firstNonEmptyStr(dialogMode, sess.DialogMode, "default")

	members := EnabledMembers(def)
	if len(members) == 0 {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("TEAM", "team has no enabled members")
	}

	run := biz.TeamRun{
		ID:           uuid.NewString(),
		TeamID:       teamRow.ID,
		SessionID:    sess.ID,
		Mode:         mode,
		Status:       "running",
		InputPreview: preview(content, 512),
		TopologyJSON: topologyJSON(def),
		StartedAt:    agent.RFC3339Now(),
		CreatedAt:    agent.RFC3339Now(),
		UpdatedAt:    agent.RFC3339Now(),
	}
	run, err = r.teams.CreateTeamRun(ctx, run)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	if r.broker != nil {
		cp := run
		r.broker.Publish(biz.TeamRunEvent{Type: "run_started", TeamID: teamRow.ID, RunID: run.ID, Run: &cp})
	}

	t0 := time.Now()
	firstAg, err := r.catalogAgent(ctx, members[0].AgentID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = kerrors.NotFound("AGENT", "team member agent not found")
		}
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	deps := agent.BuilderDeps{
		Catalog:      r.catalog,
		AgentUC:      r.agentsUC,
		Agents:       r.agents,
		ToolsCatalog: r.toolsCatalog,
		RT:           &provider.RoundTrip{HTTP: r.llmHTTP},
	}
	skillOpts := &skillruntime.SkillToolsetOptions{Runtime: firstAg.Settings, UserQuery: content}
	_ = skillruntime.AppendEnabledPublishedSkillToolsets(ctx, &deps.Toolsets, r.skillUC, r.sys, skillOpts)

	root, plan, err := BuildWorkflowRoot(ctx, mode, def, deps, sess, provOpt, modOpt, r.catalogAgent)
	if err != nil {
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	ss := &adksvc.BizSessionService{
		Repo:    adksvc.UsecaseSessionRepo{UC: r.sessions},
		AppName: adksvc.DefaultAppName,
	}
	ss.ResolveAssistantAuthor = func(c context.Context, agentID string) (string, error) {
		a, e := r.catalogAgent(c, agentID)
		if e != nil {
			return "", e
		}
		return strings.TrimSpace(a.AgentKey), nil
	}

	rn, err := runner.New(runner.Config{
		AppName:           adksvc.DefaultAppName,
		Agent:             root,
		SessionService:    ss,
		MemoryService:     agent.NewADKMemoryService(),
		AutoCreateSession: false,
		PluginConfig: runner.PluginConfig{
			Plugins: agent.DefaultRunnerPlugins(),
		},
	})
	if err != nil {
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	prov0 := firstNonEmptyStr(provOpt, sess.Provider, firstAg.Provider)
	mod0 := firstNonEmptyStr(modOpt, sess.Model, firstAg.Model)
	anchor := &agent.TeamMemberAnchor{
		AgentID: firstAg.ID,
		Name:    firstNonEmptyStr(firstAg.DisplayName, firstAg.AgentKey),
		Role:    members[0].Role,
	}
	userOpts, err := agent.UserOptionsJSON(firstAg, dialogMode, prov0, mod0, sess.ContextUsedRatio, anchor)
	if err != nil {
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	userMsg = biz.ChatMessage{
		ID:               uuid.NewString(),
		SessionID:        sess.ID,
		Role:             "user",
		ContentMarkdown:  content,
		Status:           "ok",
		OptionsJSON:      userOpts,
		CreatedAt:        agent.RFC3339Now(),
		AttachmentsCount: attN,
	}

	streaming := stream != nil
	if streaming {
		if err := r.sessions.AppendChatMessage(ctx, sess.ID, userMsg, false); err != nil {
			r.finishRunErr(ctx, &run, t0, err.Error())
			return biz.ChatMessage{}, biz.ChatMessage{}, err
		}
		_ = stream.Emit("user_message", userMsg)
	}

	msg := genai.NewContentFromText(content, genai.RoleUser)
	uid := agent.UserIDFromCtx(ctx)
	cfg := adkagent.RunConfig{StreamingMode: adkagent.StreamingModeSSE}
	if !streaming {
		cfg = adkagent.RunConfig{StreamingMode: adkagent.StreamingModeNone}
	}

	keyMeta := map[string]struct {
		Member MemberDef
		Agent  biz.Agent
	}{}
	for _, m := range plan.persistMembers {
		ag, e := r.catalogAgent(ctx, m.AgentID)
		if e != nil {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(ag.AgentKey))
		if key != "" {
			keyMeta[key] = struct {
				Member MemberDef
				Agent  biz.Agent
			}{Member: m, Agent: ag}
		}
	}

	synthKey := strings.ToLower(strings.TrimSpace(plan.streamAuthor))
	var totalIn, totalOut int
	var lastForClient biz.ChatMessage
	var unaryAssistants []biz.ChatMessage

	var toolRelay *agent.ChatToolSSERelay
	if streaming {
		toolRelay = agent.NewChatToolSSERelay(stream)
	}

	for ev, err := range rn.Run(ctx, uid, sess.ID, msg, cfg) {
		if ctx.Err() != nil {
			r.finishRunErr(ctx, &run, t0, ctx.Err().Error())
			return userMsg, biz.ChatMessage{}, ctx.Err()
		}
		if err != nil {
			r.finishRunErr(ctx, &run, t0, err.Error())
			return userMsg, biz.ChatMessage{}, err
		}
		if ev == nil {
			continue
		}
		if streaming && toolRelay != nil {
			agEv := teamAgentForToolSSE(ev.Author, keyMeta, firstAg)
			if agent.SessionHasFunctionResponses(ev) {
				toolRelay.EmitToolResponses(agEv, ev)
			}
		}
		if streaming && ev.LLMResponse.Partial {
			if toolRelay != nil {
				agEv := teamAgentForToolSSE(ev.Author, keyMeta, firstAg)
				if agent.SessionHasFunctionCalls(ev) {
					toolRelay.EmitToolCalls(agEv, ev)
				}
			}
			auth := strings.ToLower(strings.TrimSpace(ev.Author))
			if auth == synthKey {
				main, rsn := provider.TextsFromLLMResponse(&ev.LLMResponse)
				if main != "" {
					_ = stream.Emit("delta", map[string]string{"content": main})
				}
				if rsn != "" {
					_ = stream.Emit("delta", map[string]string{"reasoning_content": rsn})
				}
			}
			continue
		}
		if !ev.IsFinalResponse() || ev.LLMResponse.Partial {
			continue
		}
		auth := strings.ToLower(strings.TrimSpace(ev.Author))
		if streaming && toolRelay != nil && agent.SessionHasFunctionCalls(ev) {
			agEv := teamAgentForToolSSE(ev.Author, keyMeta, firstAg)
			toolRelay.EmitToolCalls(agEv, ev)
		}
		meta, ok := keyMeta[auth]
		if !ok {
			continue
		}
		main, rsn := provider.TextsFromLLMResponse(&ev.LLMResponse)
		main = strings.TrimSpace(main)
		opts, e := agent.AssistantOptionsJSON(meta.Agent, &agent.TeamMemberAnchor{
			AgentID: meta.Agent.ID,
			Name:    firstNonEmptyStr(meta.Agent.DisplayName, meta.Agent.AgentKey),
			Role:    meta.Member.Role,
		})
		if e != nil {
			r.finishRunErr(ctx, &run, t0, e.Error())
			return userMsg, biz.ChatMessage{}, e
		}
		if rsn != "" {
			if opts, err = agent.MergeReasoningIntoAssistantOptionsJSON(opts, rsn); err != nil {
				r.finishRunErr(ctx, &run, t0, err.Error())
				return userMsg, biz.ChatMessage{}, err
			}
		}
		am := biz.ChatMessage{
			ID:              uuid.NewString(),
			SessionID:       sess.ID,
			Role:            "assistant",
			ContentMarkdown: main,
			ModelName:       firstNonEmptyStr(modOpt, sess.Model, meta.Agent.Model),
			Status:          "ok",
			OptionsJSON:     opts,
			CreatedAt:       agent.RFC3339Now(),
		}
		totalIn += am.TokenIn
		totalOut += am.TokenOut
		if streaming {
			if err := r.sessions.AppendChatMessage(ctx, sess.ID, am, true); err != nil {
				r.finishRunErr(ctx, &run, t0, err.Error())
				return userMsg, biz.ChatMessage{}, err
			}
		} else {
			unaryAssistants = append(unaryAssistants, am)
		}
		r.persistStep(ctx, run, teamRow.ID, sortIndexForMember(plan.persistMembers, meta.Member), meta.Member, meta.Agent, content, am)
		if auth == synthKey {
			lastForClient = am
		}
	}

	assistantMsg = lastForClient
	if !streaming {
		if len(unaryAssistants) == 0 {
			err := kerrors.InternalServer("CHAT_TEAM_NATIVE", "team workflow produced no assistant messages")
			r.finishRunErr(ctx, &run, t0, err.Error())
			return userMsg, biz.ChatMessage{}, err
		}
		if err := r.sessions.AppendChatTurn(ctx, sess.ID, userMsg, unaryAssistants[0]); err != nil {
			r.finishRunErr(ctx, &run, t0, err.Error())
			return userMsg, biz.ChatMessage{}, err
		}
		for i := 1; i < len(unaryAssistants); i++ {
			if err := r.sessions.AppendChatMessage(ctx, sess.ID, unaryAssistants[i], true); err != nil {
				r.finishRunErr(ctx, &run, t0, err.Error())
				return userMsg, biz.ChatMessage{}, err
			}
		}
		assistantMsg = unaryAssistants[len(unaryAssistants)-1]
	} else if stream != nil {
		_ = stream.Emit("done", assistantMsg)
	}

	run.Status = "success"
	run.TokenIn = totalIn
	run.TokenOut = totalOut
	run.DurationMS = int(time.Since(t0).Milliseconds())
	run.OutputPreview = preview(assistantMsg.ContentMarkdown, 512)
	run.FinishedAt = agent.RFC3339Now()
	_ = r.teams.UpdateTeamRun(ctx, run)
	if r.broker != nil {
		cp := run
		r.broker.Publish(biz.TeamRunEvent{Type: "run_finished", TeamID: teamRow.ID, RunID: run.ID, Run: &cp})
	}
	biz.HintTeamRunSSE(ctx, r.broker, r.teams, teamRow.ID)
	return userMsg, assistantMsg, nil
}

func sortIndexForMember(order []MemberDef, m MemberDef) int {
	for i, x := range order {
		if strings.TrimSpace(x.AgentID) == strings.TrimSpace(m.AgentID) && strings.TrimSpace(x.Role) == strings.TrimSpace(m.Role) {
			return i
		}
	}
	return 0
}

func teamAgentForToolSSE(author string, keyMeta map[string]struct {
	Member MemberDef
	Agent  biz.Agent
}, fallback biz.Agent) biz.Agent {
	auth := strings.ToLower(strings.TrimSpace(author))
	if meta, ok := keyMeta[auth]; ok {
		return meta.Agent
	}
	return agent.AgentForToolSSELabel(author, fallback)
}

func extractOpts(req *chatv1.SendChatMessageRequest) (dialogMode, prov, mod string, attN int) {
	if req == nil {
		return "", "", "", 0
	}
	o := req.GetOptions()
	if o == nil {
		return "", "", "", 0
	}
	return strings.TrimSpace(o.GetDialogMode()), strings.TrimSpace(o.GetProvider()), strings.TrimSpace(o.GetModel()), len(o.Attachments)
}

func (r *Runner) persistStep(ctx context.Context, run biz.TeamRun, teamID string, sortIdx int, m MemberDef, ag biz.Agent, userContent string, asst biz.ChatMessage) {
	step := biz.TeamRunStep{
		ID:            uuid.NewString(),
		RunID:         run.ID,
		TeamID:        teamID,
		AgentID:       ag.ID,
		AgentKey:      ag.AgentKey,
		AgentName:     firstNonEmptyStr(ag.DisplayName, ag.AgentKey),
		Role:          m.Role,
		SortOrder:     sortIdx,
		Status:        asst.Status,
		InputPreview:  preview(userContent, 400),
		OutputPreview: preview(asst.ContentMarkdown, 400),
		TokenIn:       asst.TokenIn,
		TokenOut:      asst.TokenOut,
		CostMicroUSD:  0,
		DurationMS:    asst.LatencyMS,
		ErrorMessage:  asst.ErrorMessage,
		StartedAt:     asst.CreatedAt,
		FinishedAt:    asst.CreatedAt,
		CreatedAt:     agent.RFC3339Now(),
	}
	saved, err := r.teams.CreateTeamRunStep(ctx, step)
	if err != nil {
		return
	}
	if r.broker != nil {
		r.broker.Publish(biz.TeamRunEvent{Type: "step_finished", TeamID: teamID, RunID: run.ID, Step: &saved})
	}
}

func (r *Runner) finishRunErr(ctx context.Context, run *biz.TeamRun, t0 time.Time, msg string) {
	if run == nil {
		return
	}
	run.Status = "failed"
	run.ErrorMessage = msg
	run.FinishedAt = agent.RFC3339Now()
	run.DurationMS = int(time.Since(t0).Milliseconds())
	_ = r.teams.UpdateTeamRun(ctx, *run)
	if r.broker != nil {
		cp := *run
		r.broker.Publish(biz.TeamRunEvent{Type: "run_finished", TeamID: run.TeamID, RunID: run.ID, Run: &cp})
	}
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
