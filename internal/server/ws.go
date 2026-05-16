package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/auth"

	"github.com/go-kratos/kratos/v2/transport"
	"github.com/gorilla/websocket"
)

var _ transport.Server = (*WSServer)(nil)

const (
	defaultWSReadLimit  = 1 << 20
	defaultWSPongWait   = 60 * time.Second
	defaultWSPingPeriod = 30 * time.Second
	defaultWSWriteWait  = 10 * time.Second
	maxSessionConns     = 5
)

type RunCanceller interface {
	CancelRun(ctx context.Context, sessionID string) bool
}

type ChatSender interface {
	SendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error)
}

type wsUpstream struct {
	Direction string `json:"direction"`
	Channel   string `json:"channel"`
	Type      string `json:"type"`
	RequestID string `json:"request_id,omitempty"`
	Payload   any    `json:"payload,omitempty"`
}

type wsDownstream struct {
	Direction string          `json:"direction"`
	Channel   string          `json:"channel"`
	Type      string          `json:"type,omitempty"`
	Payload   any             `json:"payload,omitempty"`
	Envelope  *event.Envelope `json:"envelope,omitempty"`
}

type wsConn struct {
	conn        *websocket.Conn
	sessionID   string
	userID      string
	channels    map[string]bool
	filterKey   string
	unsubscribe func()
	send        chan []byte
	replayDone  chan struct{}
	logEnabled  bool
	globalMode  bool
}

type WSServer struct {
	mu          sync.RWMutex
	conns       map[string][]*wsConn
	eventBus    event.Bus
	eventBuffer *event.Buffer
	canceller   RunCanceller
	sender      ChatSender
	upgrader    websocket.Upgrader
	addr        string
	network     string
	srv         *http.Server
	closed      bool
}

func NewWSServer(c *conf.Server, eventBus event.Bus, eventBuffer *event.Buffer, canceller RunCanceller, sender ChatSender) *WSServer {
	if c == nil || c.GetWs() == nil || !c.GetWs().GetEnable() {
		return nil
	}
	wsConf := c.GetWs()
	addr := wsConf.GetAddr()
	if addr == "" {
		addr = ":8002"
	}
	network := wsConf.GetNetwork()
	if network == "" {
		network = "tcp"
	}
	return &WSServer{
		conns:       make(map[string][]*wsConn),
		eventBus:    eventBus,
		eventBuffer: eventBuffer,
		canceller:   canceller,
		sender:      sender,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return originAllowed(r.Header.Get("Origin"))
			},
		},
		addr:    addr,
		network: network,
	}
}

func (s *WSServer) Start(ctx context.Context) error {
	if s == nil {
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ws", s.handleWS)
	s.srv = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}
	go func() {
		slog.Info("ws server listening", "addr", s.addr)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("ws server error", "error", err)
		}
	}()
	return nil
}

func (s *WSServer) Stop(ctx context.Context) error {
	if s == nil || s.srv == nil {
		return nil
	}
	s.closed = true
	s.broadcastShutdown()
	return s.srv.Shutdown(ctx)
}

func (s *WSServer) broadcastShutdown() {
	msg := wsDownstream{
		Direction: "server_to_client",
		Channel:   "system",
		Type:      "server_shutdown",
		Payload: map[string]any{
			"reason":      "server_shutting_down",
			"server_time": time.Now().UTC().Format(time.RFC3339Nano),
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, conns := range s.conns {
		for _, wc := range conns {
			select {
			case wc.send <- data:
			default:
			}
		}
	}
}

func (s *WSServer) handleWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	globalMode := sessionID == "*"

	tokenStr := strings.TrimSpace(r.URL.Query().Get("token"))
	if tokenStr == "" {
		tokenStr = r.Header.Get("Authorization")
		tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")
	}
	if tokenStr == "" {
		if cookie, err := r.Cookie("access_token"); err == nil && cookie.Value != "" {
			tokenStr = cookie.Value
		}
	}
	if tokenStr == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	claims, err := auth.ParseTokenFromRequest(tokenStr)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	userID := ""
	if claims != nil {
		userID = fmt.Sprintf("%d", claims.UserID)
	}

	if globalMode {
		s.mu.RLock()
		globalConns := len(s.conns["*"])
		s.mu.RUnlock()
		if globalConns >= 3 {
			http.Error(w, "too many global monitor connections", http.StatusTooManyRequests)
			return
		}
	} else {
		s.mu.RLock()
		existing := len(s.conns[sessionID])
		s.mu.RUnlock()
		if existing >= maxSessionConns {
			http.Error(w, "too many connections for this session", http.StatusTooManyRequests)
			return
		}
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("ws upgrade failed", "error", err)
		return
	}

	lastEventID := strings.TrimSpace(r.URL.Query().Get("last_event_id"))
	logEnabled := r.URL.Query().Get("log_enabled") == "1" || r.URL.Query().Get("log_enabled") == "true"
	filterKey := strings.TrimSpace(r.URL.Query().Get("filter_key"))

	channels := map[string]bool{
		"chat":   true,
		"system": true,
	}
	if globalMode {
		channels["monitor"] = true
		channels["team"] = true
		channels["graph"] = true
	}

	wc := &wsConn{
		conn:       conn,
		sessionID:  sessionID,
		userID:     userID,
		channels:   channels,
		filterKey:  filterKey,
		send:       make(chan []byte, 128),
		logEnabled: logEnabled,
		globalMode: globalMode,
	}

	subOpts := event.SubscribeOptions{
		BufferSize: 256,
	}
	if !globalMode {
		subOpts.SessionID = sessionID
	}
	eventCh, unsub := s.eventBus.Subscribe(subOpts)
	wc.unsubscribe = unsub

	s.mu.Lock()
	s.conns[sessionID] = append(s.conns[sessionID], wc)
	s.mu.Unlock()

	s.sendConnected(wc, sessionID, lastEventID)

	go s.writePump(wc)
	go s.readPump(wc, eventCh)

	if !globalMode && lastEventID != "" && s.eventBuffer != nil {
		wc.replayDone = make(chan struct{})
		go func() {
			defer close(wc.replayDone)
			s.replayEvents(wc, sessionID, lastEventID)
		}()
	}

	go s.eventPump(wc, eventCh)
}

func (s *WSServer) sendConnected(wc *wsConn, sessionID, lastEventID string) {
	subscribed := make([]string, 0, len(wc.channels))
	for ch := range wc.channels {
		subscribed = append(subscribed, ch)
	}
	msg := wsDownstream{
		Direction: "server_to_client",
		Channel:   "system",
		Type:      "connected",
		Payload: map[string]any{
			"session_id":          sessionID,
			"server_time":         time.Now().UTC().Format(time.RFC3339Nano),
			"subscribed_channels": subscribed,
			"last_event_id":       lastEventID,
		},
	}
	data, _ := json.Marshal(msg)
	select {
	case wc.send <- data:
	default:
	}
}

func (s *WSServer) replayEvents(wc *wsConn, sessionID, lastEventID string) {
	events := s.eventBuffer.Replay(sessionID, lastEventID)

	startMsg := wsDownstream{
		Direction: "server_to_client",
		Channel:   wc.firstSubscribedChannel(),
		Type:      "replay_start",
		Payload: map[string]any{
			"session_id":    sessionID,
			"last_event_id": lastEventID,
			"count":         len(events),
		},
	}
	if data, err := json.Marshal(startMsg); err == nil {
		select {
		case wc.send <- data:
		default:
			return
		}
	}

	for _, env := range events {
		if !wc.channels[env.Channel] {
			continue
		}
		msg := wsDownstream{
			Direction: "server_to_client",
			Channel:   env.Channel,
			Type:      "replay",
			Envelope:  &env,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		select {
		case wc.send <- data:
		default:
			return
		}
	}

	endMsg := wsDownstream{
		Direction: "server_to_client",
		Channel:   wc.firstSubscribedChannel(),
		Type:      "replay_end",
		Payload: map[string]any{
			"session_id": sessionID,
		},
	}
	if data, err := json.Marshal(endMsg); err == nil {
		select {
		case wc.send <- data:
		default:
		}
	}
}

func (wc *wsConn) firstSubscribedChannel() string {
	if wc.channels["chat"] {
		return "chat"
	}
	for ch := range wc.channels {
		return ch
	}
	return "system"
}

func (s *WSServer) writePump(wc *wsConn) {
	ticker := time.NewTicker(defaultWSPingPeriod)
	defer func() {
		ticker.Stop()
		wc.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-wc.send:
			wc.conn.SetWriteDeadline(time.Now().Add(defaultWSWriteWait))
			if !ok {
				wc.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := wc.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			wc.conn.SetWriteDeadline(time.Now().Add(defaultWSWriteWait))
			if err := wc.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *WSServer) readPump(wc *wsConn, eventCh <-chan event.Envelope) {
	defer func() {
		s.removeConn(wc)
		wc.unsubscribe()
		wc.conn.Close()
	}()
	wc.conn.SetReadLimit(defaultWSReadLimit)
	wc.conn.SetReadDeadline(time.Now().Add(defaultWSPongWait))
	wc.conn.SetPongHandler(func(string) error {
		wc.conn.SetReadDeadline(time.Now().Add(defaultWSPongWait))
		return nil
	})
	for {
		_, message, err := wc.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Debug("ws read error", "error", err, "session_id", wc.sessionID)
			}
			return
		}
		s.handleUpstream(wc, message)
	}
}

func (s *WSServer) eventPump(wc *wsConn, eventCh <-chan event.Envelope) {
	if wc.replayDone != nil {
		<-wc.replayDone
	}
	for env := range eventCh {
		if !wc.channels[env.Channel] {
			continue
		}
		if env.Type == event.EnvelopeTypeLog && !wc.logEnabled {
			continue
		}
		if wc.filterKey != "" && !event.MatchFilterKey(wc.filterKey, env.FilterKey) {
			continue
		}
		msg := wsDownstream{
			Direction: "server_to_client",
			Channel:   env.Channel,
			Envelope:  &env,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		select {
		case wc.send <- data:
		default:
			slog.Warn("ws send buffer full, dropping event", "session_id", wc.sessionID, "type", env.Type)
		}
	}
}

func (s *WSServer) handleUpstream(wc *wsConn, raw []byte) {
	var up wsUpstream
	if err := json.Unmarshal(raw, &up); err != nil {
		slog.Warn("ws upstream parse error", "error", err)
		return
	}
	if up.Direction != "client_to_server" {
		return
	}
	switch up.Type {
	case "ping":
		msg := wsDownstream{
			Direction: "server_to_client",
			Channel:   "system",
			Type:      "pong",
			Payload: map[string]any{
				"server_time": time.Now().UTC().Format(time.RFC3339Nano),
			},
		}
		data, _ := json.Marshal(msg)
		select {
		case wc.send <- data:
		default:
		}

	case "subscribe":
		payload, ok := up.Payload.(map[string]any)
		if !ok {
			return
		}
		ch, _ := payload["channel"].(string)
		if ch != "" {
			wc.channels[ch] = true
		}
		if fk, _ := payload["filter_key"].(string); fk != "" {
			wc.filterKey = fk
		}

	case "unsubscribe":
		payload, ok := up.Payload.(map[string]any)
		if !ok {
			return
		}
		ch, _ := payload["channel"].(string)
		if ch != "" && ch != "chat" && ch != "system" {
			delete(wc.channels, ch)
		}

	case "cancel":
		if s.canceller != nil {
			s.canceller.CancelRun(context.Background(), wc.sessionID)
		}
		env := event.NewEnvelope(event.EnvelopeTypeError, "user", wc.sessionID)
		env.Error = &event.EnvelopeError{
			Type:    "cancelled",
			Message: "user cancelled",
		}
		s.eventBus.Publish(context.Background(), env)

	case "enable_log":
		payload, ok := up.Payload.(map[string]any)
		if !ok {
			return
		}
		if enabled, _ := payload["enabled"].(bool); enabled {
			wc.logEnabled = true
			wc.channels["monitor"] = true
		} else {
			wc.logEnabled = false
			delete(wc.channels, "monitor")
		}

	case "user_message":
		s.handleUserMessage(wc, up)

	case "enqueue_message":
		s.handleEnqueueMessage(wc, up)
	}
}

func (s *WSServer) handleUserMessage(wc *wsConn, up wsUpstream) {
	payload, ok := up.Payload.(map[string]any)
	if !ok {
		return
	}
	content, _ := payload["content"].(string)
	if strings.TrimSpace(content) == "" {
		return
	}
	req := &chatv1.SendChatMessageRequest{
		SessionId: wc.sessionID,
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

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
		defer cancel()
		_, err := s.sender.SendChatMessage(ctx, req)
		if err != nil {
			slog.Warn("ws user_message send failed", "error", err, "session_id", wc.sessionID)
			env := event.NewEnvelope(event.EnvelopeTypeError, "ws-handler", wc.sessionID)
			env.Error = &event.EnvelopeError{
				Type:    "send_failed",
				Message: err.Error(),
			}
			s.eventBus.Publish(context.Background(), env)
		}
	}()
}

func (s *WSServer) handleEnqueueMessage(wc *wsConn, up wsUpstream) {
	payload, ok := up.Payload.(map[string]any)
	if !ok {
		return
	}
	content, _ := payload["content"].(string)
	if strings.TrimSpace(content) == "" {
		return
	}

	go func() {
		req := &chatv1.SendChatMessageRequest{
			SessionId: wc.sessionID,
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
		ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
		defer cancel()
		_, err := s.sender.SendChatMessage(ctx, req)
		if err != nil {
			slog.Warn("ws enqueue_message send failed", "error", err, "session_id", wc.sessionID)
			env := event.NewEnvelope(event.EnvelopeTypeError, "ws-handler", wc.sessionID)
			env.Error = &event.EnvelopeError{
				Type:    "enqueue_failed",
				Message: err.Error(),
			}
			s.eventBus.Publish(context.Background(), env)
		}
	}()
}

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
	return result
}

func (s *WSServer) removeConn(wc *wsConn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conns := s.conns[wc.sessionID]
	for i, c := range conns {
		if c == wc {
			s.conns[wc.sessionID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}
	if len(s.conns[wc.sessionID]) == 0 {
		delete(s.conns, wc.sessionID)
	}
}
