package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
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
	recordChatIngressUsage(ctx, s.usage, req, am, false)
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

	if _, running := s.activeRuns.Load(sessionID); running {
		pendingID := s.enqueuePending(sessionID, content)
		if pendingID == "" {
			return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT", "pending queue is full for this session")
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, nil
	}

	sess, err := s.td.Sessions.Get(ctx, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.NotFound("SESSION", "session not found")
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	if strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team") {
		if s.teamsNative == nil {
			return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.InternalServer("CHAT_TEAM_NATIVE", "team runner not wired")
		}
		if s.td.Pipeline.Bus != nil {
			env := event.NewEnvelope(event.EnvelopeTypeLog, "chat-native", sess.ID)
			env.Metadata = map[string]any{"level": "INFO", "source": "chat-native"}
			env.Content = &event.EnvelopeContent{Text: fmt.Sprintf(
				"chat_native phase=team_invoke session_id=%s team_id=%s content_len=%d",
				sess.ID, strings.TrimSpace(sess.TeamID), len(content))}
			s.td.Pipeline.Bus.Publish(ctx, env)
		}
		runID := uuid.NewString()
		teamCtx, teamCancel := context.WithCancel(ctx)
		guard := &teamRunGuard{cancel: teamCancel, runID: runID}
		s.activeRuns.Store(sessionID, guard)
		s.setRunStatus(sessionID, runID, "running", "")
		defer func() {
			s.activeRuns.Delete(sessionID)
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
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.Forbidden("CHAT_TEAM_NATIVE", "team_id is only valid for team sessions")
	}

	agentID := strings.TrimSpace(sess.AgentID)
	if agentID == "" {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "session has no agent_id")
	}
	ag, err := s.hydratedAgent(ctx, agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.NotFound("AGENT", "agent not found")
		}
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

	attN := 0
	if opts != nil {
		attN = len(opts.Attachments)
	}

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

// RunCronTurn dispatches a cron-triggered turn through the in-process agent runner
// with all plugins applied (EP-RT-07). Implements cronrunner.CronChatRunner.
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
