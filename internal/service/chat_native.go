package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/strutil"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
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
	if typed != "" && typed != "dialog_mode" {
		return &chatv1.GetChatOptionsResponse{Items: nil}, nil
	}
	return &chatv1.GetChatOptionsResponse{Items: nativeDialogModeChatOptions()}, nil
}

func (s *ChatService) nativeSendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
	userMsg, assistantMsg, err := s.runNativeAgentTurn(ctx, req, nil)
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
		biz.HintTeamRunSSE(ctx, s.td.TeamSSE, s.teams, tid)
	}
	return out, nil
}

// streamWriter is used for SSE streaming; when nil, errors are returned from runNativeAgentTurn instead of written as SSE.
type streamWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func (w *streamWriter) writeEvent(event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(w.w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	w.flusher.Flush()
	return nil
}

// Emit implements agent.StreamEmitter for native SSE.
func (w *streamWriter) Emit(event string, payload any) error {
	switch event {
	case "user_message":
		m, ok := payload.(biz.ChatMessage)
		if !ok {
			return nil
		}
		return w.writeEvent("user_message", chatMessageToMap(m))
	case "delta":
		return w.writeEvent("delta", payload)
	case "done":
		m, ok := payload.(biz.ChatMessage)
		if !ok {
			return nil
		}
		return w.writeEvent("done", map[string]any{"agent_message": chatMessageToMap(m)})
	default:
		return w.writeEvent(event, payload)
	}
}

func (s *ChatService) proxyNativeStream(ctx khttp.Context) error {
	req := ctx.Request()
	body, err := io.ReadAll(io.LimitReader(req.Body, 4<<20))
	if err != nil {
		return err
	}
	var payload struct {
		SessionID string `json:"session_id"`
		AgentKey  string `json:"agent_key"`
		TeamID    string `json:"team_id"`
		Content   string `json:"content"`
		Options   struct {
			DialogMode string `json:"dialog_mode"`
			Provider   string `json:"provider"`
			Model      string `json:"model"`
		} `json:"options"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(ctx.Response(), "invalid JSON body", http.StatusBadRequest)
		return nil
	}
	protoReq := &chatv1.SendChatMessageRequest{
		SessionId: strings.TrimSpace(payload.SessionID),
		Content:   strings.TrimSpace(payload.Content),
	}
	if ak := strings.TrimSpace(payload.AgentKey); ak != "" {
		protoReq.AgentKey = &ak
	}
	if tid := strings.TrimSpace(payload.TeamID); tid != "" {
		protoReq.TeamId = &tid
	}
	if payload.Options.DialogMode != "" || payload.Options.Provider != "" || payload.Options.Model != "" {
		protoReq.Options = &chatv1.SendMessageOptions{}
		if payload.Options.DialogMode != "" {
			protoReq.Options.DialogMode = &payload.Options.DialogMode
		}
		if payload.Options.Provider != "" {
			protoReq.Options.Provider = &payload.Options.Provider
		}
		if payload.Options.Model != "" {
			protoReq.Options.Model = &payload.Options.Model
		}
	}

	w := ctx.Response()
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return nil
	}
	sw := &streamWriter{w: w, flusher: flusher}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	_, assistantMsg, err := s.runNativeAgentTurn(req.Context(), protoReq, sw)
	if err != nil {
		_ = sw.writeEvent("error", map[string]string{"message": err.Error()})
		return nil
	}
	recordChatIngressUsage(ctx, s.usage, protoReq, chatMessageToMap(assistantMsg), true)
	return nil
}

func (s *ChatService) runNativeAgentTurn(ctx context.Context, req *chatv1.SendChatMessageRequest, stream *streamWriter) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	content := strings.TrimSpace(req.GetContent())
	if sessionID == "" || content == "" {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "session_id and content are required")
	}

	sess, err := s.td.Sessions.Get(ctx, sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.NotFound("SESSION", "session not found")
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	if strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team") {
		if s.teamsNative == nil {
			return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.InternalServer("CHAT_TEAM_NATIVE", "team runner not wired")
		}
		if s.td.MonitorLogs != nil {
			transport := "unary"
			if stream != nil {
				transport = "sse"
			}
			s.td.MonitorLogs.Publish(ctx, "INFO", fmt.Sprintf(
				"chat_native phase=team_invoke session_id=%s team_id=%s content_len=%d transport=%s",
				sess.ID, strings.TrimSpace(sess.TeamID), len(content), transport), "chat-native")
		}
		var emitter agent.StreamEmitter
		if stream != nil {
			emitter = stream
		}
		return s.teamsNative.RunTurn(ctx, sess, req, emitter)
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
		if err == sql.ErrNoRows {
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

	return s.runAgentTurn(ctx, sess, req, ag, dialogMode, prov, mod, attN, stream)
}

func (s *ChatService) runAgentTurn(
	ctx context.Context,
	sess biz.Session,
	req *chatv1.SendChatMessageRequest,
	ag biz.Agent,
	dialogMode, prov, mod string,
	attN int,
	stream *streamWriter,
) (biz.ChatMessage, biz.ChatMessage, error) {
	return s.runSingleAgentViaTRPC(ctx, sess, req, ag, dialogMode, prov, mod, attN, stream)
}

func (s *ChatService) hydratedAgent(ctx context.Context, agentID string) (biz.Agent, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return biz.Agent{}, kerrors.BadRequest("CHAT_NATIVE", "agent id is required")
	}
	if s.td.AgentsUC != nil {
		return s.td.AgentsUC.Get(ctx, agentID)
	}
	if s.td.Agents == nil {
		return biz.Agent{}, kerrors.InternalServer("CHAT_NATIVE", "agent repository not configured")
	}
	if s.td.ToolsCatalog != nil {
		ephemeral := biz.NewAgentUsecase(s.td.Agents, s.td.ToolsCatalog)
		return ephemeral.Get(ctx, agentID)
	}
	return s.td.Agents.GetAgentByID(ctx, agentID)
}

// RunNativeTurnUnary runs the native in-process agent/team turn, ignoring LEGACY_REST_ORIGIN (for Channel webhooks).
func (s *ChatService) RunNativeTurnUnary(ctx context.Context, req *chatv1.SendChatMessageRequest) (biz.ChatMessage, biz.ChatMessage, error) {
	return s.runNativeAgentTurn(ctx, req, nil)
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
