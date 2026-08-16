package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/voice"
	"aranea-agents/pkg/loggateway"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/websocket"
)

// VoiceStatusProbe 报告 ASR/TTS 当前配置是否可用（V2-T8 差距2：前端麦克风
// 置灰门控的数据源）。实现 = DB-first/env-fallback 配置读取 + Validate。
type VoiceStatusProbe func(ctx context.Context) (asrAvailable, ttsAvailable bool)

// VoiceWSServer 是 /v1/voice 语音网关（设计 §2）：鉴权/单会话单连接/帧路由。
// 音频帧与高频事件走独立端点，不污染 /v1/ws 事件总线通道。
type VoiceWSServer struct {
	upgrader        websocket.Upgrader
	sessionAuth     SessionAuthorizer
	executor        WSTurnExecutor
	canceller       RunCanceller
	confirmer       voice.ConfirmResolver      // V2-T5 语音确认拦截；nil 关闭
	archiver        voice.AudioArchiver        // V2-T6 语音留档；nil 关闭
	probe           VoiceStatusProbe           // V2-T8 麦克风置灰门控；nil = 保守报不可用
	prewarmer       voice.TurnPrewarmer        // C1 语音 Turn 构建预热；nil 关闭
	speculator      voice.IntentSpeculator     // C2 投机意图；nil 关闭
	delegation      *voice.DelegationRegistry  // M74 V9 委派登记表；nil 关闭委派分流
	delegationSteps voice.DelegationStepReader // M74 V9 委派终稿读取；nil 播简报
	newASR          voice.ASRProviderFactory
	newTTS          voice.TTSProviderFactory
	bus             biz.EventBus
	infra           *event.Infra
	lg              loggateway.Logger

	mu    sync.Mutex
	conns map[string]*voice.Session

	// allowNilSessionAuth, when true, permits sessionAuth == nil to skip
	// ownership checks (test-only). Default false = fail-closed.
	allowNilSessionAuth bool
}

func NewVoiceWSServer(
	sessionAuth SessionAuthorizer,
	executor WSTurnExecutor,
	canceller RunCanceller,
	newASR voice.ASRProviderFactory,
	newTTS voice.TTSProviderFactory,
	bus biz.EventBus,
	infra *event.Infra,
	lg loggateway.Logger,
	confirmer voice.ConfirmResolver,
	archiver voice.AudioArchiver,
	probe VoiceStatusProbe,
) *VoiceWSServer {
	return &VoiceWSServer{
		upgrader:    websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		sessionAuth: sessionAuth,
		executor:    executor,
		canceller:   canceller,
		confirmer:   confirmer,
		archiver:    archiver,
		probe:       probe,
		newASR:      newASR,
		newTTS:      newTTS,
		bus:         bus,
		infra:       infra,
		lg:          lg.With(loggateway.Domain("voice")),
		conns:       map[string]*voice.Session{},
	}
}

// SetAllowNilSessionAuth permits sessionAuth == nil to skip ownership checks.
// Test-only escape hatch; production must never call it.
func (s *VoiceWSServer) SetAllowNilSessionAuth(allow bool) {
	if s == nil {
		return
	}
	s.allowNilSessionAuth = allow
}

func (s *VoiceWSServer) RegisterOnKratos(srv *kratoshttp.Server) {
	if s == nil || srv == nil {
		return
	}
	// Exemption from red-line #12 (no business routes in Server layer):
	// WebSocket upgrade cannot be defined via proto; HandleFunc is the only way.
	srv.HandleFunc("/v1/voice", s.handleVoiceWS)
	// V2-T8：语音可用性探测（前端麦克风置灰门控）。任意登录用户可读——
	// 只暴露 ASR/TTS 是否配置完成的布尔位，不泄漏 endpoint/凭据等敏感信息。
	srv.HandleFunc("/v1/voice/status", s.handleVoiceStatus)
}

// SetTurnPrewarmer 注入语音 Turn 构建预热端口（C1，可选依赖 setter 模式，
// 与 WSServer.SetTaskResumer 一致）；nil 或未调用 = 关闭预热。
func (s *VoiceWSServer) SetTurnPrewarmer(p voice.TurnPrewarmer) {
	if s == nil {
		return
	}
	s.prewarmer = p
}

// SetIntentSpeculator 注入投机意图端口（C2，可选依赖 setter 模式，
// 与 SetTurnPrewarmer 一致）；nil 或未调用 = 关闭投机。
func (s *VoiceWSServer) SetIntentSpeculator(sp voice.IntentSpeculator) {
	if s == nil {
		return
	}
	s.speculator = sp
}

// SetDelegationRegistry 注入语音委派登记表 + 终稿读取端口（M74 V9，设计 74
// §15.4-C，可选依赖 setter 模式，与 SetTurnPrewarmer 一致）；nil 或未调用 =
// 关闭委派三路分流（eventLoop 仅处理本会话事件）。
func (s *VoiceWSServer) SetDelegationRegistry(reg *voice.DelegationRegistry, steps voice.DelegationStepReader) {
	if s == nil {
		return
	}
	s.delegation = reg
	s.delegationSteps = steps
}

// handleVoiceStatus 返回语音服务可用性（V2-T8 差距2）。probe 未接线时保守
// 报不可用（前端置灰），禁止默认可用导致用户点了麦克风才发现降级。
func (s *VoiceWSServer) handleVoiceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := wsAuthenticate(r); err != nil { // 与 /v1/ws 同一鉴权（JWT / dev bypass）
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	asrOK, ttsOK := false, false
	if s.probe != nil {
		asrOK, ttsOK = s.probe(r.Context())
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{
		"asr_available": asrOK,
		"tts_available": ttsOK,
	})
}

func (s *VoiceWSServer) handleVoiceWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	claims, err := wsAuthenticate(r) // 与 /v1/ws 同一鉴权（JWT / dev bypass）
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	userID := fmt.Sprintf("%d", claims.UserID)
	// 会话归属校验（IDOR 防护，admin 豁免）——与 handleWS 同语义，
	// nil SessionAuthorizer fail-closed（除非 SetAllowNilSessionAuth 显式允许）。
	if !claims.HasAdminAccess() {
		if s.sessionAuth == nil && !s.allowNilSessionAuth {
			s.lg.Warn("voice WS session ownership check unavailable (sessionAuth nil), rejecting connection",
				loggateway.StepID("voice.ws.ownership_unavailable"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("user_id", userID))
			http.Error(w, "session ownership check unavailable", http.StatusServiceUnavailable)
			return
		}
		if s.sessionAuth != nil {
			if err := s.sessionAuth.CheckOwnership(r.Context(), sessionID, userID); err != nil {
				s.lg.Warn("voice WS ownership denied",
					loggateway.StepID("voice.ws.ownership_denied"),
					loggateway.Str("session_id", sessionID),
					loggateway.Str("user_id", userID),
					loggateway.Err(err))
				http.Error(w, "session ownership required", http.StatusForbidden)
				return
			}
		}
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.lg.Warn("voice WS upgrade failed", loggateway.StepID("voice.ws.upgrade_failed"), loggateway.Err(err))
		return
	}

	down := &voiceWSDownlink{conn: conn}
	deps := voice.SessionDeps{
		NewASR:          s.newASR,
		NewTTS:          s.newTTS,
		Bus:             s.bus,
		Executor:        voiceChatTurnExecutor{inner: s.executor},
		Canceller:       voiceRunCanceller{inner: s.canceller},
		Confirmer:       s.confirmer,
		Archiver:        s.archiver,
		Prewarmer:       s.prewarmer,
		Speculator:      s.speculator,
		Delegation:      s.delegation,
		DelegationSteps: s.delegationSteps,
		Infra:           s.infra,
		LG:              s.lg,
	}
	sess := voice.NewSession(r.Context(), deps, sessionID, userID, down)

	// 单会话单语音连接：第二连接到达时旧连接收 voice.replaced 后关闭（设计 §2.1）
	s.mu.Lock()
	old := s.conns[sessionID]
	s.conns[sessionID] = sess
	s.mu.Unlock()
	if old != nil {
		old.ReplaceNoticeAndClose()
	}

	s.lg.Info("voice WS connected",
		loggateway.StepID("voice.ws.connected"),
		loggateway.SessionID(sessionID),
		loggateway.Str("user_id", userID))
	s.readPump(sess, conn, sessionID)
}

func (s *VoiceWSServer) readPump(sess *voice.Session, conn *websocket.Conn, sessionID string) {
	defer func() {
		s.mu.Lock()
		if s.conns[sessionID] == sess {
			delete(s.conns, sessionID)
		}
		s.mu.Unlock()
		sess.Close()
		_ = conn.Close()
	}()
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		switch mt {
		case websocket.BinaryMessage:
			sess.WriteAudio(data)
		case websocket.TextMessage:
			var msg voiceControlMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			s.handleControl(sess, msg)
		}
	}
}

type voiceControlMessage struct {
	Type       string `json:"type"`
	SampleRate int    `json:"sample_rate"`
	Language   string `json:"language"`
	DialogMode string `json:"dialog_mode"`
	AgentKey   string `json:"agent_key"`
	TeamID     string `json:"team_id"`
	DetectMs   int    `json:"detect_ms"`
	// Mode 会话模式（空=对话；voice.ModeDictation=听写：终稿仅下行文本不建 Turn）。
	Mode string `json:"mode"`
	// Source 唤醒来源（V10，仅 voice.wake 使用）：kws | manual | system。
	Source string `json:"source"`
}

func (s *VoiceWSServer) handleControl(sess *voice.Session, msg voiceControlMessage) {
	switch msg.Type {
	case "voice.start":
		sess.Start(voice.StartParams{
			SampleRate: msg.SampleRate,
			Language:   msg.Language,
			DialogMode: msg.DialogMode,
			AgentKey:   msg.AgentKey,
			TeamID:     msg.TeamID,
			Mode:       msg.Mode,
		})
	case "voice.stop":
		sess.Stop()
	case "voice.commit":
		sess.Commit()
	case "voice.barge_in", "voice.cancel": // V1 裁剪 #4：barge_in 复用 cancel 路径
		sess.Cancel(msg.Type)
	case "voice.wake": // V10：dormant → listening（KWS 检出/手动点击；非 dormant 幂等忽略）
		sess.Wake(msg.Source)
	case "ping":
		sess.Ping()
	}
}

// voiceWSDownlink 实现 voice.Downlink；gorilla 写并发需写锁。
type voiceWSDownlink struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (d *voiceWSDownlink) SendJSON(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.conn.WriteMessage(websocket.TextMessage, raw)
}

func (d *voiceWSDownlink) SendAudio(pcm []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.conn.WriteMessage(websocket.BinaryMessage, pcm)
}

// ---- server 既有端口 → voice 窄端口适配器 ----

type voiceChatTurnExecutor struct{ inner WSTurnExecutor }

func (a voiceChatTurnExecutor) ExecuteTurn(ctx context.Context, in voice.ChatTurnInput) error {
	if a.inner == nil {
		return nil
	}
	return a.inner.ExecuteTurn(ctx, WSTurnInput{
		SessionID:   in.SessionID,
		Content:     in.Content,
		AgentKey:    in.AgentKey,
		TeamID:      in.TeamID,
		AllowQueue:  true,
		AllowStream: true,
		Voice:       in.Voice, // V2-T6 语音溯源元数据透传
	})
}

type voiceRunCanceller struct{ inner RunCanceller }

func (a voiceRunCanceller) CancelRun(ctx context.Context, sessionID string) bool {
	if a.inner == nil {
		return false
	}
	return a.inner.CancelRun(ctx, sessionID)
}
