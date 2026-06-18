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

// WSServer is the WebSocket server coordinator. It delegates to focused sub-modules:
//   - ws_conn.go         — wsConn struct + connStore (connection lifecycle)
//   - ws_conn_manager.go — connection limit checks and removal
//   - ws_codec.go        — protocol types (wsUpstream/wsDownstream)
//   - ws_message_handler.go — upstream message dispatch and business logic
//   - ws_io_pump.go      — read/write/event pump goroutines
//   - ws_event.go        — event subscription, replay, and connected handshake
//   - ws_priority.go     — three-priority send queue with backpressure
type WSServer struct {
	store                 *connStore
	eventBus              event.Bus
	monitorBus            event.Bus
	eventBuffer           *event.Buffer
	crossProcessStore     contract.CrossProcessStore // optional (P1-6): Postgres replay fallback
	canceller             RunCanceller
	sender                ChatSender
	turnExecutor          WSTurnExecutor
	serverConf            *conf.Server
	runtimeConf           *conf.Runtime
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
	}, canceller, sender, nil, nil, nil)
}

// NewWSServerFromInfra uses session bus for chat envelopes and monitor bus for flow_log (P0).
func NewWSServerFromInfra(c *conf.Server, infra *event.Infra, canceller RunCanceller, sender ChatSender, turnExecutor WSTurnExecutor, runtimeConf *conf.Runtime, lg loggateway.Logger) *WSServer {
	if c == nil || c.GetWs() == nil || !c.GetWs().GetEnable() {
		return nil
	}
	if infra == nil {
		infra = event.NewInfra(lg, nil, nil)
	}
	monitor := infra.MonitorBus
	if monitor == nil {
		monitor = infra.SessionBus
	}
	wsCfg := runtimeConf.WSConfig()
	return &WSServer{
		store:                 newConnStore(),
		eventBus:              infra.SessionBus,
		monitorBus:            monitor,
		eventBuffer:           infra.Buffer,
		crossProcessStore:     infra.CrossProcessStore,
		canceller:             canceller,
		sender:                sender,
		turnExecutor:          turnExecutor,
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
	userID, err := wsAuthenticate(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
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
	var eventCh <-chan event.Envelope
	var monitorCh <-chan event.Envelope
	if !probeMode {
		eventCh, monitorCh = s.setupEventSubscription(wc, globalMode)
	}

	// Register connection
	s.store.add(sessionID, wc)

	// Send connected message
	lastEventID := strings.TrimSpace(r.URL.Query().Get("last_event_id"))
	s.sendConnected(wc, sessionID, lastEventID)

	// Start goroutines
	connCtx := r.Context()
	safego.Go(connCtx, "ws-write-pump", func() { s.writePump(wc) })
	safego.Go(connCtx, "ws-read-pump", func() { s.readPump(wc) })

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

// wsAuthenticate validates the request and returns the userID.
func wsAuthenticate(r *http.Request) (string, error) {
	if auth.HTTPAuthBypassEnabled() {
		claims := auth.DevBypassPrincipal()
		return fmt.Sprintf("%d", claims.UserID), nil
	}
	tokenStr := auth.TokenFromHTTPRequest(r)
	if tokenStr == "" {
		return "", errors.New("unauthorized")
	}
	claims, err := auth.ParseTokenFromRequest(tokenStr)
	if err != nil {
		return "", errors.New("invalid token")
	}
	return fmt.Sprintf("%d", claims.UserID), nil
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
