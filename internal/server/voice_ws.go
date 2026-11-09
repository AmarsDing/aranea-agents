package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

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

// VoiceRevocationRegistrar 是语音侧需要的会话吊销登记端口（评审 2026-11-09
// MEDIUM-2）：握手成功的语音连接经它纳入与 /v1/ws 相同的「注册 → tv 兜底
// 清扫 / pub/sub bump 主动关闭」机制。由 *WSServer 实现，NewHTTPServer
// 组合根注入；nil 或未接线 = 语音连接不受吊销覆盖（仅限测试装配）。
type VoiceRevocationRegistrar interface {
	RegisterRevocableConn(c RevocableConn)
	UnregisterRevocableConn(c RevocableConn)
}

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
	// revocation 会话吊销登记端口（MEDIUM-2）；nil = 不登记（测试）。
	revocation VoiceRevocationRegistrar

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
		// 评审 2026-11-09 V2：CheckOrigin 原先恒真——任何网页均可跨站建立
		// 语音 WS（配合用户登录态实施跨站语音指令/旁听）。与 /v1/ws 对齐为
		// 同一 OriginAllowed 策略：空 Origin（非浏览器客户端）放行，其余按
		// CORS 白名单（dev localhost 前缀 + KRATOS_HTTP_EXTRA_CORS_ORIGINS
		// 精确匹配）同语义判定，不放宽。
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				if WSOriginAllowed(r) {
					return true
				}
				lg.Warn("voice WS origin rejected",
					loggateway.StepID("voice.ws.origin_rejected"),
					loggateway.Str("origin", strings.TrimSpace(r.Header.Get("Origin"))),
					loggateway.Str("remote_addr", r.RemoteAddr))
				return false
			},
		},
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

// SetRevocationRegistrar 注入会话吊销登记端口（评审 2026-11-09 MEDIUM-2，
// 可选依赖 setter 模式，与 SetTurnPrewarmer 一致）；nil 或未调用 = 语音连接
// 不纳入会话吊销覆盖（仅限测试装配；生产由 NewHTTPServer 注入 *WSServer）。
func (s *VoiceWSServer) SetRevocationRegistrar(r VoiceRevocationRegistrar) {
	if s == nil {
		return
	}
	s.revocation = r
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

	// MEDIUM-2（评审 2026-11-09）：握手成功后纳入会话吊销覆盖——账号吊销/
	// 改密 bump 即时关闭（pub/sub），删号/漏事件由兜底扫描比对握手期 tv
	// 关闭（与 /v1/ws 同一机制，同一份 TokenVersionIsCurrent 判定）。
	var revEntry *voiceRevocableConn
	if s.revocation != nil {
		revEntry = &voiceRevocableConn{
			userID:       userID,
			sessionID:    sessionID,
			tokenVersion: claims.TokenVersion,
			conn:         conn,
		}
		s.revocation.RegisterRevocableConn(revEntry)
	}

	s.lg.Info("voice WS connected",
		loggateway.StepID("voice.ws.connected"),
		loggateway.SessionID(sessionID),
		loggateway.Str("user_id", userID))
	s.readPump(sess, conn, sessionID, revEntry)
}

func (s *VoiceWSServer) readPump(sess *voice.Session, conn *websocket.Conn, sessionID string, revEntry *voiceRevocableConn) {
	defer func() {
		// 单一拆除路径：无论自然断开还是 RevokeClose 触发，统一在此摘除
		// 吊销登记与会话索引（两者均幂等，吊销路径已先摘亦安全）。
		if revEntry != nil && s.revocation != nil {
			s.revocation.UnregisterRevocableConn(revEntry)
		}
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

// voiceRevocableConn 把一条语音 WS 连接适配进 WSServer 的会话吊销机制
// （评审 2026-11-09 MEDIUM-2，实现 server.RevocableConn）。
type voiceRevocableConn struct {
	userID       string
	sessionID    string
	tokenVersion int64 // 握手期 JWT tv claim，兜底扫描比对用
	conn         *websocket.Conn
	closeOnce    sync.Once // pub/sub 与兜底扫描可能并发触发
}

func (c *voiceRevocableConn) RevokeUserID() string      { return c.userID }
func (c *voiceRevocableConn) RevokeTokenVersion() int64 { return c.tokenVersion }
func (c *voiceRevocableConn) RevokeSessionID() string   { return c.sessionID }

// RevokeClose 发送吊销 close frame（4401）后关闭底层连接——readPump 的
// ReadMessage 随之出错返回，其 defer 完成 UnregisterRevocableConn / 摘会话
// 索引 / sess.Close()（单一拆除路径，与聊天 WS 的 readPump 清理同构）。
// gorilla 保证 WriteControl 与 Close 可与其它方法并发调用，故与 TTS 下行
// 音频帧（downlink 写锁内）并发安全。
func (c *voiceRevocableConn) RevokeClose(code int, reason string) {
	c.closeOnce.Do(func() {
		_ = c.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(code, reason),
			time.Now().Add(time.Second),
		)
		_ = c.conn.Close()
	})
}

// voiceWSDownlink 实现 voice.Downlink + voice.DownlinkCloser；gorilla 写并发
// 需写锁。
type voiceWSDownlink struct {
	conn      *websocket.Conn
	mu        sync.Mutex
	closeOnce sync.Once // 替换关闭与吊销/自然拆除可能并发触发
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

// CloseWithCode 以业务关闭码优雅关闭底层连接（评审 2026-11-09 V1，实现
// voice.DownlinkCloser，供 ReplaceNoticeAndClose 关闭被替换的旧连接）：
// 先写 close frame（对端读到有序的最后一帧 voice.replaced 后收到明确关闭码），
// 再 Close。幂等；gorilla 保证 WriteControl/Close 可与其它写并发，故与
// TTS 下行音频帧（本结构写锁内）及吊销路径（voiceRevocableConn.RevokeClose）
// 并发安全。readPump 的 ReadMessage 随 Close 出错返回，其 defer 完成
// 单一拆除路径（摘吊销登记/会话索引/sess.Close），无需此处代劳。
func (d *voiceWSDownlink) CloseWithCode(code int, text string) {
	d.closeOnce.Do(func() {
		_ = d.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(code, text),
			time.Now().Add(time.Second),
		)
		_ = d.conn.Close()
	})
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
