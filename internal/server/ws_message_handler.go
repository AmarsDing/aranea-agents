package server

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
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
			s.canceller.CancelRun(context.Background(), wc.sessionID)
		}

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
		safego.Go(appctx.Ctx(), "ws-user-message", func() {
			if err := s.turnExecutor.ExecuteTurn(appctx.Ctx(), input); err != nil {
				s.lg.With(loggateway.SessionID(sessionID)).Warn("WebSocket 用户消息发送失败", loggateway.StepID("ws.send_failed"), loggateway.Err(err))
				env := event.NewEnvelope(event.EnvelopeTypeError, "ws-handler", sessionID)
				env.RequestID = requestID
				env.Error = &event.EnvelopeError{
					Type:    "send_failed",
					Message: err.Error(),
				}
				s.eventBus.Publish(context.Background(), env)
				s.lg.With(loggateway.SessionID(sessionID)).Info("WebSocket error envelope published",
					loggateway.StepID("ws.error_envelope_published"),
					loggateway.Any("envelope_id", env.ID),
					loggateway.Any("channel", env.Channel),
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
	safego.Go(appctx.Ctx(), "ws-user-message", func() {
		_, err := s.sender.SendChatMessage(appctx.Ctx(), req)
		if err != nil {
			s.lg.With(loggateway.SessionID(sessionID)).Warn("WebSocket 用户消息发送失败", loggateway.StepID("ws.send_failed"), loggateway.Err(err))
			env := event.NewEnvelope(event.EnvelopeTypeError, "ws-handler", sessionID)
			env.RequestID = requestID
			env.Error = &event.EnvelopeError{
				Type:    "send_failed",
				Message: err.Error(),
			}
			s.eventBus.Publish(context.Background(), env)
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
	safego.Go(appctx.Ctx(), "ws-enqueue-message", func() {
		resp, err := s.sender.EnqueueUserMessage(appctx.Ctx(), req)
		if err != nil {
			s.lg.With(loggateway.SessionID(sessionID)).Warn("WebSocket 入队消息发送失败", loggateway.StepID("ws.send_failed"), loggateway.Err(err))
			env := event.NewEnvelope(event.EnvelopeTypeError, "ws-handler", sessionID)
			env.RequestID = requestID
			env.Error = &event.EnvelopeError{
				Type:    "enqueue_failed",
				Message: err.Error(),
			}
			s.eventBus.Publish(context.Background(), env)
			return
		}
		if resp == nil || !resp.GetAccepted() {
			env := event.NewEnvelope(event.EnvelopeTypeError, "ws-handler", sessionID)
			env.RequestID = requestID
			env.Error = &event.EnvelopeError{
				Type:    "enqueue_rejected",
				Message: "no active run for session",
			}
			s.eventBus.Publish(context.Background(), env)
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
