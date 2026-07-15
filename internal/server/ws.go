package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/transport"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/websocket"
)

type WSTurnInput struct {
	SessionID   string
	Content     string
	AgentKey    string
	TeamID      string
	Options     WSTurnOptions
	AllowQueue  bool
	AllowStream bool
}

type WSTurnOptions struct {
	DialogMode     string
	Provider       string
	Model          string
	AttachmentIDs  []string
	KnowledgeBases []string
}

type WSTurnExecutor interface {
	ExecuteTurn(ctx context.Context, input WSTurnInput) error
}

var _ transport.Server = (*WSServer)(nil)

type RunCanceller interface {
	CancelRun(ctx context.Context, sessionID string) bool
}

type ChatSender interface {
	SendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error)
	EnqueueUserMessage(ctx context.Context, req *chatv1.EnqueueUserMessageRequest) (*chatv1.EnqueueUserMessageResponse, error)
}

// SessionAuthorizer checks whether a user is allowed to subscribe to a
// specific session's WebSocket events. Used by the WS handler to prevent
// users from subscribing to other users' sessions (IDOR protection).
//
// Implementations should return nil if the user owns the session (or is
// otherwise authorized), and a non-nil error otherwise.
type SessionAuthorizer interface {
	CheckOwnership(ctx context.Context, sessionID, userID string) error
}

// WSServer is the WebSocket server coordinator. It delegates to focused sub-modules:
//   - ws_conn.go         — wsConn struct + connStore (connection lifecycle)
//   - ws_conn_manager.go — connection limit checks and removal
//   - ws_codec.go        — protocol types (wsUpstream/wsDownstream)
//   - ws_message_handler.go — upstream message dispatch and business logic
//   - ws_io_pump.go      — read/write/event pump goroutines
//   - ws_event.go        — event subscription and connected handshake
//   - ws_priority.go     — three-priority send queue with backpressure
type WSServer struct {
	store                 *connStore
	monitorBus            contract.MonitorBus
	// eventBus is the v2 typed EventBus (Phase 3b-D). Used by publishWSErrorActivity
	// to emit ActivityBridgeEvent payloads (wrapping v1 ActivityEvent) for the
	// chat error activity. The v2 WSV2Subscriber fans these out to WS clients.
	eventBus              biz.EventBus
	// outbox provides durable critical-event replay for last_event_id (B-06). Optional.
	outbox                biz.EventDeliveryOutboxRepo
	canceller             RunCanceller
	sender                ChatSender
	turnExecutor          WSTurnExecutor
	sessionAuth           SessionAuthorizer
	serverConf            *conf.Server
	runtimeConf           *conf.Runtime
	upgrader              websocket.Upgrader
	closed                bool
	maxSessionConns       int
	maxGlobalMonitorConns int
	lg                    loggateway.Logger
}

// SetEventOutbox wires the durable critical-event outbox used for last_event_id replay.
func (s *WSServer) SetEventOutbox(repo biz.EventDeliveryOutboxRepo) {
	if s == nil {
		return
	}
	s.outbox = repo
}

// NewWSServerFromInfra uses monitor bus for monitor events and the v2 EventBus
// for chat/system events (AF pipeline via WSV2Subscriber).
func NewWSServerFromInfra(c *conf.Server, infra *event.Infra, canceller RunCanceller, sender ChatSender, turnExecutor WSTurnExecutor, runtimeConf *conf.Runtime, lg loggateway.Logger, eventBus biz.EventBus, sessionAuth SessionAuthorizer) *WSServer {
	if c == nil || c.GetWs() == nil || !c.GetWs().GetEnable() {
		return nil
	}
	if infra == nil {
		infra = event.NewInfra(lg)
	}
	monitor := infra.MonitorEventBus
	wsCfg := runtimeConf.WSConfig()
	return &WSServer{
		store:                 newConnStore(),
		monitorBus:            monitor,
		eventBus:              eventBus,
		canceller:             canceller,
		sender:                sender,
		turnExecutor:          turnExecutor,
		sessionAuth:           sessionAuth,
		serverConf:            c,
		runtimeConf:           runtimeConf,
		maxSessionConns:       envInt("WS_MAX_SESSION_CONNS", int(wsCfg.MaxSessionConns)),
		maxGlobalMonitorConns: envInt("WS_MAX_GLOBAL_MONITOR_CONNS", int(wsCfg.MaxGlobalMonitorConns)),
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

// wsConfig returns the resolved WebSocket config from Runtime.
func (s *WSServer) wsConfig() conf.RuntimeWSConfig {
	return s.runtimeConf.WSConfig()
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
	// Exemption from red-line #12 (no business routes in Server layer):
	// WebSocket upgrade cannot be defined via proto; HandleFunc is the only way.
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
	s.store.forEachConn(func(wc *wsConn) {
		wc.sendSystemRaw(data)
	})
}

// handleWS is the main WebSocket HTTP handler. It orchestrates authentication,
// connection limit checks, WebSocket upgrade, and goroutine startup.
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

	// Authentication
	claims, err := wsAuthenticate(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	userID := fmt.Sprintf("%d", claims.UserID)

	// Global mode (session_id=*) requires admin access — prevents any
	// authenticated user from subscribing to all sessions' events.
	if globalMode && !claims.HasAdminAccess() {
		http.Error(w, "admin access required for global subscription", http.StatusForbidden)
		return
	}

	// Non-global mode: verify session ownership — prevents users from
	// subscribing to other users' sessions (IDOR protection). Admins
	// bypass this check. Skipped when no SessionAuthorizer is wired
	// (e.g., in unit tests without a session repo).
	if !globalMode && !claims.HasAdminAccess() && s.sessionAuth != nil {
		if err := s.sessionAuth.CheckOwnership(r.Context(), sessionID, userID); err != nil {
			s.lg.Warn("WS session ownership denied",
				loggateway.StepID("ws.ownership_denied"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("user_id", userID),
				loggateway.Err(err),
			)
			http.Error(w, "session ownership required", http.StatusForbidden)
			return
		}
	}

	// Connection limit check
	if code, msg := s.canAcceptConnection(sessionID, globalMode, probeMode); code != 0 {
		http.Error(w, msg, code)
		return
	}

	// WebSocket upgrade
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.lg.Warn("WebSocket 升级失败", loggateway.StepID("ws.upgrade_failed"), loggateway.Err(err))
		return
	}

	// Build connection object
	wc := s.newWSConn(conn, sessionID, userID, globalMode, probeMode, r)

	// Event subscription
	var monitorCh <-chan contract.MonitorEvent
	if !probeMode {
		monitorCh = s.setupEventSubscription(wc, globalMode)
	}

	// Register connection
	s.store.add(sessionID, wc)

	// B-06: replay missed critical outbox frames before "connected" so the
	// client applies them prior to treating the stream as caught up.
	lastEventID := strings.TrimSpace(r.URL.Query().Get("last_event_id"))
	if !globalMode && !probeMode && lastEventID != "" {
		s.replayOutbox(wc, sessionID, lastEventID)
	}

	// Send connected message
	s.sendConnected(wc, sessionID, lastEventID)

	// Start goroutines
	connCtx := r.Context()
	safego.Go(connCtx, "ws-write-pump", func() { s.writePump(wc) })
	safego.Go(connCtx, "ws-read-pump", func() { s.readPump(wc) })

	if !probeMode {
		if monitorCh != nil {
			safego.Go(connCtx, "ws-monitor-pump", func() { s.monitorEventPump(wc, monitorCh) })
		}
	}
}

// wsAuthenticate validates the request and returns the authenticated claims.
func wsAuthenticate(r *http.Request) (*auth.Auth, error) {
	if auth.HTTPAuthBypassEnabled() {
		claims := auth.DevBypassPrincipal()
		return claims, nil
	}
	tokenStr := auth.TokenFromHTTPRequest(r)
	if tokenStr == "" {
		return nil, errors.New("unauthorized")
	}
	claims, err := auth.ParseTokenFromRequest(tokenStr)
	if err != nil {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// newWSConn creates a wsConn from the upgraded WebSocket connection and request parameters.
func (s *WSServer) newWSConn(conn *websocket.Conn, sessionID, userID string, globalMode, probeMode bool, r *http.Request) *wsConn {
	logEnabled := r.URL.Query().Get("log_enabled") == "1" || r.URL.Query().Get("log_enabled") == "true"
	if globalMode && !probeMode && !logEnabled && s.serverConf != nil && s.serverConf.ProcessLogEnabled() {
		logEnabled = true
	}
	filterKey := strings.TrimSpace(r.URL.Query().Get("filter_key"))

	wcCtx, wcCancel := context.WithCancel(context.Background())
	cfg := s.wsConfig()
	return &wsConn{
		conn:       conn,
		sessionID:  sessionID,
		userID:     userID,
		channels:   wsBuildChannels(globalMode, probeMode),
		filterKey:  filterKey,
		send:       make(chan []byte, cfg.NormalCap),
		queues:     newConnQueues(cfg),
		logEnabled: logEnabled,
		globalMode: globalMode,
		probeMode:  probeMode,
		connCtx:    wcCtx,
		connCancel: wcCancel,
	}
}
