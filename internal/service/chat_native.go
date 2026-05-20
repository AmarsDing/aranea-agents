package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	a2apkg "aranea-agents/internal/a2a"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/strutil"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"
)

func chatNowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func chatMessageToMap(m biz.ChatMessage) map[string]any {
	return map[string]any{
		"id":                m.ID,
		"session_id":        m.SessionID,
		"parent_message_id": m.ParentMessageID,
		"turn_index":        m.TurnIndex,
		"role":              m.Role,
		"content_markdown":  m.ContentMarkdown,
		"model_name":        m.ModelName,
		"token_in":          m.TokenIn,
		"token_out":         m.TokenOut,
		"latency_ms":        m.LatencyMS,
		"status":            m.Status,
		"attachments_count": m.AttachmentsCount,
		"options_json":      m.OptionsJSON,
		"error_message":     m.ErrorMessage,
		"created_at":        m.CreatedAt,
	}
}

func nativeDialogModeChatOptions() []*chatv1.ChatOption {
	return []*chatv1.ChatOption{
		{Type: "dialog_mode", Key: "default", Label: "标准对话", Enabled: true, SortOrder: 1},
		{Type: "dialog_mode", Key: "plan", Label: "深思考", Enabled: true, SortOrder: 2},
		{Type: "dialog_mode", Key: "code", Label: "仅代码", Enabled: true, SortOrder: 3},
	}
}

func (s *ChatService) nativeGetChatOptions(ctx context.Context, req *chatv1.GetChatOptionsRequest) (*chatv1.GetChatOptionsResponse, error) {
	typed := strings.TrimSpace(req.GetType())
	switch typed {
	case "", "dialog_mode":
		return &chatv1.GetChatOptionsResponse{Items: nativeDialogModeChatOptions()}, nil
	case "provider":
		return s.nativeGetProviderOptions(ctx)
	case "model":
		return s.nativeGetModelOptions(ctx)
	default:
		return &chatv1.GetChatOptionsResponse{Items: nil}, nil
	}
}

func (s *ChatService) nativeGetProviderOptions(ctx context.Context) (*chatv1.GetChatOptionsResponse, error) {
	if s.td.Catalog.LLM == nil {
		return &chatv1.GetChatOptionsResponse{Items: nil}, nil
	}
	rows, err := s.td.Catalog.LLM.List(ctx)
	if err != nil {
		return &chatv1.GetChatOptionsResponse{Items: nil}, nil
	}
	seen := make(map[string]struct{})
	var items []*chatv1.ChatOption
	for _, row := range rows {
		p := strings.TrimSpace(row.Provider)
		if p == "" || row.Enabled == false {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		items = append(items, &chatv1.ChatOption{
			Type:      "provider",
			Key:       p,
			Label:     p,
			Enabled:   true,
			SortOrder: int32(len(items) + 1),
		})
	}
	return &chatv1.GetChatOptionsResponse{Items: items}, nil
}

func (s *ChatService) nativeGetModelOptions(ctx context.Context) (*chatv1.GetChatOptionsResponse, error) {
	if s.td.Catalog.LLM == nil {
		return &chatv1.GetChatOptionsResponse{Items: nil}, nil
	}
	rows, err := s.td.Catalog.LLM.List(ctx)
	if err != nil {
		return &chatv1.GetChatOptionsResponse{Items: nil}, nil
	}
	var items []*chatv1.ChatOption
	for i, row := range rows {
		if row.Enabled == false {
			continue
		}
		mj := "{}"
		if row.Provider != "" || row.Model != "" {
			mj = fmt.Sprintf(`{"provider":"%s","model":"%s"}`, row.Provider, row.Model)
		}
		label := row.Name
		if label == "" {
			label = row.Key
		}
		if label == "" {
			label = row.Model
		}
		items = append(items, &chatv1.ChatOption{
			Type:         "model",
			Key:          row.Key,
			Label:        label,
			Enabled:      true,
			SortOrder:    int32(i + 1),
			MetadataJson: mj,
		})
	}
	return &chatv1.GetChatOptionsResponse{Items: items}, nil
}

func (s *ChatService) nativeSendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
	userMsg, assistantMsg, err := s.runNativeAgentTurn(ctx, req)
	if err != nil {
		return nil, err
	}
	um := chatMessageToMap(userMsg)
	am := chatMessageToMap(assistantMsg)
	out := &chatv1.SendChatMessageResponse{}
	if st, err := structpb.NewStruct(um); err != nil {
		return nil, kerrors.InternalServer("CHAT_NATIVE", fmt.Sprintf("encode user_message: %v", err))
	} else {
		out.UserMessage = st
	}
	if st, err := structpb.NewStruct(am); err != nil {
		return nil, kerrors.InternalServer("CHAT_NATIVE", fmt.Sprintf("encode agent_message: %v", err))
	} else {
		out.AgentMessage = st
	}
	// 用量由 trpc_turn defer → recordTurnUsage 写入；勿再调用 recordChatIngressUsage 以免双写。
	if tid := strings.TrimSpace(req.GetTeamId()); tid != "" {
		if s.td.Pipeline.Bus != nil {
			env := event.NewEnvelope(event.EnvelopeTypeTeamRunFinished, "chat-native", "")
			env.TeamID = tid
			env.Metadata = map[string]any{"hint": true}
			s.td.Pipeline.Bus.Publish(ctx, env)
		}
	}
	return out, nil
}

func (s *ChatService) runNativeAgentTurn(ctx context.Context, req *chatv1.SendChatMessageRequest) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	content := strings.TrimSpace(req.GetContent())
	if sessionID == "" || content == "" {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "session_id and content are required")
	}

	// #region debug-point A:turn-entry
	flow := event.NewFlowLogger(s.td.Pipeline.Bus, s.td.Pipeline.Buffer, sessionID, "")
	flow.LogStart("chat.receive", "收到用户消息", event.P("content_len", len(content)))
	// #endregion

	unlock := s.lockSession(sessionID)
	// #region debug-point B:has-active
	hasActive := s.runs.HasActive(sessionID)
	flow.Log("chat.active_check", event.FlowPhaseDone, "检查活跃运行", event.P("has_active", hasActive))
	// #endregion
	if hasActive {
		if _, _, ok := s.runs.ActiveRunner(sessionID); ok {
			unlock()
			enqueued, err := s.runs.EnqueueUserMessage(sessionID, content)
			if err != nil {
				return biz.ChatMessage{}, biz.ChatMessage{}, err
			}
			if enqueued {
				s.publishMessageQueued(sessionID)
				return biz.ChatMessage{}, biz.ChatMessage{}, nil
			}
			pendingID := s.pending.Enqueue(sessionID, content)
			if pendingID == "" {
				return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT", "pending queue is full for this session")
			}
			s.publishMessageQueued(sessionID)
			return biz.ChatMessage{}, biz.ChatMessage{}, nil
		}
		// Stale placeholder from a crashed/partial run — clear and start a fresh turn.
		s.runs.Finish(sessionID)
	}

	sess, err := s.td.Sessions.Get(ctx, sessionID)
	if err != nil {
		unlock()
		flow.LogError("chat.session_fetch", "获取会话失败", event.P("error", err.Error()))
		if errors.Is(err, sql.ErrNoRows) {
			return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.NotFound("SESSION", "session not found")
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	// #region debug-point C:session-state
	flow.LogDone("chat.session_fetch", "会话已获取", event.P("owner_type", sess.OwnerType), event.P("agent_id", sess.AgentID), event.P("team_id", sess.TeamID))
	// #endregion

	if strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team") {
		if s.teamsNative == nil {
			unlock()
			return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.InternalServer("CHAT_TEAM_NATIVE", "team runner not wired")
		}
		if qerr := enforceChatTurnQuotas(ctx, s.usage, "", chatagent.UserIDFromCtx(ctx)); qerr != nil {
			unlock()
			return biz.ChatMessage{}, biz.ChatMessage{}, qerr
		}
		if qerr := s.checkTeamMemberQuotas(ctx, strings.TrimSpace(sess.TeamID)); qerr != nil {
			unlock()
			return biz.ChatMessage{}, biz.ChatMessage{}, qerr
		}
		flow.LogStart("chat.team.invoke", "委派团队会话",
			event.P("team_id", strings.TrimSpace(sess.TeamID)), event.P("content_len", len(content)))
		runID := uuid.NewString()
		teamCtx, teamCancel := context.WithCancel(ctx)
		s.runs.StoreCancelable(sessionID, runID, teamCancel)
		s.setRunStatus(sessionID, runID, "running", "")
		unlock()
		defer func() {
			s.runs.Finish(sessionID)
			s.processPendingQueue(sessionID, sess, biz.Agent{}, "", "", "")
		}()
		userMsg, assistantMsg, err := s.teamsNative.RunTurn(teamCtx, sess, req)
		if err != nil {
			s.setRunStatus(sessionID, runID, "failed", err.Error())
		} else {
			s.setRunStatus(sessionID, runID, "completed", "")
			s.recordTeamSessionTurn(ctx, sessionID, strings.TrimSpace(sess.TeamID),
				userMsg.ID, assistantMsg.ID, "", "",
				assistantMsg.TokenIn, assistantMsg.TokenOut, assistantMsg.ContentMarkdown)
		}
		return userMsg, assistantMsg, err
	}

	if rtid := strings.TrimSpace(req.GetTeamId()); rtid != "" {
		unlock()
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.Forbidden("CHAT_TEAM_NATIVE", "team_id is only valid for team sessions")
	}

	agentID := strings.TrimSpace(sess.AgentID)
	if agentID == "" {
		unlock()
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "session has no agent_id")
	}
	ag, err := s.hydratedAgent(ctx, agentID)
	if err != nil {
		unlock()
		flow.LogError("chat.agent_hydrate", "加载Agent配置失败", event.P("agent_id", agentID), event.P("error", err.Error()))
		if errors.Is(err, sql.ErrNoRows) {
			return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.NotFound("AGENT", "agent not found")
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	// #region debug-point D:agent-state
	flow.LogDone("chat.agent_hydrate", "Agent配置已加载", event.P("agent_key", ag.AgentKey), event.P("provider", ag.Provider), event.P("model", ag.Model))
	// #endregion
	if err := enforceChatTurnQuotas(ctx, s.usage, agentID, chatagent.UserIDFromCtx(ctx)); err != nil {
		unlock()
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	opts := req.GetOptions()
	dialogMode := ""
	prov := ""
	mod := ""
	if opts != nil {
		dialogMode = strings.TrimSpace(opts.GetDialogMode())
		prov = strings.TrimSpace(opts.GetProvider())
		mod = strings.TrimSpace(opts.GetModel())
	}
	dialogMode = strutil.FirstNonEmpty(dialogMode, sess.DialogMode, "default")
	prov = strutil.FirstNonEmpty(prov, sess.DefaultProvider, ag.Provider)
	mod = strutil.FirstNonEmpty(mod, sess.DefaultModel, ag.Model)
	flow.LogDone("chat.provider_resolve", "Provider/Model已解析", event.P("provider", prov), event.P("model", mod), event.P("dialog_mode", dialogMode))

	attN := 0
	if opts != nil {
		attN = len(opts.Attachments)
	}

	// #region debug-point E:enter-turn
	flow.LogStart("chat.turn.enter", "进入Agent Turn执行", event.P("dialog_mode", dialogMode), event.P("provider", prov), event.P("model", mod), event.P("attachments", attN))
	// #endregion

	s.runs.StorePlaceholder(sessionID)
	unlock()
	return s.runSingleAgentViaTRPC(ctx, sess, req, ag, dialogMode, prov, mod, attN)
}

func (s *ChatService) hydratedAgent(ctx context.Context, agentID string) (biz.Agent, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return biz.Agent{}, kerrors.BadRequest("CHAT_NATIVE", "agent id is required")
	}
	if s.td.Catalog.AgentsUC != nil {
		return s.td.Catalog.AgentsUC.Get(ctx, agentID)
	}
	if s.td.Catalog.Agents == nil {
		return biz.Agent{}, kerrors.InternalServer("CHAT_NATIVE", "agent repository not configured")
	}
	return s.td.Catalog.Agents.GetAgentByID(ctx, agentID)
}

// RunNativeTurnUnary runs the native in-process agent/team turn, ignoring LEGACY_REST_ORIGIN (for Channel webhooks).
func (s *ChatService) RunNativeTurnUnary(ctx context.Context, req *chatv1.SendChatMessageRequest) (biz.ChatMessage, biz.ChatMessage, error) {
	return s.runNativeAgentTurn(ctx, req)
}

// RunAgentTurn implements a2a.AgentTurnRunner for call_agent and HTTP Invoke dispatch (EP-A2A-01).
func (s *ChatService) RunAgentTurn(ctx context.Context, agentID, input string, timeoutSec int) (string, error) {
	if s == nil || s.td.Sessions == nil {
		return "", kerrors.InternalServer("A2A", "chat service not configured")
	}
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	sess, err := s.td.Sessions.Create(runCtx, biz.Session{
		ID:        uuid.NewString(),
		AgentID:   strings.TrimSpace(agentID),
		OwnerType: "agent",
		Title:     fmt.Sprintf("a2a-%s", agentID),
		UserID:    "1",
	})
	if err != nil {
		return "", kerrors.InternalServer("A2A", "create session: "+err.Error())
	}
	_, asst, err := s.RunNativeTurnUnary(runCtx, &chatv1.SendChatMessageRequest{
		SessionId: sess.ID,
		Content:   strings.TrimSpace(input),
	})
	if err != nil {
		return "", err
	}
	return asst.ContentMarkdown, nil
}

func (s *ChatService) injectA2AContext(ctx context.Context, callerAgentID string) context.Context {
	if s == nil || s.a2aUC == nil {
		return ctx
	}
	inv := a2apkg.NewInvoker(s, s.a2aUC, s.td.Catalog.Agents)
	return a2apkg.InjectRunContext(ctx, s.a2aUC, callerAgentID, inv)
}

// RunCronTurn dispatches a cron-triggered turn through the in-process agent runner
// with all plugins applied (EP-RT-07). Implements cronrunner.CronChatRunner and
// cronrunner.SessionRunControl via HasActiveRun / RunGateway.
func (s *ChatService) RunCronTurn(ctx context.Context, sessionID, content, teamID string) (userMsgID, agentMsgID string, err error) {
	req := &chatv1.SendChatMessageRequest{
		SessionId: sessionID,
		Content:   content,
	}
	if strings.TrimSpace(teamID) != "" {
		tid := teamID
		req.TeamId = &tid
	}
	user, asst, err := s.RunNativeTurnUnary(ctx, req)
	if err != nil {
		return "", "", err
	}
	return user.ID, asst.ID, nil
}

func patchSessionContextUsage(ctx context.Context, s *ChatService, sessionID string, ag biz.Agent, promptTok, completionTok int) {
	if s == nil || s.td.Sessions == nil {
		return
	}
	win := ag.ContextWindow
	if win <= 0 {
		win = 128000
	}
	_ = s.td.Sessions.UpdateSessionContextFromLLMUsage(ctx, sessionID, promptTok, completionTok, win)
	if s.td.Compress != nil {
		s.td.Compress.AfterNativeTurn(ctx, sessionID, ag)
	}
}
