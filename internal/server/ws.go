package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/transport"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/websocket"
)

var _ transport.Server = (*WSServer)(nil)

const (
	defaultWSReadLimit           = 1 << 20
	defaultWSPongWait            = 60 * time.Second
	defaultWSPingPeriod          = 30 * time.Second
	defaultWSWriteWait           = 10 * time.Second
	defaultWSTurnTimeout         = 5 * time.Minute
	defaultWSSystemSendWait      = 3 * time.Second
	defaultMaxSessionConns       = 5
	defaultMaxGlobalMonitorConns = 3
)

type RunCanceller interface {
	CancelRun(ctx context.Context, sessionID string) bool
}

type ChatSender interface {
	SendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error)
	EnqueueUserMessage(ctx context.Context, req *chatv1.EnqueueUserMessageRequest) (*chatv1.EnqueueUserMessageResponse, error)
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
	// send is the legacy/system channel (normal priority). eventPump now routes
	// through queues; non-event callers (sendSystemDownstream, replay) still use send.
	// MON-OPT-04: send is kept for backward compat; writePump drains queues first.
	send       chan []byte
	queues     *connQueues // MON-OPT-04 priority lanes
	replayDone chan struct{}
	logEnabled bool
	globalMode bool
	probeMode  bool
	// connCtx is cancelled when this WebSocket connection closes so that
	// in-flight turns started by this connection are also cancelled (COR-03).
	connCtx    context.Context
	connCancel context.CancelFunc
}

func wsProbeMode(r *http.Request) bool {
	q := r.URL.Query()
	v := strings.TrimSpace(q.Get("probe"))
	if v == "1" || strings.EqualFold(v, "true") {
		return true
	}
	return strings.TrimSpace(q.Get("health")) == "1"
}

func (wc *wsConn) sendSystemDownstream(msg wsDownstream) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	// H-02: route through queues (normal priority) so writePump has a single drain path.
	wc.queues.enqueueSystem(data)
	wc.wakeWriter()
}

func (wc *wsConn) contextOrBackground() context.Context {
	if wc != nil && wc.connCtx != nil {
		return wc.connCtx
	}
	return context.Background()
}

func (s *WSServer) countGlobalMonitorConns() int {
	conns := s.conns["*"]
	n := 0
	for _, wc := range conns {
		if wc != nil && !wc.probeMode {
			n++
		}
	}
	return n
}

type WSServer struct {
	mu                    sync.RWMutex
	conns                 map[string][]*wsConn
	eventBus              event.Bus
	monitorBus            event.Bus
	eventBuffer           *event.Buffer
	canceller             RunCanceller
	sender                ChatSender
	turnGateway           biz.TurnExecutorGateway
	serverConf            *conf.Server
	upgrader              websocket.Upgrader
	closed                bool
	maxSessionConns       int
	maxGlobalMonitorConns int
	lg                    loggateway.Logger
}

// NewWSServer wires a single bus (tests / legacy).
func NewWSServer(c *conf.Server, eventBus event.Bus, eventBuffer *event.Buffer, canceller RunCanceller, sender ChatSender) *WSServer {
	return NewWSServerFromInfra(c, &event.Infra{
		SessionBus: eventBus,
		MonitorBus: eventBus,
		Buffer:     eventBuffer,
	}, canceller, sender, nil, nil)
}

// NewWSServerFromInfra uses session bus for chat envelopes and monitor bus for flow_log (P0).
func NewWSServerFromInfra(c *conf.Server, infra *event.Infra, canceller RunCanceller, sender ChatSender, turnGateway biz.TurnExecutorGateway, lg loggateway.Logger) *WSServer {
	if c == nil || c.GetWs() == nil || !c.GetWs().GetEnable() {
		return nil
	}
	if infra == nil {
		infra = event.NewInfra()
	}
	monitor := infra.MonitorBus
	if monitor == nil {
		monitor = infra.SessionBus
	}
	return &WSServer{
		conns:                 make(map[string][]*wsConn),
		eventBus:              infra.SessionBus,
		monitorBus:            monitor,
		eventBuffer:           infra.Buffer,
		canceller:             canceller,
		sender:                sender,
		turnGateway:           turnGateway,
		serverConf:            c,
		maxSessionConns:       envInt("WS_MAX_SESSION_CONNS", defaultMaxSessionConns),
		maxGlobalMonitorConns: envInt("WS_MAX_GLOBAL_MONITOR_CONNS", defaultMaxGlobalMonitorConns),
		lg:                    lg,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				o := strings.TrimSpace(r.Header.Get("Origin"))
				if o == "" {
					return true
				}
				return OriginAllowed(o)
			},
		},
	}
}

func envInt(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func (s *WSServer) Start(ctx context.Context) error {
	return nil
}

func (s *WSServer) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.closed = true
	s.broadcastShutdown()
	return nil
}

func (s *WSServer) RegisterOnKratos(srv *kratoshttp.Server) {
	if s == nil || srv == nil {
		return
	}
	srv.HandleFunc("/v1/ws", s.handleWS)
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
			wc.sendSystemRaw(data)
		}
	}
}

func (wc *wsConn) sendSystemRaw(data []byte) {
	// H-02: route through queues (normal priority).
	wc.queues.enqueueSystem(data)
	wc.wakeWriter()
}

func (wc *wsConn) wakeWriter() {
	if wc == nil || wc.send == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	select {
	case wc.send <- nil:
	default:
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
	probeMode := globalMode && wsProbeMode(r)

	var claims *auth.Auth
	if auth.HTTPAuthBypassEnabled() {
		claims = auth.DevBypassPrincipal()
	} else {
		tokenStr := auth.TokenFromHTTPRequest(r)
		if tokenStr == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var err error
		claims, err = auth.ParseTokenFromRequest(tokenStr)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
	}

	userID := ""
	if claims != nil {
		userID = fmt.Sprintf("%d", claims.UserID)
	}

	if globalMode && !probeMode {
		s.mu.RLock()
		globalConns := s.countGlobalMonitorConns()
		s.mu.RUnlock()
		if globalConns >= s.maxGlobalMonitorConns {
			http.Error(w, "too many global monitor connections", http.StatusTooManyRequests)
			return
		}
	} else if !globalMode {
		s.mu.RLock()
		existing := len(s.conns[sessionID])
		s.mu.RUnlock()
		if existing >= s.maxSessionConns {
			http.Error(w, "too many connections for this session", http.StatusTooManyRequests)
			return
		}
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.lg.Warn("WebSocket 升级失败", loggateway.StepID("system.ws.upgrade_failed"), loggateway.Err(err))
		return
	}

	lastEventID := strings.TrimSpace(r.URL.Query().Get("last_event_id"))
	logEnabled := r.URL.Query().Get("log_enabled") == "1" || r.URL.Query().Get("log_enabled") == "true"
	if globalMode && !probeMode && !logEnabled && s.serverConf != nil && s.serverConf.ProcessLogEnabled() {
		logEnabled = true
	}
	filterKey := strings.TrimSpace(r.URL.Query().Get("filter_key"))

	channels := map[string]bool{
		"chat":   true,
		"system": true,
	}
	if globalMode && !probeMode {
		channels["monitor"] = true
		channels["team"] = true
		channels["graph"] = true
		channels["knowledge"] = true
	}

	wcCtx, wcCancel := context.WithCancel(context.Background())
	wc := &wsConn{
		conn:       conn,
		sessionID:  sessionID,
		userID:     userID,
		channels:   channels,
		filterKey:  filterKey,
		send:       make(chan []byte, wsNormalCap), // system/replay messages (normal priority)
		queues:     newConnQueues(),                // MON-OPT-04 priority lanes for bus events
		logEnabled: logEnabled,
		globalMode: globalMode,
		probeMode:  probeMode,
		connCtx:    wcCtx,
		connCancel: wcCancel,
	}

	var eventCh <-chan event.Envelope
	var monitorCh <-chan event.Envelope
	wc.unsubscribe = func() {}
	if !probeMode {
		subOpts := event.SubscribeOptions{
			BufferSize: 256,
			Reliable:   !globalMode,
		}
		if !globalMode {
			subOpts.SessionID = sessionID
		}
		ch, unsub := s.eventBus.Subscribe(subOpts)
		eventCh = ch
		unsubSession := unsub
		unsubAll := func() { unsubSession() }
		if s.monitorBus != nil && s.monitorBus != s.eventBus {
			monOpts := event.SubscribeOptions{
				BufferSize: 128,
				DropPolicy: event.DropNewest,
			}
			if !globalMode {
				monOpts.SessionID = sessionID
			}
			mCh, mUnsub := s.monitorBus.Subscribe(monOpts)
			monitorCh = mCh
			prev := unsubAll
			unsubAll = func() {
				prev()
				mUnsub()
			}
		}
		wc.unsubscribe = unsubAll
	}

	s.mu.Lock()
	s.conns[sessionID] = append(s.conns[sessionID], wc)
	s.mu.Unlock()

	s.sendConnected(wc, sessionID, lastEventID)

	connCtx := r.Context()
	safego.Go(connCtx, "ws-write-pump", func() { s.writePump(wc) })
	safego.Go(connCtx, "ws-read-pump", func() { s.readPump(wc, eventCh) })

	if !probeMode && !globalMode && lastEventID != "" && s.eventBuffer != nil {
		wc.replayDone = make(chan struct{})
		safego.Go(connCtx, "ws-replay", func() {
			defer close(wc.replayDone)
			s.replayEvents(wc, sessionID, lastEventID)
		})
	}

	if !probeMode {
		safego.Go(connCtx, "ws-event-pump", func() { s.eventPump(wc, eventCh) })
		if monitorCh != nil {
			safego.Go(connCtx, "ws-monitor-pump", func() { s.eventPump(wc, monitorCh) })
		}
	}
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
	wc.sendSystemDownstream(msg)
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
	// H-02: all replay messages route through queues (normal priority).
	if data, err := json.Marshal(startMsg); err == nil {
		wc.queues.enqueueSystem(data)
		wc.wakeWriter()
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
		wc.queues.enqueueSystem(data)
		wc.wakeWriter()
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
		wc.queues.enqueueSystem(data)
		wc.wakeWriter()
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
	bpTicker := time.NewTicker(wsBackpressureInterval) // MON-OPT-04 backpressure reporter
	defer func() {
		ticker.Stop()
		bpTicker.Stop()
		wc.conn.Close()
	}()
	for {
		// H-02 + MON-OPT-04: drain priority queues (high → normal → low) before blocking.
		// wc.queues is the sole write path; wc.send is only used for the close signal.
		// M-02: drainSelect drains high/normal greedily and low at most wsLowDrainPerLoop
		// times per outer iteration to prevent high/normal starvation.
		for {
			data, prio, ok := wc.queues.drainSelect()
			if !ok {
				break
			}
			wc.conn.SetWriteDeadline(time.Now().Add(defaultWSWriteWait))
			if err := wc.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
			// After draining a low-priority message, cap further low drains this
			// loop iteration to allow high/normal items to be checked again.
			if prio == wsPriorityLow {
				lowCount := 1
				for lowCount < wsLowDrainPerLoop {
					d, p, o := wc.queues.drainSelect()
					if !o {
						goto endDrain
					}
					wc.conn.SetWriteDeadline(time.Now().Add(defaultWSWriteWait))
					if err := wc.conn.WriteMessage(websocket.TextMessage, d); err != nil {
						return
					}
					if p == wsPriorityLow {
						lowCount++
					} else {
						// Non-low message found — continue outer loop for high/normal priority.
						break
					}
				}
				break // yield to outer select (ping/bp tickers) after low batch
			}
		}
	endDrain:

		select {
		case _, ok := <-wc.send:
			// wc.send closed = eventPump signalled a high-queue timeout → close.
			if !ok {
				wc.conn.SetWriteDeadline(time.Now().Add(defaultWSWriteWait))
				wc.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
		case <-ticker.C:
			wc.conn.SetWriteDeadline(time.Now().Add(defaultWSWriteWait))
			if err := wc.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-bpTicker.C:
			// MON-OPT-04: inject backpressure notification if drops occurred.
			if bp := wc.queues.backpressurePayload(int(wsBackpressureInterval.Seconds())); bp != nil {
				wc.conn.SetWriteDeadline(time.Now().Add(defaultWSWriteWait))
				_ = wc.conn.WriteMessage(websocket.TextMessage, bp)
			}
		}
	}
}

func (s *WSServer) readPump(wc *wsConn, eventCh <-chan event.Envelope) {
	defer func() {
		// COR-03: cancel connection context so in-flight turns started by this
		// connection are cancelled when the client disconnects.
		wc.connCancel()
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
				s.lg.With(loggateway.SessionID(wc.sessionID)).Warn("WebSocket 读错误", loggateway.StepID("system.ws.read_error"), loggateway.Err(err))
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
		// flow_log is always delivered on monitor channel (no enable_log gate).
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
		// MON-OPT-04: route to priority queue; close connection on high-queue timeout.
		prio := wsEnvelopePriority(env.Type)
		if ok := wc.queues.enqueue(prio, data); !ok {
			s.lg.With(loggateway.SessionID(wc.sessionID)).Warn("WebSocket 高优先级队列超时，关闭连接",
				loggateway.StepID("system.ws.high_queue_timeout"), loggateway.Any("type", env.Type))
			close(wc.send) // signal writePump to exit
			return
		}
		wc.wakeWriter()
	}
}

func (s *WSServer) handleUpstream(wc *wsConn, raw []byte) {
	var up wsUpstream
	if err := json.Unmarshal(raw, &up); err != nil {
		s.lg.Warn("WebSocket 上行消息解析失败", loggateway.StepID("system.ws.parse_error"), loggateway.Err(err))
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
			wc.logEnabled = true
			wc.channels["monitor"] = true
		} else {
			wc.logEnabled = false
			if !wc.globalMode {
				delete(wc.channels, "monitor")
			}
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

	sessionID := wc.sessionID
	requestID := strings.TrimSpace(up.RequestID)

	// Prefer biz.TurnExecutorGateway when available (Phase B2: unified turn entry point).
	if s.turnGateway != nil {
		input := biz.TurnInput{
			SessionID: sessionID,
			Content:   strings.TrimSpace(content),
			EntryConfig: biz.TurnEntryPointConfig{
				EntryPoint:  biz.EntryPointWS,
				AllowQueue:  true,
				AllowStream: true,
			},
		}
		if agentKey, _ := payload["agent_key"].(string); agentKey != "" {
			input.AgentKey = agentKey
		}
		if teamID, _ := payload["team_id"].(string); teamID != "" {
			input.TeamID = teamID
		}
		if opts, ok := payload["options"].(map[string]any); ok {
			input.Options = buildBizTurnOptions(opts)
		}
		// COR-03: derive turn context from the connection context so disconnecting
		// the WebSocket also cancels in-flight agent turns for this connection.
		connCtx := wc.contextOrBackground()
		safego.Go(context.Background(), "ws-user-message", func() {
			ctx, cancel := context.WithTimeout(connCtx, defaultWSTurnTimeout)
			defer cancel()
			_, err := s.turnGateway.ExecuteTurn(ctx, input)
			if err != nil {
				s.lg.With(loggateway.SessionID(sessionID)).Warn("WebSocket 用户消息发送失败", loggateway.StepID("system.ws.send_failed"), loggateway.Err(err))
				// ExecuteTurn publishes user-facing failures through the turn projector.
				// Avoid a second raw ws-handler error that can leak provider details.
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

	// COR-03: derive turn context from the connection context so disconnecting
	// the WebSocket also cancels in-flight agent turns for this connection.
	connCtx := wc.contextOrBackground()
	safego.Go(context.Background(), "ws-user-message", func() {
		ctx, cancel := context.WithTimeout(connCtx, defaultWSTurnTimeout)
		defer cancel()
		_, err := s.sender.SendChatMessage(ctx, req)
		if err != nil {
			s.lg.With(loggateway.SessionID(sessionID)).Warn("WebSocket 用户消息发送失败", loggateway.StepID("system.ws.send_failed"), loggateway.Err(err))
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

func (s *WSServer) handleEnqueueMessage(wc *wsConn, up wsUpstream) {
	if s == nil || s.sender == nil {
		return
	}
	payload, ok := up.Payload.(map[string]any)
	if !ok {
		return
	}
	content, _ := payload["content"].(string)
	if strings.TrimSpace(content) == "" {
		return
	}

	sessionID := wc.sessionID
	requestID := strings.TrimSpace(up.RequestID)
	req := &chatv1.EnqueueUserMessageRequest{
		SessionId: sessionID,
		Content:   strings.TrimSpace(content),
	}

	connCtxEq := wc.contextOrBackground()
	safego.Go(context.Background(), "ws-enqueue-message", func() {
		ctx, cancel := context.WithTimeout(connCtxEq, defaultWSTurnTimeout)
		defer cancel()
		resp, err := s.sender.EnqueueUserMessage(ctx, req)
		if err != nil {
			s.lg.With(loggateway.SessionID(sessionID)).Warn("WebSocket 入队消息发送失败", loggateway.StepID("system.ws.send_failed"), loggateway.Err(err))
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

// buildBizTurnOptions builds a biz.TurnOptions from WS payload options.
// Used by the TurnGateway path (Phase B2).
func buildBizTurnOptions(opts map[string]any) biz.TurnOptions {
	result := biz.TurnOptions{}
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
