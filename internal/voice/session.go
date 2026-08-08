package voice

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
)

// idleReclaimTimeout 是 ASR 上游空闲回收时长（设计 §2.1）。
const idleReclaimTimeout = 10 * time.Minute

// interruptedSettleDelay 是 interrupted 过渡态自动回 listening 的延迟（设计 §5）。
const interruptedSettleDelay = 300 * time.Millisecond

// ---- 注入端口（server 层适配，避免 voice 依赖 internal/server）----

// ASRProviderFactory 按当前配置解析 ASR Provider + 会话参数。
type ASRProviderFactory func(ctx context.Context) (biz.StreamingASRProvider, biz.ASRSessionConfig, error)

// TTSProviderFactory 按当前配置解析 TTS Provider + 会话参数。
type TTSProviderFactory func(ctx context.Context) (biz.StreamingTTSProvider, biz.TTSSessionConfig, error)

// ChatTurnInput 是入 Chat 管线的最小参数集（对齐 server.WSTurnInput 子集）。
type ChatTurnInput struct {
	SessionID string
	Content   string
	AgentKey  string
	TeamID    string
}

// Stability:evolving — Chat 管线入口端口（server.WSTurnExecutor 适配实现）。
type ChatTurnExecutor interface {
	ExecuteTurn(ctx context.Context, input ChatTurnInput) error
}

// Stability:evolving — Turn 取消端口（server.RunCanceller 适配实现）。
type RunCanceller interface {
	CancelRun(ctx context.Context, sessionID string) bool
}

// Downlink 是网关 → 客户端的下行通道（WS 实现，写锁由实现保证）。
type Downlink interface {
	SendJSON(v any) error
	SendAudio(pcm []byte) error
}

// StartParams 对应 voice.start 控制帧。
type StartParams struct {
	SampleRate int
	Language   string
	DialogMode string
	AgentKey   string
	TeamID     string
}

// SessionDeps 是语音会话的全部外部依赖。
type SessionDeps struct {
	NewASR    ASRProviderFactory
	NewTTS    TTSProviderFactory
	Bus       biz.EventBus
	Executor  ChatTurnExecutor
	Canceller RunCanceller
	// Confirmer 语音确认决议端口（V2-T5）；nil 时关闭语音确认拦截。
	Confirmer ConfirmResolver
	Infra     *event.Infra
	LG        loggateway.Logger
}

// Session 编排单条语音 WS 连接的生命周期（设计 §2.4）。
type Session struct {
	sessionID string
	userID    string
	deps      SessionDeps
	down      Downlink
	lg        loggateway.Logger
	flow      *event.TraceEmitter

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	closed chan struct{}
	once   sync.Once

	mu            sync.Mutex
	state         VoiceState
	params        StartParams
	asr           biz.ASRSession
	chunker       *SentenceChunker
	scheduler     *TTSScheduler
	pendingTurns  int
	ttsStarted    bool
	flushEnqueued bool // 当前 Turn 尾句（flush=true）是否已入队
	turnSeq       int
	eventStarted  bool
	unsub         func()
	idleTimer     *time.Timer
}

func NewSession(ctx context.Context, deps SessionDeps, sessionID, userID string, down Downlink) *Session {
	sctx, cancel := context.WithCancel(ctx)
	lg := deps.LG
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	lg = lg.With(loggateway.Domain("voice"), loggateway.SessionID(sessionID))
	s := &Session{
		sessionID: sessionID,
		userID:    userID,
		deps:      deps,
		down:      down,
		lg:        lg,
		ctx:       sctx,
		cancel:    cancel,
		state:     StateIdle,
		closed:    make(chan struct{}),
	}
	s.flow = event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:       sctx,
		SessionID: sessionID,
		Domain:    event.TraceDomainVoice,
		LG:        lg,
		Infra:     deps.Infra,
	})
	return s
}

// ---- 网关入口 ----

// Start 处理 voice.start：开 ASR、订阅事件流、idle/error → listening。
func (s *Session) Start(p StartParams) {
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	if st != StateIdle && st != StateError {
		return
	}
	if err := s.transition(EvVoiceStart); err != nil {
		return
	}
	s.mu.Lock()
	s.params = p
	s.mu.Unlock()
	if err := s.openASR(); err != nil {
		s.sendError("ASR_UNAVAILABLE", err, true)
		s.recoverToListening()
		return
	}
	s.startEventLoop()
	s.resetIdleTimer()
	s.flow.LogStart("voice.session.start", "语音会话开始")
	s.lg.Info("voice session started", loggateway.StepID("voice.session.start"))
	s.broadcastState()
}

// WriteAudio 处理上行 PCM 帧；ASR 被空闲回收后懒重连。
func (s *Session) WriteAudio(pcm []byte) {
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	if st != StateListening {
		return // thinking/speaking/idle/error 不收音频（打断走控制帧）
	}
	s.resetIdleTimer()
	s.mu.Lock()
	asr := s.asr
	s.mu.Unlock()
	if asr == nil {
		if err := s.openASR(); err != nil {
			s.sendError("ASR_UNAVAILABLE", err, true)
			return
		}
		s.mu.Lock()
		asr = s.asr
		s.mu.Unlock()
	}
	if err := asr.Write(pcm); err != nil {
		s.sendError("ASR_WRITE", err, true)
	}
}

// Commit 处理 voice.commit（PTT 兜底）：标记当前语句结束。
func (s *Session) Commit() {
	s.mu.Lock()
	asr := s.asr
	s.mu.Unlock()
	if asr == nil {
		return
	}
	if err := asr.Finish(); err != nil {
		s.sendError("ASR_FINISH", err, true)
	}
}

// Cancel 处理 voice.cancel / voice.barge_in（V1 裁剪 #4：同路径）。
func (s *Session) Cancel(reason string) {
	s.deps.Canceller.CancelRun(ctxuser.WithUserID(context.Background(), s.userID), s.sessionID)
	s.mu.Lock()
	sch := s.scheduler
	st := s.state
	s.chunker = nil
	s.scheduler = nil
	s.ttsStarted = false
	s.mu.Unlock()
	if sch != nil {
		sch.Cancel()
	}
	_ = s.down.SendJSON(map[string]any{"type": "tts.end", "interrupted": true})
	switch st {
	case StateSpeaking:
		if err := s.transition(EvBargeIn); err == nil {
			s.broadcastState()
			s.flow.LogDone("voice.barge_in", "语音打断", event.P("reason", reason))
			time.AfterFunc(interruptedSettleDelay, func() {
				s.mu.Lock()
				settled := s.state == StateInterrupted
				if settled {
					s.state = StateListening // 过渡态自动回 listening（设计 §5，无需事件）
				}
				s.mu.Unlock()
				if settled {
					s.broadcastState()
				}
			})
		}
	case StateThinking:
		if err := s.transition(EvBargeIn); err == nil {
			s.broadcastState()
		}
	}
}

// Stop 处理 voice.stop：退出语音模式，全量清理（连接保留）。
func (s *Session) Stop() {
	if err := s.transition(EvVoiceStop); err == nil {
		s.broadcastState()
	}
	s.teardown()
	s.flow.LogDone("voice.session.done", "语音会话结束")
	s.lg.Info("voice session stopped", loggateway.StepID("voice.session.done"))
}

// Ping 处理心跳。
func (s *Session) Ping() {
	_ = s.down.SendJSON(map[string]any{"type": "pong"})
}

// ReplaceNoticeAndClose 发 voice.replaced 后关闭（单会话单连接，设计 §2.1）。
func (s *Session) ReplaceNoticeAndClose() {
	_ = s.down.SendJSON(map[string]any{"type": "voice.replaced"})
	s.Close()
}

// Close 全量拆除（连接断开）。幂等。
func (s *Session) Close() {
	s.once.Do(func() {
		s.teardown()
		s.cancel()
		close(s.closed)
		s.wg.Wait()
	})
}

// ---- 内部：ASR ----

func (s *Session) openASR() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.asr != nil {
		return nil
	}
	provider, cfg, err := s.deps.NewASR(s.ctx)
	if err != nil {
		return err
	}
	if s.params.SampleRate > 0 {
		cfg.SampleRate = s.params.SampleRate
	}
	if s.params.Language != "" {
		cfg.Language = s.params.Language
	}
	sess, err := provider.Open(s.ctx, cfg)
	if err != nil {
		return err
	}
	s.asr = sess
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.asrPump(sess)
	}()
	return nil
}

func (s *Session) asrPump(sess biz.ASRSession) {
	events := sess.Events()
	for {
		select {
		case <-s.ctx.Done(): // Close 先 cancel 再 wg.Wait：pump 必须响应 ctx 退出
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			switch ev.Type {
			case biz.ASREventPartial:
				_ = s.down.SendJSON(map[string]any{"type": "asr.partial", "text": ev.Text})
			case biz.ASREventFinal:
				s.handleASRFinal(ev)
			case biz.ASREventError:
				s.sendError("ASR_ERROR", ev.Err, true)
				s.recoverToListening()
			case biz.ASREventVadEnd:
				// 服务端 VAD 端点信号；终稿由 Final 事件承载，V1 无动作
			}
		}
	}
}

func (s *Session) handleASRFinal(ev biz.ASREvent) {
	text := strings.TrimSpace(ev.Text)
	if text == "" {
		return
	}
	_ = s.down.SendJSON(map[string]any{"type": "asr.final", "text": text, "duration_ms": ev.DurationMs})
	s.flow.LogDone("voice.asr.final", "语音识别终稿", event.P("duration_ms", ev.DurationMs))
	// V2-T5 语音确认拦截：词表命中且有待决议确认时，决议确认且不创建 turn。
	if decision := MatchVoiceConfirm(text); decision != VoiceConfirmNone {
		if s.tryResolveVoiceConfirm(text, decision) {
			return
		}
	}
	s.mu.Lock()
	if _, err := Transition(s.state, EvASRFinal); err != nil {
		s.mu.Unlock()
		return // 非 listening 态的残余 final（如 cancel 后），忽略
	}
	s.state = StateThinking
	s.pendingTurns++
	s.flushEnqueued = false
	s.turnSeq++
	turnID := s.turnSeq
	params := s.params
	s.mu.Unlock()
	s.broadcastState()
	_ = s.down.SendJSON(map[string]any{"type": "turn.accepted", "turn_id": turnRef(turnID)})
	// 与 WS 用户消息一致：turn 存活独立于连接（appctx），传播 userID。
	turnCtx := ctxuser.WithUserID(appctx.Ctx(), s.userID)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.deps.Executor.ExecuteTurn(turnCtx, ChatTurnInput{
			SessionID: s.sessionID, Content: text, AgentKey: params.AgentKey, TeamID: params.TeamID,
		}); err != nil {
			s.handleTurnFailure(err)
		}
	}()
}

// tryResolveVoiceConfirm 尝试将词表命中的语句决议为工具确认（V2-T5）。
// 返回 true = 已拦截（停留 listening）；false = 按普通语句进 Chat 管线
// （无待决议确认 / resolver 故障降级，NFR7：语音失败不影响文字对话）。
func (s *Session) tryResolveVoiceConfirm(text string, decision VoiceConfirmDecision) bool {
	s.mu.Lock()
	resolver := s.deps.Confirmer
	st := s.state
	s.mu.Unlock()
	if resolver == nil || st != StateListening {
		return false
	}
	approved := decision == VoiceConfirmApprove
	// 与 Chat Turn 一致：决议存活独立于连接（appctx），传播 userID。
	ctx := ctxuser.WithUserID(appctx.Ctx(), s.userID)
	resolved, err := resolver.ResolvePendingConfirm(ctx, s.sessionID, approved)
	if err != nil {
		s.lg.Warn("voice confirm resolve failed, falling through to chat turn",
			loggateway.StepID("voice.confirm.resolve_fail"), loggateway.Err(err))
		return false
	}
	if !resolved {
		return false
	}
	word := "deny"
	if approved {
		word = "approve"
	}
	_ = s.down.SendJSON(map[string]any{"type": "confirm.resolved", "decision": word})
	s.flow.LogDone("voice.confirm.resolved", "语音确认决议", event.P("decision", word), event.P("text", text))
	s.lg.Info("voice confirm resolved", loggateway.StepID("voice.confirm.resolved"), loggateway.Str("decision", word))
	return true
}

func turnRef(n int) string {
	return "vt-" + strings.Repeat("0", 0) + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// ---- 内部：事件流 → chunker → TTS ----

func (s *Session) startEventLoop() {
	s.mu.Lock()
	if s.eventStarted {
		s.mu.Unlock()
		return
	}
	s.eventStarted = true
	s.mu.Unlock()
	ch, unsub := s.deps.Bus.Subscribe(biz.EventSubscribeOptions{SpiritSessionID: s.sessionID})
	s.mu.Lock()
	s.unsub = unsub
	s.mu.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.eventLoop(ch)
	}()
}

func (s *Session) eventLoop(ch <-chan biz.Event) {
	for {
		select {
		case <-s.ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			switch ev := e.(type) {
			case *biz.StepStreamingEvent:
				if ev.DeltaField == "content" && ev.DeltaChunk != "" {
					s.feedDelta(ev.DeltaChunk)
				}
			case *biz.TurnCompletedEvent:
				s.handleTurnCompleted()
			case *biz.TurnFailedEvent:
				s.handleTurnFailure(nil)
			}
		}
	}
}

func (s *Session) feedDelta(delta string) {
	s.mu.Lock()
	active := s.pendingTurns > 0 && (s.state == StateThinking || s.state == StateSpeaking)
	s.mu.Unlock()
	if !active {
		return
	}
	if err := s.ensureTTS(); err != nil {
		s.sendError("TTS_UNAVAILABLE", err, true)
		return
	}
	s.mu.Lock()
	ch := s.chunker
	s.mu.Unlock()
	if ch != nil {
		ch.Write(delta) // 队列满时 Enqueue 阻塞 = 背压（设计 §4.2）
	}
}

func (s *Session) ensureTTS() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.chunker != nil {
		return nil
	}
	provider, cfg, err := s.deps.NewTTS(s.ctx)
	if err != nil {
		return err
	}
	s.scheduler = NewTTSScheduler(TTSSchedulerOpts{
		Provider:  provider,
		Config:    cfg,
		OnAudio:   s.onTTSAudio,
		OnDrained: s.onTTSDrained,
		OnError:   s.onTTSError,
		LG:        s.lg,
	})
	s.scheduler.Start(s.ctx)
	s.chunker = NewSentenceChunker(func(text string, flush bool) {
		if flush {
			s.mu.Lock()
			s.flushEnqueued = true
			s.mu.Unlock()
		}
		if err := s.scheduler.Enqueue(s.ctx, text, flush); err != nil {
			s.lg.Warn("voice tts enqueue failed", loggateway.StepID("voice.tts.enqueue_fail"), loggateway.Err(err))
		}
	})
	return nil
}

func (s *Session) handleTurnCompleted() {
	s.mu.Lock()
	if s.pendingTurns > 0 {
		s.pendingTurns--
	}
	ch := s.chunker
	sch := s.scheduler
	idleNoText := s.scheduler == nil && s.state == StateThinking
	s.mu.Unlock()
	if ch != nil {
		ch.Flush()
	}
	if sch != nil {
		// Turn 文本可能已在句边界全部切出（Flush 无残余、尾句 flush=false）：
		// 补一条空文本 flush 哨兵驱动 OnDrained，否则 tts.end 缺失、状态停在 speaking。
		s.mu.Lock()
		tailSent := s.flushEnqueued
		s.mu.Unlock()
		if !tailSent {
			if err := sch.Enqueue(s.ctx, "", true); err != nil {
				s.lg.Warn("voice tts flush sentinel enqueue failed", loggateway.StepID("voice.tts.enqueue_fail"), loggateway.Err(err))
			}
		}
	}
	if idleNoText {
		// 无文本 Turn（纯工具调用等）：thinking --tts_end--> listening（设计 §5）
		if err := s.transition(EvTTSEnd); err == nil {
			s.broadcastState()
		}
	}
}

func (s *Session) handleTurnFailure(err error) {
	s.mu.Lock()
	if s.pendingTurns > 0 {
		s.pendingTurns--
	}
	s.mu.Unlock()
	if err == nil {
		err = errTurnFailed
	}
	s.sendError("TURN_FAILED", err, true)
	s.recoverToListening()
}

var errTurnFailed = errString("turn failed")

type errString string

func (e errString) Error() string { return string(e) }

// recoverToListening 可恢复错误路径：error --voice_start--> listening（设计 §5）。
func (s *Session) recoverToListening() {
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	if st == StateListening || st == StateIdle {
		return
	}
	if err := s.transition(EvTurnFailed); err == nil {
		s.broadcastState()
	}
	if err := s.transition(EvVoiceStart); err == nil {
		s.broadcastState()
	}
}

// ---- 内部：TTS 回调 ----

func (s *Session) onTTSAudio(pcm []byte) {
	s.mu.Lock()
	first := !s.ttsStarted
	if first {
		s.ttsStarted = true
	}
	s.mu.Unlock()
	if first {
		_ = s.down.SendJSON(map[string]any{"type": "tts.start", "encoding": "pcm_f32le_16k", "sample_rate": 16000})
		s.flow.LogDone("voice.tts.start", "语音播报开始")
		if err := s.transition(EvFirstTTSAudio); err == nil {
			s.broadcastState()
		}
	}
	_ = s.down.SendAudio(pcm)
}

func (s *Session) onTTSDrained() {
	s.mu.Lock()
	s.ttsStarted = false
	s.chunker = nil
	s.scheduler = nil
	s.mu.Unlock()
	_ = s.down.SendJSON(map[string]any{"type": "tts.end"})
	s.flow.LogDone("voice.tts.end", "语音播报结束")
	if err := s.transition(EvTTSEnd); err == nil {
		s.broadcastState()
	}
}

func (s *Session) onTTSError(err error) {
	// K3 降级：连续合成失败 → 告知前端退回文字模式，状态回 listening
	s.flow.LogWarn("voice.provider.fallback", "语音服务降级", "TTS 连续合成失败", event.P("error", err.Error()))
	s.mu.Lock()
	s.ttsStarted = false
	s.chunker = nil
	s.scheduler = nil
	s.mu.Unlock()
	_ = s.down.SendJSON(map[string]any{"type": "tts.end"})
	s.sendError("TTS_UNAVAILABLE", err, true)
	if err := s.transition(EvTTSEnd); err == nil {
		s.broadcastState()
	}
}

// ---- 内部：公共 ----

func (s *Session) transition(ev VoiceEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	to, err := Transition(s.state, ev)
	if err != nil {
		return err
	}
	s.state = to
	return nil
}

func (s *Session) broadcastState() {
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	_ = s.down.SendJSON(map[string]any{"type": "voice.state", "state": string(st)})
}

func (s *Session) sendError(code string, err error, retryable bool) {
	s.flow.LogError("voice.error", "语音链路错误", event.P("code", code), event.P("error", err.Error()))
	s.lg.Warn("voice session error", loggateway.StepID("voice.error"), loggateway.Str("code", code), loggateway.Err(err))
	_ = s.down.SendJSON(map[string]any{"type": "voice.error", "code": code, "message": err.Error(), "retryable": retryable})
}

func (s *Session) resetIdleTimer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idleTimer != nil {
		s.idleTimer.Stop()
	}
	s.idleTimer = time.AfterFunc(idleReclaimTimeout, s.reclaimIdleASR)
}

func (s *Session) reclaimIdleASR() {
	s.mu.Lock()
	asr := s.asr
	s.asr = nil
	s.mu.Unlock()
	if asr != nil {
		_ = asr.Close()
		s.lg.Info("voice asr: idle upstream reclaimed", loggateway.StepID("voice.asr.idle_reclaim"))
	}
}

func (s *Session) teardown() {
	s.mu.Lock()
	if s.idleTimer != nil {
		s.idleTimer.Stop()
		s.idleTimer = nil
	}
	asr := s.asr
	s.asr = nil
	sch := s.scheduler
	s.scheduler = nil
	s.chunker = nil
	unsub := s.unsub
	s.unsub = nil
	s.mu.Unlock()
	if asr != nil {
		_ = asr.Close()
	}
	if sch != nil {
		sch.Cancel()
	}
	if unsub != nil {
		unsub()
	}
}
