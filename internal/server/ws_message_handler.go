package server

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

// handleUpstream dispatches an incoming client message to the appropriate handler.
func (s *WSServer) handleUpstream(wc *wsConn, raw []byte) {
	var up wsUpstream
	if err := json.Unmarshal(raw, &up); err != nil {
		s.lg.Warn("WebSocket 上行消息解析失败", loggateway.StepID("ws.parse_error"), loggateway.Err(err))
		return
	}
	if up.Direction != "client_to_server" {
		return
	}
	switch up.Type {
	case "ping":
		wc.sendSystemDownstream(wsDownstream{
			Direction: "server_to_client",
			Channel:   "system",
			Type:      "pong",
			Payload: map[string]any{
				"server_time": time.Now().UTC().Format(time.RFC3339Nano),
			},
		})

	case "subscribe":
		payload, ok := up.Payload.(map[string]any)
		if !ok {
			return
		}
		ch, _ := payload["channel"].(string)
		if ch != "" {
			wc.setChannel(ch, true)
		}
		if fk, _ := payload["filter_key"].(string); fk != "" {
			wc.setFilterKey(fk)
		}

	case "unsubscribe":
		payload, ok := up.Payload.(map[string]any)
		if !ok {
			return
		}
		ch, _ := payload["channel"].(string)
		if ch != "" && ch != "chat" && ch != "system" {
			wc.setChannel(ch, false)
		}

	case "cancel":
		if s.canceller != nil {
			// P0-02 fix: propagate authenticated userID for ownership checks downstream.
			s.canceller.CancelRun(ctxuser.WithUserID(context.Background(), wc.userID), wc.sessionID)
		}

	case "resume_task":
		s.handleResumeTask(wc, up)

	case "enable_log":
		payload, ok := up.Payload.(map[string]any)
		if !ok {
			return
		}
		enabled, _ := payload["enabled"].(bool)
		if enabled && s.serverConf != nil && !s.serverConf.ProcessLogEnabled() {
			return
		}
		if enabled {
			wc.setLogEnabled(true)
			wc.setChannel("monitor", true)
		} else {
			wc.setLogEnabled(false)
			if !wc.globalMode {
				wc.setChannel("monitor", false)
			}
		}

	case "user_message":
		s.handleUserMessage(wc, up)

	case "enqueue_message":
		s.handleEnqueueMessage(wc, up)
	}
}

// handleUserMessage processes a user_message upstream event.
func (s *WSServer) handleUserMessage(wc *wsConn, up wsUpstream) {
	payload, ok := up.Payload.(map[string]any)
	if !ok {
		s.lg.Warn("WebSocket user_message dropped: payload is not an object",
			loggateway.StepID("ws.user_msg_drop"), loggateway.SessionID(wc.sessionID))
		return
	}
	content, _ := payload["content"].(string)
	if strings.TrimSpace(content) == "" {
		s.lg.Warn("WebSocket user_message dropped: empty content",
			loggateway.StepID("ws.user_msg_drop"), loggateway.SessionID(wc.sessionID))
		return
	}

	sessionID := wc.sessionID
	requestID := strings.TrimSpace(up.RequestID)

	// Prefer biz.TurnExecutorGateway when available (Phase B2: unified turn entry point).
	if s.turnExecutor != nil {
		input := WSTurnInput{
			SessionID:   sessionID,
			Content:     strings.TrimSpace(content),
			AllowQueue:  true,
			AllowStream: true,
		}
		if agentKey, _ := payload["agent_key"].(string); agentKey != "" {
			input.AgentKey = agentKey
		}
		if teamID, _ := payload["team_id"].(string); teamID != "" {
			input.TeamID = teamID
		}
		if opts, ok := payload["options"].(map[string]any); ok {
			input.Options = buildWSTurnOptions(opts)
		}
		s.lg.With(loggateway.SessionID(sessionID)).Info("WS handleUserMessage: 开始处理",
			loggateway.StepID("ws.user_msg_start"),
			loggateway.Any("agent_key", input.AgentKey),
			loggateway.Any("team_id", input.TeamID),
			loggateway.Any("content_len", len(content)))
		// Derive from appctx.Ctx() so the turn outlives the WebSocket connection.
		// Disconnecting the WS no longer cancels in-flight turns; users cancel via
		// StopGeneration (RunRegistry.Cancel) instead. Aligns with HTTP path
		// (submitChatMessageAsync) and the No-Timeout principle (2026-06-18).
		//
		// P0-02 fix: propagate the authenticated userID into the turn context so
		// the Runner session key, memory tools, and quota checks use the real user
		// scope instead of default_user.
		turnCtx := ctxuser.WithUserID(appctx.Ctx(), wc.userID)
		safego.Go(turnCtx, "ws-user-message", func() {
			if err := s.turnExecutor.ExecuteTurn(turnCtx, input); err != nil {
				s.lg.With(loggateway.SessionID(sessionID)).Warn("WebSocket 用户消息发送失败", loggateway.StepID("ws.send_failed"), loggateway.Err(err))
				s.publishWSErrorActivity(sessionID, requestID, "send_failed", err.Error(), input.Content)
				s.lg.With(loggateway.SessionID(sessionID)).Info("WebSocket error ActivityEvent published",
					loggateway.StepID("ws.error_activity_published"),
					loggateway.Any("request_id", requestID))
			}
		})
		return
	}

	// Fallback: proto-based ChatSender (legacy path).
	req := &chatv1.SendChatMessageRequest{
		SessionId: sessionID,
		Content:   strings.TrimSpace(content),
	}
	if agentKey, _ := payload["agent_key"].(string); agentKey != "" {
		req.AgentKey = &agentKey
	}
	if teamID, _ := payload["team_id"].(string); teamID != "" {
		req.TeamId = &teamID
	}
	if opts, ok := payload["options"].(map[string]any); ok {
		req.Options = buildChatOptions(opts)
	}

	// Derive from appctx.Ctx() so the turn outlives the WebSocket connection
	// (aligns with HTTP path and No-Timeout principle). Cancellation is handled
	// via StopGeneration (RunRegistry.Cancel), not via the connection context.
	// P0-02 fix: propagate authenticated userID into the turn context.
	legacyCtx := ctxuser.WithUserID(appctx.Ctx(), wc.userID)
	safego.Go(legacyCtx, "ws-user-message", func() {
		_, err := s.sender.SendChatMessage(legacyCtx, req)
		if err != nil {
			s.lg.With(loggateway.SessionID(sessionID)).Warn("WebSocket 用户消息发送失败", loggateway.StepID("ws.send_failed"), loggateway.Err(err))
			s.publishWSErrorActivity(sessionID, requestID, "send_failed", err.Error(), req.Content)
		}
	})
}

// handleResumeTask processes a resume_task upstream event (L3): the user
// clicked "continue" on an interrupted task card. Validation, the CAS claim
// and the asynchronous rerun all live in ChatService.ResumeInterruptedTask;
// here we only translate transport concerns. Failures surface as a ws_error
// notice + synthetic error block (transient, not persisted).
func (s *WSServer) handleResumeTask(wc *wsConn, up wsUpstream) {
	payload, ok := up.Payload.(map[string]any)
	if !ok {
		return
	}
	taskID, _ := payload["task_id"].(string)
	if strings.TrimSpace(taskID) == "" {
		return
	}
	sessionID := wc.sessionID
	requestID := strings.TrimSpace(up.RequestID)
	if s.resumer == nil {
		s.publishWSErrorActivity(sessionID, requestID, "resume_unavailable", "task resume is not available", "")
		return
	}
	// Derive from appctx so the resume outlives the WS connection (aligns
	// with user_message handling). Propagate the authenticated userID.
	resumeCtx := ctxuser.WithUserID(appctx.Ctx(), wc.userID)
	safego.Go(resumeCtx, "ws-resume-task", func() {
		if err := s.resumer.ResumeInterruptedTask(resumeCtx, sessionID, taskID); err != nil {
			s.lg.With(loggateway.SessionID(sessionID)).Warn("WebSocket 任务续跑失败",
				loggateway.StepID("ws.resume_task_failed"), loggateway.Err(err))
			s.publishWSErrorActivity(sessionID, requestID, "resume_failed", err.Error(), "")
		}
	})
}

// handleEnqueueMessage processes an enqueue_message upstream event.
func (s *WSServer) handleEnqueueMessage(wc *wsConn, up wsUpstream) {
	if s == nil || s.sender == nil {
		s.lg.Warn("WebSocket enqueue_message dropped: sender not available",
			loggateway.StepID("ws.enqueue_msg_drop"), loggateway.SessionID(wc.sessionID))
		return
	}
	payload, ok := up.Payload.(map[string]any)
	if !ok {
		s.lg.Warn("WebSocket enqueue_message dropped: payload is not an object",
			loggateway.StepID("ws.enqueue_msg_drop"), loggateway.SessionID(wc.sessionID))
		return
	}
	content, _ := payload["content"].(string)
	if strings.TrimSpace(content) == "" {
		s.lg.Warn("WebSocket enqueue_message dropped: empty content",
			loggateway.StepID("ws.enqueue_msg_drop"), loggateway.SessionID(wc.sessionID))
		return
	}

	sessionID := wc.sessionID
	requestID := strings.TrimSpace(up.RequestID)
	req := &chatv1.EnqueueUserMessageRequest{
		SessionId: sessionID,
		Content:   strings.TrimSpace(content),
	}

	// Derive from appctx.Ctx() so the enqueue outlives the WebSocket connection
	// (aligns with HTTP path and No-Timeout principle).
	// P0-02 fix: propagate authenticated userID into the turn context.
	enqCtx := ctxuser.WithUserID(appctx.Ctx(), wc.userID)
	safego.Go(enqCtx, "ws-enqueue-message", func() {
		resp, err := s.sender.EnqueueUserMessage(enqCtx, req)
		if err != nil {
			s.lg.With(loggateway.SessionID(sessionID)).Warn("WebSocket 入队消息发送失败", loggateway.StepID("ws.send_failed"), loggateway.Err(err))
			s.publishWSErrorActivity(sessionID, requestID, "enqueue_failed", err.Error(), req.Content)
			return
		}
		if resp == nil || !resp.GetAccepted() {
			s.publishWSErrorActivity(sessionID, requestID, "enqueue_rejected", "no active run for session", req.Content)
		}
	})
}

// buildChatOptions builds proto SendMessageOptions from WS payload options.
func buildChatOptions(opts map[string]any) *chatv1.SendMessageOptions {
	result := &chatv1.SendMessageOptions{}
	if dm, _ := opts["dialog_mode"].(string); dm != "" {
		result.DialogMode = &dm
	}
	if p, _ := opts["provider"].(string); p != "" {
		result.Provider = &p
	}
	if m, _ := opts["model"].(string); m != "" {
		result.Model = &m
	}
	if atts, ok := opts["attachments"].([]any); ok {
		for _, att := range atts {
			if m, ok := att.(map[string]any); ok {
				if id, _ := m["id"].(string); id != "" {
					result.Attachments = append(result.Attachments, &chatv1.AttachmentRef{Id: id})
				}
			}
		}
	}
	if kbs, ok := opts["knowledge_bases"].([]any); ok {
		for _, kb := range kbs {
			if s, ok := kb.(string); s != "" && ok {
				result.KnowledgeBases = append(result.KnowledgeBases, s)
			}
		}
	}
	return result
}

// publishWSErrorActivity publishes a system.notice (ws_error) plus synthetic
// v2 Task/Turn/Step events so the timeline can render a stable ErrorBlock.
// Synthetic IDs use the "ws-err-" prefix and are NOT persisted (WS-layer
// publish bypasses the Sequencer), so they vanish on refresh — intended for
// transient send failures the user can retry.
//
// userContent carries the user's original message text so the synthetic Task
// can populate UserMessage (visible in TaskCard). Pass "" when the message
// is unavailable (e.g. enqueue_rejected where no content was sent).
func (s *WSServer) publishWSErrorActivity(sessionID, requestID, errorType, message, userContent string) {
	if s.eventBus == nil {
		return
	}
	meta := map[string]any{
		"error_type": errorType,
		"source":     "ws-handler",
	}
	if requestID != "" {
		meta["request_id"] = requestID
	}
	s.eventBus.Publish(context.Background(), biz.NewSystemNoticeEvent(sessionID, "ws_error", message, meta))

	// P2-05: emit synthetic v2 Task/Turn/Step events so the v2 timeline
	// (ActivityStream / TaskCard / TurnContainer / ErrorBlock) can render a
	// stable failure surface. Synthetic IDs are prefixed with "ws-err-" so
	// they never collide with real entity IDs. These events bypass the
	// Sequencer (WS-layer direct publish), so they are NOT persisted to the
	// database and will disappear on page refresh — appropriate for
	// transient send/enqueue failures that the user can retry.
	synID := requestID
	if synID == "" {
		synID = uuid.NewString()
	}
	taskID := "ws-err-" + synID
	turnID := taskID + "-turn"
	stepID := turnID + "-s1"
	now := time.Now().UTC()
	completedAt := &now

	synTask := biz.Task{
		ID:          taskID,
		SessionID:   sessionID,
		UserMessage: userContent,
		Status:      biz.TaskStatusFailed,
		Seq:         0,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CompletedAt: completedAt,
	}
	synTurn := biz.Turn{
		ID:              turnID,
		TaskID:          taskID,
		SessionID:       sessionID,
		SpiritSessionID: sessionID,
		Seq:             1,
		Version:         1,
		Status:          biz.TurnStatusFailed,
		StartedAt:       now,
		CompletedAt:     completedAt,
	}
	errStep := biz.Step{
		ID:              stepID,
		TurnID:          turnID,
		TaskID:          taskID,
		SessionID:       sessionID,
		SpiritSessionID: sessionID,
		Kind:            biz.StepKindError,
		Seq:             1,
		Version:         1,
		Content:         message,
		ToolErrorCode:   errorType,
		Status:          biz.StepStatusCompleted,
		StartedAt:       now,
		CompletedAt:     completedAt,
	}

	ctx := context.Background()
	s.eventBus.Publish(ctx, biz.NewTaskFailedEvent(synTask))
	s.eventBus.Publish(ctx, biz.NewTurnFailedEvent(synTurn))
	s.eventBus.Publish(ctx, biz.NewStepCreatedEvent(errStep))
	s.eventBus.Publish(ctx, biz.NewStepCompletedEvent(errStep))
}

// buildWSTurnOptions builds a WSTurnOptions from WS payload options.
// Used by the TurnGateway path (Phase B2).
func buildWSTurnOptions(opts map[string]any) WSTurnOptions {
	result := WSTurnOptions{}
	if dm, _ := opts["dialog_mode"].(string); dm != "" {
		result.DialogMode = dm
	}
	if p, _ := opts["provider"].(string); p != "" {
		result.Provider = p
	}
	if m, _ := opts["model"].(string); m != "" {
		result.Model = m
	}
	if atts, ok := opts["attachments"].([]any); ok {
		for _, att := range atts {
			if m, ok := att.(map[string]any); ok {
				if id, _ := m["id"].(string); id != "" {
					result.AttachmentIDs = append(result.AttachmentIDs, id)
				}
			}
		}
	}
	if kbs, ok := opts["knowledge_bases"].([]any); ok {
		for _, kb := range kbs {
			if s, ok := kb.(string); s != "" && ok {
				result.KnowledgeBases = append(result.KnowledgeBases, s)
			}
		}
	}
	return result
}
