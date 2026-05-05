package team

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

const synthesisCue = "请将上文中团队成员的并行回复整合为一条对用户问题的最终答案，语气连贯，不要解释团队内部流程。"

// Runner executes native team workflows (OpenAI-compat members).
type Runner struct {
	teams    biz.TeamRepository
	sessions *biz.SessionUsecase
	agents   biz.AgentRepository
	agentsUC *biz.AgentUsecase
	catalog  *biz.LlmProviderModelUsecase
	broker   *biz.TeamRunEventBroker
	llmHTTP  *http.Client
}

// NewRunner wires a team runner (llmHTTP should match chat transport timeouts).
func NewRunner(
	teams biz.TeamRepository,
	sessions *biz.SessionUsecase,
	agents biz.AgentRepository,
	agentsUC *biz.AgentUsecase,
	catalog *biz.LlmProviderModelUsecase,
	broker *biz.TeamRunEventBroker,
) *Runner {
	return &Runner{
		teams:    teams,
		sessions: sessions,
		agents:   agents,
		agentsUC: agentsUC,
		catalog:  catalog,
		broker:   broker,
		llmHTTP:  &http.Client{Timeout: 300 * time.Second},
	}
}

func (r *Runner) catalogAgent(ctx context.Context, id string) (biz.Agent, error) {
	if r.agentsUC != nil {
		return r.agentsUC.Get(ctx, id)
	}
	return r.agents.GetAgentByID(ctx, id)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
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
	switch mode {
	case "sequential":
		return r.runSequential(ctx, sess, req, teamRow, def, stream)
	case "parallel":
		return r.runParallel(ctx, sess, req, teamRow, def, stream)
	case "coordinator", "critic_loop", "adaptive":
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.ServiceUnavailable("CHAT_TEAM_NATIVE", fmt.Sprintf("Team mode %q is not supported by the native executor yet; set LEGACY_REST_ORIGIN.", mode))
	default:
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("TEAM", "unsupported team mode")
	}
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
		AgentName:     firstNonEmpty(ag.DisplayName, ag.AgentKey),
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

func (r *Runner) runSequential(ctx context.Context, sess biz.Session, req *chatv1.SendChatMessageRequest, teamRow biz.Team, def Definition, stream agent.StreamEmitter) (biz.ChatMessage, biz.ChatMessage, error) {
	members := EnabledMembers(def)
	if len(members) == 0 {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("TEAM", "team has no enabled members")
	}
	content := strings.TrimSpace(req.GetContent())
	if content == "" {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "content is required")
	}
	dialogMode, provOpt, modOpt, attN := extractOpts(req)
	dialogMode = firstNonEmpty(dialogMode, sess.DialogMode, "default")

	run := biz.TeamRun{
		ID:            uuid.NewString(),
		TeamID:        teamRow.ID,
		SessionID:     sess.ID,
		Mode:          "sequential",
		Status:        "running",
		InputPreview:  preview(content, 512),
		TopologyJSON:  topologyJSON(def),
		StartedAt:     agent.RFC3339Now(),
		CreatedAt:     agent.RFC3339Now(),
		UpdatedAt:     agent.RFC3339Now(),
	}
	run, err := r.teams.CreateTeamRun(ctx, run)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	if r.broker != nil {
		cp := run
		r.broker.Publish(biz.TeamRunEvent{Type: "run_started", TeamID: teamRow.ID, RunID: run.ID, Run: &cp})
	}

	ad := agent.Deps{Agents: r.agents, AgentUC: r.agentsUC, Catalog: r.catalog, HTTP: r.llmHTTP}
	t0 := time.Now()
	var userMsg biz.ChatMessage
	var lastAsst biz.ChatMessage
	var totalIn, totalOut int

	for i, m := range members {
		ag, err := r.catalogAgent(ctx, m.AgentID)
		if err != nil {
			if err == sql.ErrNoRows {
				err = kerrors.NotFound("AGENT", "team member agent not found")
			}
			r.finishRunErr(ctx, &run, t0, err.Error())
			return biz.ChatMessage{}, biz.ChatMessage{}, err
		}
		prov := firstNonEmpty(provOpt, sess.Provider, ag.Provider)
		mod := firstNonEmpty(modOpt, sess.Model, ag.Model)
		anchor := &agent.TeamMemberAnchor{
			AgentID: ag.ID,
			Name:    firstNonEmpty(ag.DisplayName, ag.AgentKey),
			Role:    m.Role,
		}
		stepStream := stream
		if i != len(members)-1 {
			stepStream = nil
		}
		if i == 0 {
			uin := agent.TurnInput{
				SessionID:        sess.ID,
				Agent:            ag,
				UserContent:      content,
				DialogMode:       dialogMode,
				Provider:         prov,
				Model:            mod,
				AgentKeyFromReq:  strings.TrimSpace(req.GetAgentKey()),
				AttachmentsCount: attN,
				ContextRatio:     sess.ContextUsedRatio,
				TeamMember:       anchor,
			}
			var e error
			userMsg, lastAsst, e = agent.ExecuteOpenAICompatTurn(ctx, ad, r.sessions, uin, stepStream)
			if e != nil {
				r.finishRunErr(ctx, &run, t0, e.Error())
				return biz.ChatMessage{}, biz.ChatMessage{}, e
			}
		} else {
			rin := agent.RelayStepInput{
				SessionID:       sess.ID,
				Agent:           ag,
				DialogMode:      dialogMode,
				Provider:        prov,
				Model:           mod,
				AgentKeyFromReq: strings.TrimSpace(req.GetAgentKey()),
				ContextRatio:    sess.ContextUsedRatio,
				TeamMember:      anchor,
			}
			var e error
			lastAsst, e = agent.ExecuteOpenAIRelayStep(ctx, ad, r.sessions, rin, stepStream)
			if e != nil {
				r.finishRunErr(ctx, &run, t0, e.Error())
				return userMsg, biz.ChatMessage{}, e
			}
		}
		totalIn += lastAsst.TokenIn
		totalOut += lastAsst.TokenOut
		r.persistStep(ctx, run, teamRow.ID, i, m, ag, content, lastAsst)
	}

	run.Status = "success"
	run.TokenIn = totalIn
	run.TokenOut = totalOut
	run.DurationMS = int(time.Since(t0).Milliseconds())
	run.OutputPreview = preview(lastAsst.ContentMarkdown, 512)
	run.FinishedAt = agent.RFC3339Now()
	_ = r.teams.UpdateTeamRun(ctx, run)
	if r.broker != nil {
		cp := run
		r.broker.Publish(biz.TeamRunEvent{Type: "run_finished", TeamID: teamRow.ID, RunID: run.ID, Run: &cp})
	}
	biz.HintTeamRunSSE(ctx, r.broker, r.teams, teamRow.ID)
	return userMsg, lastAsst, nil
}

func (r *Runner) runParallel(ctx context.Context, sess biz.Session, req *chatv1.SendChatMessageRequest, teamRow biz.Team, def Definition, stream agent.StreamEmitter) (biz.ChatMessage, biz.ChatMessage, error) {
	workers := ParallelWorkers(def)
	if len(workers) == 0 {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("TEAM", "team has no enabled members")
	}
	content := strings.TrimSpace(req.GetContent())
	if content == "" {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "content is required")
	}
	dialogMode, provOpt, modOpt, attN := extractOpts(req)
	dialogMode = firstNonEmpty(dialogMode, sess.DialogMode, "default")

	synthID := strings.TrimSpace(SynthesizerAgentID(def))
	if len(workers) > 1 && synthID == "" {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("TEAM", "parallel team requires synthesizer_agent_id or a synthesizer member")
	}

	run := biz.TeamRun{
		ID:           uuid.NewString(),
		TeamID:       teamRow.ID,
		SessionID:    sess.ID,
		Mode:         "parallel",
		Status:       "running",
		InputPreview: preview(content, 512),
		TopologyJSON: topologyJSON(def),
		StartedAt:    agent.RFC3339Now(),
		CreatedAt:    agent.RFC3339Now(),
		UpdatedAt:    agent.RFC3339Now(),
	}
	run, err := r.teams.CreateTeamRun(ctx, run)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	if r.broker != nil {
		cp := run
		r.broker.Publish(biz.TeamRunEvent{Type: "run_started", TeamID: teamRow.ID, RunID: run.ID, Run: &cp})
	}

	ad := agent.Deps{Agents: r.agents, AgentUC: r.agentsUC, Catalog: r.catalog, HTTP: r.llmHTTP}
	t0 := time.Now()

	firstAg, err := r.catalogAgent(ctx, workers[0].AgentID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = kerrors.NotFound("AGENT", "team member agent not found")
		}
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	prov0 := firstNonEmpty(provOpt, sess.Provider, firstAg.Provider)
	mod0 := firstNonEmpty(modOpt, sess.Model, firstAg.Model)
	userOpts, err := agent.UserOptionsJSON(firstAg, dialogMode, prov0, mod0, sess.ContextUsedRatio, nil)
	if err != nil {
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	userMsg := biz.ChatMessage{
		ID:               uuid.NewString(),
		SessionID:        sess.ID,
		Role:             "user",
		ContentMarkdown:  content,
		Status:           "ok",
		OptionsJSON:      userOpts,
		CreatedAt:        agent.RFC3339Now(),
		AttachmentsCount: attN,
	}
	if err := r.sessions.AppendChatMessage(ctx, sess.ID, userMsg, false); err != nil {
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	if stream != nil {
		_ = stream.Emit("user_message", userMsg)
	}

	var lastAsst biz.ChatMessage
	var totalIn, totalOut int

	if len(workers) == 1 && synthID == "" {
		ag := firstAg
		prov := firstNonEmpty(provOpt, sess.Provider, ag.Provider)
		mod := firstNonEmpty(modOpt, sess.Model, ag.Model)
		anchor := &agent.TeamMemberAnchor{AgentID: ag.ID, Name: firstNonEmpty(ag.DisplayName, ag.AgentKey), Role: workers[0].Role}
		pin := agent.ParallelWorkerInput{
			SessionID:       sess.ID,
			Agent:           ag,
			UserLine:        content,
			DialogMode:      dialogMode,
			Provider:        prov,
			Model:           mod,
			AgentKeyFromReq: strings.TrimSpace(req.GetAgentKey()),
			TeamMember:      anchor,
			SkipPersist:     false,
			Stream:          stream,
		}
		lastAsst, err = agent.ExecuteOpenAIParallelMember(ctx, ad, r.sessions, pin)
		if err != nil {
			r.finishRunErr(ctx, &run, t0, err.Error())
			return userMsg, biz.ChatMessage{}, err
		}
		totalIn += lastAsst.TokenIn
		totalOut += lastAsst.TokenOut
		r.persistStep(ctx, run, teamRow.ID, 0, workers[0], ag, content, lastAsst)
	} else {
		results := make([]biz.ChatMessage, len(workers))
		eg, egctx := errgroup.WithContext(ctx)
		for i := range workers {
			i := i
			eg.Go(func() error {
				m := workers[i]
				ag, err := r.catalogAgent(egctx, m.AgentID)
				if err != nil {
					return err
				}
				prov := firstNonEmpty(provOpt, sess.Provider, ag.Provider)
				mod := firstNonEmpty(modOpt, sess.Model, ag.Model)
				anchor := &agent.TeamMemberAnchor{AgentID: ag.ID, Name: firstNonEmpty(ag.DisplayName, ag.AgentKey), Role: m.Role}
				pin := agent.ParallelWorkerInput{
					SessionID:       sess.ID,
					Agent:           ag,
					UserLine:        content,
					DialogMode:      dialogMode,
					Provider:        prov,
					Model:           mod,
					AgentKeyFromReq: strings.TrimSpace(req.GetAgentKey()),
					TeamMember:      anchor,
					SkipPersist:     true,
				}
				am, err := agent.ExecuteOpenAIParallelMember(egctx, ad, r.sessions, pin)
				if err != nil {
					return err
				}
				results[i] = am
				return nil
			})
		}
		if err := eg.Wait(); err != nil {
			r.finishRunErr(ctx, &run, t0, err.Error())
			return userMsg, biz.ChatMessage{}, err
		}
		pctx, pcancel := agent.ChatPersistCtx(ctx)
		defer pcancel()
		for i, m := range workers {
			am := results[i]
			ag, err := r.catalogAgent(ctx, m.AgentID)
			if err != nil {
				r.finishRunErr(ctx, &run, t0, err.Error())
				return userMsg, biz.ChatMessage{}, err
			}
			if err := r.sessions.AppendChatMessage(pctx, sess.ID, am, true); err != nil {
				r.finishRunErr(ctx, &run, t0, err.Error())
				return userMsg, biz.ChatMessage{}, err
			}
			totalIn += am.TokenIn
			totalOut += am.TokenOut
			r.persistStep(ctx, run, teamRow.ID, i, m, ag, content, am)
		}
		synthAg, err := r.catalogAgent(ctx, synthID)
		if err != nil {
			if err == sql.ErrNoRows {
				err = kerrors.NotFound("AGENT", "synthesizer agent not found")
			}
			r.finishRunErr(ctx, &run, t0, err.Error())
			return userMsg, biz.ChatMessage{}, err
		}
		sprov := firstNonEmpty(provOpt, sess.Provider, synthAg.Provider)
		smod := firstNonEmpty(modOpt, sess.Model, synthAg.Model)
		sanchor := &agent.TeamMemberAnchor{AgentID: synthAg.ID, Name: firstNonEmpty(synthAg.DisplayName, synthAg.AgentKey), Role: "synthesizer"}
		rin := agent.RelayStepInput{
			SessionID:        sess.ID,
			Agent:            synthAg,
			DialogMode:       dialogMode,
			Provider:         sprov,
			Model:            smod,
			AgentKeyFromReq:  strings.TrimSpace(req.GetAgentKey()),
			ContextRatio:     sess.ContextUsedRatio,
			TeamMember:       sanchor,
			RelayUserContent: synthesisCue,
		}
		lastAsst, err = agent.ExecuteOpenAIRelayStep(ctx, ad, r.sessions, rin, stream)
		if err != nil {
			r.finishRunErr(ctx, &run, t0, err.Error())
			return userMsg, biz.ChatMessage{}, err
		}
		totalIn += lastAsst.TokenIn
		totalOut += lastAsst.TokenOut
		synthMember := MemberDef{AgentID: synthAg.ID, Role: "synthesizer"}
		r.persistStep(ctx, run, teamRow.ID, len(workers), synthMember, synthAg, content, lastAsst)
	}

	run.Status = "success"
	run.TokenIn = totalIn
	run.TokenOut = totalOut
	run.DurationMS = int(time.Since(t0).Milliseconds())
	run.OutputPreview = preview(lastAsst.ContentMarkdown, 512)
	run.FinishedAt = agent.RFC3339Now()
	_ = r.teams.UpdateTeamRun(ctx, run)
	if r.broker != nil {
		cp := run
		r.broker.Publish(biz.TeamRunEvent{Type: "run_finished", TeamID: teamRow.ID, RunID: run.ID, Run: &cp})
	}
	biz.HintTeamRunSSE(ctx, r.broker, r.teams, teamRow.ID)
	return userMsg, lastAsst, nil
}
