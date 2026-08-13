package voice

// TECH-DEBT(COG): 文件总行数=1355, 上限=500（AS-COG-01 既有债务，V8/V9
// 持续加剧）——下一迭代按职责拆分：eventLoop/委派播报 → session_delegation.go，
// 确认/澄清 → session_confirm.go，ASR 泵 → session_asr.go。

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// idleReclaimTimeout 是 ASR 上游空闲回收时长（设计 §2.1）。
const idleReclaimTimeout = 10 * time.Minute

// firstAudioBudget 是「ASR 终稿 → 首帧音频下行」的语音快速通道预算
// （E，2026-08-10：停口→出声目标 ~1.7s，含编排/LLM TTFT/首句合成）。
// 超预算不阻断，仅 Warn 提示排障（K2）。
const firstAudioBudget = 2500 * time.Millisecond

// interruptedSettleDelay 是 interrupted 过渡态自动回 listening 的延迟（设计 §5）。
const interruptedSettleDelay = 300 * time.Millisecond

// speculativeStabilityDelay 是 ASR partial 文本稳定判定窗口（C2，设计 §14.4）：
// partial 文本 500ms 无变化视为稳定，触发投机意图（L2）。文本变化重置计时；
// 同文重发不重置（稳定窗口从首次出现算起）。
const speculativeStabilityDelay = 500 * time.Millisecond

// voiceTurnBusyRetries/voiceTurnBusyBackoff 对齐 channel 入口的 busy 重试策略
// （channelTurnBusyRetries/channelTurnBusyBackoff）。CHAT_TURN_BUSY 是准入
// TOCTOU 竞态（chat_orchestrator_turn.go 锁内复查）：重试让准入重查，
// AllowQueue=true 时消息转入排队队列，而非整轮无回复无播报。
const (
	voiceTurnBusyRetries = 3
	voiceTurnBusyBackoff = 250 * time.Millisecond
)

// isTurnBusyError 报告 CHAT_TURN_BUSY 准入竞态（镜像 service/turn_outcome.go
// 的判定；voice 不反向依赖 service，领域字符串在此保持一致）。
func isTurnBusyError(err error) bool {
	ae, ok := apierror.From(err)
	return ok && ae.Code == apierror.CodeConflict && ae.Domain == "CHAT_TURN_BUSY"
}

// executeTurnWithBusyRetry 派发 Chat Turn，撞 CHAT_TURN_BUSY 时短退避重试。
func (s *Session) executeTurnWithBusyRetry(ctx context.Context, in ChatTurnInput) error {
	var err error
	for attempt := 0; attempt < voiceTurnBusyRetries; attempt++ {
		if attempt > 0 {
			if attempt == 1 {
				// K4：首次重试记一条进程日志即可，非每次
				s.lg.Warn("voice turn busy, retrying (K4)",
					loggateway.StepID("voice.turn.busy_retry"),
					loggateway.SessionID(s.sessionID))
			}
			timer := time.NewTimer(voiceTurnBusyBackoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		if err = s.deps.Executor.ExecuteTurn(ctx, in); err == nil || !isTurnBusyError(err) {
			return err
		}
	}
	return err
}

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
	// Voice 语音输入溯源元数据（V2-T6）：随用户消息 options_json 持久化；
	// ASR 终稿派发的 Turn 恒非 nil。
	Voice *biz.VoiceTurnMeta
}

// Stability:evolving — Chat 管线入口端口（server.WSTurnExecutor 适配实现）。
type ChatTurnExecutor interface {
	ExecuteTurn(ctx context.Context, input ChatTurnInput) error
}

// Stability:evolving — Turn 预热端口（service.VoiceTurnPrewarmer 适配实现）。
// C1（2026-08-10）：voice.start 成功后后台触发一次 Agent 构建缓存填充，
// 消除首个语音 Turn 的构建冷启动（真机实测 cache miss 2.6-2.7s）。
// 实现必须非阻断容错：预热失败仅记日志，不影响语音链路。
type TurnPrewarmer interface {
	PrewarmTurn(ctx context.Context, sessionID string)
}

// Stability:evolving — 投机意图端口（service.VoiceIntentSpeculator 适配实现）。
// C2（2026-08-11）：ASR partial 文本稳定 500ms 后后台预跑意图识别（投机阶梯
// L2）；ASR final 时（L3 判定）final 文本与投机源一致则注入产物复用，
// 失配/超时/失败即丢弃走常规意图路径。实现必须非阻断容错，nil 关闭投机。
type IntentSpeculator interface {
	// SpeculateIntent 对稳定 partial 文本后台预跑意图识别（fire-and-forget）。
	SpeculateIntent(ctx context.Context, sessionID, text string)
	// WithSpeculativeIntent 判定 final 复用：命中返回注入产物的 ctx，
	// 未命中原样返回。允许有界等待在途投机完成（实现侧兜底上限）。
	WithSpeculativeIntent(ctx context.Context, sessionID, finalText string) context.Context
}

// ttsConnPrewarmer 是 TTS Provider 的可选预热能力（L5，2026-08-11）：
// voice.start 预拨 WS 连接存槽，首个 Turn 首句 Write 弹出复用免握手。
// 未实现该能力的 provider 自动跳过预热（首句维持逐句拨号现状）。
type ttsConnPrewarmer interface {
	PrewarmTTSConn(ctx context.Context)
	ReleaseWarmTTSConn()
}

// Stability:evolving — Turn 取消端口（server.RunCanceller 适配实现）。
type RunCanceller interface {
	CancelRun(ctx context.Context, sessionID string) bool
}

// Stability:evolving — 委派播报取精灵终稿的窄端口（M74 V9，设计 74 §15.4.1
// R7：biz.StepV2Reader 子集，spirit_team_usecase.go 取终稿同款模式）。
type DelegationStepReader interface {
	ListStepsBySessionID(ctx context.Context, sessionID string) ([]biz.Step, error)
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
	// Mode 会话模式：空 = 对话（ASR 终稿建 Chat Turn + TTS 播报）；
	// ModeDictation = 听写（终稿仅下行文本，由前端填入输入框，不建 Turn 不播报）。
	Mode string
}

// ModeDictation 听写模式（聊天页语音输入按钮）。
const ModeDictation = "dictation"

// ModeCompanion 语音伴侣模式（V10）：进入即待命（dormant，本地 KWS 监听，
// ASR 零占用），检出唤醒词/手动唤醒后进 listening（设计 §16.2）。
const ModeCompanion = "companion"

// sleepTimeout V10：listening 态静默休眠阈值（需求 §2.12：60s 无交互回待命）。
// 包级变量（非 const）供测试缩短（ttsSentenceTimeout 同款先例）。
var sleepTimeout = 60 * time.Second

// V10 自足 TTS 应答文案（不经 Chat Turn，不占消息流）。
const wakeAckText = "我在"
const exitAckText = "好的，我先休息了"

// SessionDeps 是语音会话的全部外部依赖。
type SessionDeps struct {
	NewASR    ASRProviderFactory
	NewTTS    TTSProviderFactory
	Bus       biz.EventBus
	Executor  ChatTurnExecutor
	Canceller RunCanceller
	// Confirmer 语音确认决议端口（V2-T5）；nil 时关闭语音确认拦截。
	Confirmer ConfirmResolver
	// Archiver 语音留档端口（V2-T6）；nil 时关闭留档（不缓冲 PCM）。
	Archiver AudioArchiver
	// Prewarmer Turn 预热端口（C1）；nil 时关闭预热。听写模式恒跳过。
	Prewarmer TurnPrewarmer
	// Speculator 投机意图端口（C2）；nil 时关闭投机。听写模式恒跳过。
	Speculator IntentSpeculator
	// Delegation 委派登记表单例（M74 V9）；nil 时关闭委派三路分流
	// （eventLoop 仅处理本会话事件，无 task 绑定/终态播报）。
	Delegation *DelegationRegistry
	// DelegationSteps 委派播报终稿读取端口（R7）；nil 时委派完成仅播简报。
	DelegationSteps DelegationStepReader
	Infra           *event.Infra
	LG              loggateway.Logger
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
	asrDriver     string                   // 当前 ASR 配置驱动名（V2-T6 asr_provider 元数据）
	sampleRate    int                      // 当前 ASR 配置采样率（V2-T6 WAV 封装）
	ttsProvider   biz.StreamingTTSProvider // L5：Start 预解析持有（ensureTTS 复用；nil=未解析/失败回退工厂）
	ttsCfg        biz.TTSSessionConfig     // L5：与 ttsProvider 同次解析的会话参数
	utterBuf      []byte                   // 当前语句留档 PCM 缓冲（V2-T6；Archiver 非 nil 时累积）
	utterOverflow bool                     // 当前语句缓冲已截断（超 maxUtterancePCMBytes）
	chunker       *SentenceChunker
	scheduler     *TTSScheduler
	pendingTurns  int
	ttsStarted    bool
	turnT0        time.Time // 当前 Turn 的 T0（ASR 终稿派发时刻，E 首音频延迟测量）；首帧/取消时复位
	flushEnqueued bool      // 当前 Turn 尾句（flush=true）是否已入队
	turnSeq       int
	eventStarted  bool
	unsub         func()
	idleTimer     *time.Timer
	// V10：companion 静默休眠（listening 60s 无交互 → dormant，设计 §16.4②）。
	sleepTimer      *time.Timer // listening 态计时；进入 listening 重置、离开停止
	sleepAfterDrain bool         // 退出词应答播报完后转 dormant（onTTSDrained 消费）
	// C2 投机意图：partial 稳定追踪（specTimer 500ms 无变化触发投机）。
	partialText string      // 当前追踪的 partial 归一化文本
	partialGen  int         // partial 代际（文本变化递增，计时器防陈旧触发）
	specTimer   *time.Timer // 稳定判定计时器；final/cancel/teardown 停止
	// M74 V9：委派终态播报 FIFO（voice 正忙时排队，回 listening 排空，§15.3）。
	delegationOutbox []string
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
// V10：ModeCompanion 从 idle 进入 dormant（本地 KWS 监听，ASR 零占用，
// 设计 §16.2）；error 恢复/dictation 保持直进 listening。
func (s *Session) Start(p StartParams) {
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	if st != StateIdle && st != StateError {
		return
	}
	dormant := p.Mode == ModeCompanion && st == StateIdle
	if dormant {
		if err := s.transition(EvVoiceStartDormant); err != nil {
			return
		}
	} else if err := s.transition(EvVoiceStart); err != nil {
		return
	}
	s.mu.Lock()
	s.params = p
	s.mu.Unlock()
	if dormant {
		// dormant（G1）：保持事件订阅与委派 watcher（委派终态系统唤醒），
		// 延迟 ASR/TTS 预热至唤醒；C1 Turn 预热保留（与 ASR/TTS 无关）。
		s.startEventLoop()
		if reg := s.deps.Delegation; reg != nil {
			reg.SetWatcher(s.sessionID, s.onDelegationNotice)
		}
		if pw := s.deps.Prewarmer; pw != nil {
			s.wg.Add(1)
			safego.Go(appctx.Ctx(), "voice.prewarm", func() {
				defer s.wg.Done()
				pw.PrewarmTurn(ctxuser.WithUserID(appctx.Ctx(), s.userID), s.sessionID)
			})
		}
	} else {
		if err := s.openASR(); err != nil {
			s.sendError("ASR_UNAVAILABLE", err, true)
			s.recoverToListening()
			return
		}
		if !s.dictation() {
			s.startEventLoop() // 听写模式无 TTS 播报，不订阅事件总线
			// M74 V9：委派提交同步失败（永无 TaskCreated）的带外通知口播（R12）。
			if reg := s.deps.Delegation; reg != nil {
				reg.SetWatcher(s.sessionID, s.onDelegationNotice)
			}
			// C1 预热：后台填充 Agent 构建缓存，首个语音 Turn 免去冷启动。
			// 与 Turn 派发一致：预热存活独立于连接（appctx），传播 userID。
			if pw := s.deps.Prewarmer; pw != nil {
				s.wg.Add(1)
				safego.Go(appctx.Ctx(), "voice.prewarm", func() {
					defer s.wg.Done()
					pw.PrewarmTurn(ctxuser.WithUserID(appctx.Ctx(), s.userID), s.sessionID)
				})
			}
			// L5 预热：预解析 TTS provider（ensureTTS 复用）并预拨连接，
			// 首个 Turn 首句免握手。非阻断容错：失败仅 Warn，ensureTTS 回退工厂。
			s.resolveAndPrewarmTTS()
		}
	}
	s.resetIdleTimer()
	s.flow.LogStart("voice.session.start", "语音会话开始", event.P("mode", p.Mode))
	s.lg.Info("voice session started", loggateway.StepID("voice.session.start"), loggateway.Str("mode", p.Mode))
	s.broadcastState()
}

// Wake 处理 voice.wake：dormant → listening，懒启动 ASR 上游（V10，设计 §16.4②）。
// source ∈ kws/manual/system；kws/manual 播自足唤醒应答「我在」，
// system（委派终态唤醒）不应答——委派播报本身即内容。非 dormant 幂等忽略。
func (s *Session) Wake(source string) {
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	if st != StateDormant {
		return
	}
	if err := s.transition(EvWake); err != nil {
		return
	}
	if err := s.openASR(); err != nil {
		// 对齐 Start 错误路径：报 ASR_UNAVAILABLE，停留 listening 由前端重试。
		s.sendError("ASR_UNAVAILABLE", err, true)
	}
	s.flow.LogDone("voice.wake.detect", "语音唤醒", event.P("source", source))
	s.lg.Info("voice session woken", loggateway.StepID("voice.wake.detect"), loggateway.Str("source", source))
	s.broadcastState()
	if source != "system" {
		s.speakSelfSufficient(wakeAckText, "voice.wake.detect", "语音唤醒应答")
	}
}

// dictation 报告当前会话是否为听写模式（params 在 Start 时一次性写入）。
func (s *Session) dictation() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.params.Mode == ModeDictation
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
	s.bufferUtterance(pcm)
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

// bufferUtterance 累积当前语句 PCM（V2-T6 留档）；超上限截断并 Warn 一次（K3 双轨）。
func (s *Session) bufferUtterance(pcm []byte) {
	s.mu.Lock()
	if s.deps.Archiver == nil || s.params.Mode == ModeDictation {
		// 听写模式不创建消息，无留档挂载点，不缓冲 PCM。
		s.mu.Unlock()
		return
	}
	truncated := false
	if len(s.utterBuf)+len(pcm) > maxUtterancePCMBytes {
		if !s.utterOverflow {
			s.utterOverflow = true
			truncated = true
		}
	} else {
		s.utterBuf = append(s.utterBuf, pcm...)
	}
	s.mu.Unlock()
	if truncated {
		// 流程日志在锁外发射（TraceEmitter 异步，但避免持锁跨层调用）。
		s.lg.Warn("voice utterance archive buffer truncated",
			loggateway.StepID("voice.archive.truncate"),
			loggateway.Str("session_id", s.sessionID))
		s.flow.LogWarn("voice.archive.truncate", "语音留档截断", "语句音频超上限，留档仅保留前段",
			event.P("limit_bytes", maxUtterancePCMBytes))
	}
}

// takeUtteranceBuffer 取出并复位当前语句缓冲（终稿/取消/停止时调用）。
func (s *Session) takeUtteranceBuffer() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	buf := s.utterBuf
	s.utterBuf = nil
	s.utterOverflow = false
	return buf
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
	s.takeUtteranceBuffer() // 丢弃未终稿语句的留档缓冲（V2-T6）
	s.mu.Lock()
	sch := s.scheduler
	st := s.state
	s.chunker = nil
	s.scheduler = nil
	s.ttsStarted = false
	s.turnT0 = time.Time{}    // E：打断后迟到的音频帧不产生误导性延迟测量
	s.stopSpeculationLocked() // C2：打断后残余 partial 不再触发投机
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
	s.asrDriver = cfg.Driver
	s.sampleRate = cfg.SampleRate
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
				// 上游终结事件流（真机：火山末帧应答后 close 1000 关连接，一句一连接）。
				// CAS 摘掉已终结会话，下一句 WriteAudio 懒重开（对齐 reclaimIdleASR）。
				// teardown/reclaim 已摘表时 CAS 失败，不重复 Close、不打日志。
				s.mu.Lock()
				stale := s.asr == sess
				if stale {
					s.asr = nil
				}
				s.mu.Unlock()
				if stale {
					_ = sess.Close()
					s.lg.Info("voice asr: upstream stream ended, next utterance reopens",
						loggateway.StepID("voice.asr.upstream_end"))
				}
				return
			}
			switch ev.Type {
			case biz.ASREventPartial:
				_ = s.down.SendJSON(map[string]any{"type": "asr.partial", "text": ev.Text})
				s.trackPartialStability(ev.Text) // C2：稳定 500ms 触发投机意图
				s.resetSleepTimer()              // V10：ASR 活动重置静默休眠计时
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

// trackPartialStability 追踪 ASR partial 文本稳定性（C2）：文本变化重置
// 500ms 稳定计时；同文重发不重置（稳定窗口从首次出现算起）。稳定触发投机
// 意图（L2）。听写模式/未接线投机器时不追踪（零开销）。
func (s *Session) trackPartialStability(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deps.Speculator == nil || s.params.Mode == ModeDictation {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" || text == s.partialText {
		return
	}
	s.partialText = text
	s.partialGen++
	if s.specTimer != nil {
		s.specTimer.Stop()
	}
	gen := s.partialGen
	s.specTimer = time.AfterFunc(speculativeStabilityDelay, func() { s.fireSpeculation(gen) })
}

// fireSpeculation 稳定窗口到达：代际未变且仍处 listening 时触发投机意图。
// ctx 用会话 ctx（s.ctx）：连接拆除即取消在途投机 LLM 调用（final 不可达，
// 投机无消费者），传播 userID 供审计。
func (s *Session) fireSpeculation(gen int) {
	s.mu.Lock()
	if gen != s.partialGen || s.state != StateListening {
		s.mu.Unlock()
		return
	}
	text := s.partialText
	sp := s.deps.Speculator
	s.mu.Unlock()
	if sp == nil || text == "" {
		return
	}
	sp.SpeculateIntent(ctxuser.WithUserID(s.ctx, s.userID), s.sessionID, text)
	s.lg.Info("voice speculative intent triggered",
		loggateway.StepID("voice.intent.speculate"),
		loggateway.Int("text_len", len(text)))
}

// stopSpeculationLocked 停止 partial 稳定追踪（final/cancel/teardown 调用）。
// 调用方须持有 s.mu。已触发的投机槽不在此清理（由 resolve/取代/TTL 兜底）。
func (s *Session) stopSpeculationLocked() {
	if s.specTimer != nil {
		s.specTimer.Stop()
		s.specTimer = nil
	}
	s.partialText = ""
	s.partialGen++ // 使任何已派发的计时器回调失效
}

func (s *Session) handleASRFinal(ev biz.ASREvent) {
	text := strings.TrimSpace(ev.Text)
	if text == "" {
		return
	}
	// C2：终稿即语句边界，停止 partial 稳定追踪（投机槽由 resolve/取代/TTL 清理）。
	s.mu.Lock()
	s.stopSpeculationLocked()
	s.mu.Unlock()
	// 终稿 = 语句边界：取出并复位留档缓冲（V2-T6）。确认拦截/残余 final 等
	// 不进 Chat 管线的分支直接丢弃缓冲，保证下一句从空缓冲开始。
	pcm := s.takeUtteranceBuffer()
	_ = s.down.SendJSON(map[string]any{"type": "asr.final", "text": text, "duration_ms": ev.DurationMs})
	s.flow.LogDone("voice.asr.final", "语音识别终稿", event.P("duration_ms", ev.DurationMs))
	if s.dictation() {
		// 听写模式：终稿文本已下行，由前端填入输入框；不拦截确认词、
		// 不建 Chat Turn、不触发 TTS，状态停留 listening 连续听写。
		return
	}
	// V10 拦截顺序①：唤醒词剥离（连说形态「小媛，查天气」→ 净文本进 Chat 管线）。
	if stripped, hit := StripWakeWord(text); hit {
		if stripped == "" {
			return // listening 态重复单唤醒词：吞掉不建 Turn
		}
		text = stripped
	}
	// V10 拦截顺序②：退出词 → 自足应答后回待命（先于确认词，设计 §16.4①）。
	if MatchExitWord(text) {
		s.handleExitWord()
		return
	}
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
	s.turnT0 = time.Now() // E：首音频延迟测量 T0（首帧下行时消费）
	turnID := s.turnSeq
	params := s.params
	driver := s.asrDriver
	rate := s.sampleRate
	// M74 V9 评审竞态修复：listening 态存活的 scheduler 必为无主自足播报
	// （委派结果/失败残余——turn 的 scheduler 回 listening 前已被 OnDrained
	// 置空）。用户开口优先（barge-in 语义）：锁内摘除、锁外取消。否则其
	// flush 哨兵 OnDrained 会把 thinking 经 EvTTSEnd（无文本 Turn 合法出口）
	// 提前拍回 listening，新 turn delta 全丢 + tts.end 缺失。
	orphanSch := s.scheduler
	if orphanSch != nil {
		s.chunker = nil
		s.scheduler = nil
		s.ttsStarted = false
	}
	s.mu.Unlock()
	if orphanSch != nil {
		orphanSch.Cancel() // 锁外取消（wg.Wait 防与回调抢 s.mu 死锁，Cancel 先例）
	}
	voiceMeta := &biz.VoiceTurnMeta{
		ASRProvider: driver,
		DurationMs:  ev.DurationMs,
		Archive:     s.archiveUtterance(pcm, rate, ev.DurationMs),
	}
	s.broadcastState()
	_ = s.down.SendJSON(map[string]any{"type": "turn.accepted", "turn_id": turnRef(turnID)})
	// 与 WS 用户消息一致：turn 存活独立于连接（appctx），传播 userID。
	turnCtx := ctxuser.WithUserID(appctx.Ctx(), s.userID)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		// C2 L3 判定：final 文本与投机 partial 一致则注入产物复用
		// （允许有界等待在途投机完成；失配/超时/失败原样返回走常规意图路径）。
		if sp := s.deps.Speculator; sp != nil {
			turnCtx = sp.WithSpeculativeIntent(turnCtx, s.sessionID, text)
		}
		if err := s.executeTurnWithBusyRetry(turnCtx, ChatTurnInput{
			SessionID: s.sessionID, Content: text, AgentKey: params.AgentKey, TeamID: params.TeamID, Voice: voiceMeta,
		}); err != nil {
			s.handleTurnFailure(err)
		}
	}()
}

// archiveUtterance 将终稿语句 PCM 封装为 WAV 并送留档端口（V2-T6）。
// 返回 nil 表示无附件引用（端口未接线/开关关闭/无音频/失败降级 K3）。
// 同步执行于 asrPump：本地产物存储 + 单次 DB 写入，时延可忽略；
// 留档失败仅 Warn + 流程日志降级，不阻断 Turn 派发。
func (s *Session) archiveUtterance(pcm []byte, sampleRate, durationMs int) *artifactbiz.Ref {
	s.mu.Lock()
	archiver := s.deps.Archiver
	s.mu.Unlock()
	if archiver == nil || len(pcm) == 0 {
		return nil
	}
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	// 与 Chat Turn 一致：留档存活独立于连接（appctx），传播 userID。
	ctx := ctxuser.WithUserID(appctx.Ctx(), s.userID)
	ref, err := archiver.SaveUtteranceAudio(ctx, s.sessionID, EncodeWAV(pcm, sampleRate), durationMs)
	if err != nil {
		s.lg.Warn("voice archive failed, turn continues without attachment",
			loggateway.StepID("voice.archive.degraded"), loggateway.Err(err))
		s.flow.LogWarn("voice.archive.degraded", "语音留档降级", "留档失败，消息正常派发", event.P("error", err.Error()))
		return nil
	}
	if ref.ID == "" {
		return nil // 开关关闭（实现内部判定）
	}
	s.flow.LogDone("voice.archive.saved", "语音留档保存",
		event.P("duration_ms", durationMs), event.P("size", ref.Size), event.P("artifact_id", ref.ID))
	return &ref
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
			s.routeEvent(e)
		}
	}
}

// routeEvent 事件三路分流（M74 V9 组件 D，评审 R3）：V2Bus.Subscribe 忽略
// EventSubscribeOptions 过滤参数（全量广播），必须显式按归属分流——
// ① SpiritSessionID()==本会话 → 快答路径（现状）；② 本会话委派的精灵任务
// 事件 → task 绑定/终态播报；③ 其余一律丢弃（精灵后台执行事件不得串入
// 语音 TTS）。
func (s *Session) routeEvent(e biz.Event) {
	if e == nil {
		return
	}
	sid := e.SpiritSessionID()
	if sid == s.sessionID {
		switch ev := e.(type) {
		case *biz.StepStreamingEvent:
			if ev.DeltaField == "content" && ev.DeltaChunk != "" {
				s.feedDelta(ev.DeltaChunk)
			}
		case *biz.StepCreatedEvent:
			s.maybeBroadcastClarification(ev)
		case *biz.TurnCompletedEvent:
			s.handleTurnCompleted()
		case *biz.TurnFailedEvent:
			s.handleTurnFailure(nil)
		}
		return
	}
	reg := s.deps.Delegation
	if reg == nil || sid == "" {
		return
	}
	switch ev := e.(type) {
	case *biz.TaskCreatedEvent:
		// 内容匹配绑定 taskID（R10）：先注册后提交无漏绑窗口，内容精确匹配
		// 免疫外来并发 turn 错绑。多语音会话并发时先到先得，绑定结果幂等。
		owner, ok := reg.BindTask(sid, ev.Task.UserMessage, ev.TaskID())
		if ok && owner == s.sessionID {
			s.flow.LogDone("voice.delegation.bind", "委派任务绑定", event.P("task_id", ev.TaskID()))
			s.lg.Info("voice delegation task bound",
				loggateway.StepID("voice.delegation.bind"),
				loggateway.Str("task_id", ev.TaskID()))
		}
	case *biz.TaskCompletedEvent:
		// owner 限定消费（CompleteTask 第一参）：非本会话委派不得截胡。
		if entry, ok := reg.CompleteTask(s.sessionID, sid, ev.TaskID()); ok {
			s.handleDelegationTerminal(entry, ev.Task.Status, sid)
		}
	case *biz.TaskFailedEvent:
		if entry, ok := reg.CompleteTask(s.sessionID, sid, ev.TaskID()); ok {
			s.handleDelegationTerminal(entry, biz.TaskStatusFailed, sid)
		}
	}
}

// handleDelegationTerminal 委派任务终态：组织口播文本并入队/播报（K5 双轨）。
// TaskCompletedEvent 含 cancelled（Task.Status 区分，§15.2 R9）。
func (s *Session) handleDelegationTerminal(entry DelegationEntry, status biz.TaskStatus, spiritSessionID string) {
	var text string
	switch status {
	case biz.TaskStatusCompleted:
		if reply := s.delegationReplyText(spiritSessionID); reply != "" {
			text = "精灵助手来回复了。" + reply
		} else {
			text = "精灵助手的任务已完成，详细结果请在聊天窗口查看。"
		}
	case biz.TaskStatusCancelled:
		text = "交给精灵助手的任务已取消。"
	default:
		text = "交给精灵助手的任务未能完成，详细情况请在聊天窗口查看。"
	}
	s.flow.LogDone("voice.delegation.terminal", "委派任务终态",
		event.P("task_id", entry.TaskID), event.P("status", string(status)))
	s.lg.Info("voice delegation task terminal",
		loggateway.StepID("voice.delegation.terminal"),
		loggateway.Str("task_id", entry.TaskID), loggateway.Str("status", string(status)))
	s.enqueueDelegationSpeech(text)
}

// delegationReplyText 取精灵会话最新 completed reply step 全文（R7）。
// 读失败/无回复返回空串（调用方降级为简报，K3）。
func (s *Session) delegationReplyText(spiritSessionID string) string {
	r := s.deps.DelegationSteps
	if r == nil || spiritSessionID == "" {
		return ""
	}
	// 与 Turn 派发一致：读取存活独立于连接（appctx），传播 userID。
	ctx, cancel := context.WithTimeout(ctxuser.WithUserID(appctx.Ctx(), s.userID), 5*time.Second)
	defer cancel()
	steps, err := r.ListStepsBySessionID(ctx, spiritSessionID)
	if err != nil {
		s.lg.Warn("voice delegation reply read failed, broadcast brief (K3)",
			loggateway.StepID("voice.delegation.reply_read_fail"), loggateway.Err(err))
		return ""
	}
	for i := len(steps) - 1; i >= 0; i-- {
		st := steps[i]
		if st.Kind == biz.StepKindReply && st.Status == biz.StepStatusCompleted {
			return strings.TrimSpace(st.Content)
		}
	}
	return ""
}

// enqueueDelegationSpeech 委派播报入口：listening 空闲即播；正忙
// （thinking/speaking/interrupted 或 turn 在途）入 session FIFO，回
// listening 排空（§15.3「用户说话优先；不打断进行中的问答」）。
func (s *Session) enqueueDelegationSpeech(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	if st == StateDormant {
		// G1（设计 §16.4③）：dormant 委派终态系统唤醒——source=system
		// 无「我在」应答，委派播报本身即内容；唤醒落 listening 后走下方
		// 空闲即播，播报完按 SleepTimer 规则回 dormant。
		s.Wake("system")
	}
	s.mu.Lock()
	idle := s.state == StateListening && s.pendingTurns == 0
	if !idle {
		s.delegationOutbox = append(s.delegationOutbox, text)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	s.speakDelegation(text)
}

// speakDelegation 自足播报（R15）：委派结果/失败通知（流程日志 voice.delegation.broadcast）。
func (s *Session) speakDelegation(text string) {
	s.speakSelfSufficient(text, "voice.delegation.broadcast", "委派结果播报")
}

// speakSelfSufficient 自足播报（R15 机制泛化，V10 唤醒/退出应答复用）：
// listening 态无活跃 turn/chunker flush 源，必须一次性 ensureTTS → Write →
// Flush → flush 哨兵（复用 handleTurnCompleted 哨兵逻辑驱动 OnDrained →
// tts.end）。状态停留 listening：TTS 播放期间用户说话照常进 ASR。
func (s *Session) speakSelfSufficient(text, stepID, title string) {
	if err := s.ensureTTS(); err != nil {
		s.sendError("TTS_UNAVAILABLE", err, true)
		return
	}
	s.mu.Lock()
	// 自足路径无 turn 起点复位（handleASRFinal），显式清零防陈旧 true 跳哨兵。
	s.flushEnqueued = false
	ch := s.chunker
	s.mu.Unlock()
	if ch == nil {
		return
	}
	s.flow.LogDone(stepID, title, event.P("chars", len(text)))
	s.lg.Info("voice self-sufficient speech",
		loggateway.StepID(stepID), loggateway.Int("chars", len(text)))
	ch.Write(text)
	s.flushTTSTail()
}

// handleExitWord V10：退出词命中 —— 自足 TTS 应答确认，播报完转 dormant
// （onTTSDrained 消费 sleepAfterDrain）；TTS 不可用降级为立即休眠（不阻塞）。
func (s *Session) handleExitWord() {
	s.mu.Lock()
	if s.state != StateListening {
		s.mu.Unlock()
		return // 残余 final（竞态已离 listening），忽略
	}
	s.mu.Unlock()
	// 流程日志统一在 enterDormant 发射（应答后/TTS 降级两路径各一次，此处不重复）。
	s.lg.Info("voice session exit word matched", loggateway.StepID("voice.sleep.exit_word"))
	if err := s.ensureTTS(); err != nil {
		s.sendError("TTS_UNAVAILABLE", err, true)
		s.enterDormant(EvExitWord, "voice.sleep.exit_word", "退出词休眠（TTS 降级直休眠）")
		return
	}
	s.mu.Lock()
	s.sleepAfterDrain = true
	s.mu.Unlock()
	s.speakSelfSufficient(exitAckText, "voice.sleep.exit_word", "退出词应答")
}

// enterDormant listening → dormant 公共路径：转换 + 关 ASR 上游 + 广播。
// 仅 onSleepTimeout / onTTSDrained（退出词应答后）/ handleExitWord 降级调用。
func (s *Session) enterDormant(ev VoiceEvent, stepID, msg string) {
	if err := s.transition(ev); err != nil {
		return
	}
	s.closeASRUpstream()
	s.flow.LogDone(stepID, msg)
	s.lg.Info("voice session dormant", loggateway.StepID(stepID))
	s.broadcastState()
}

// drainDelegationOutbox 回 listening 时排空委派播报 FIFO：一次播一条，本次
// 播报的 OnDrained 再触发下一条（串联播报，防多条叠音）。
func (s *Session) drainDelegationOutbox() {
	s.mu.Lock()
	if s.state != StateListening || s.pendingTurns > 0 || len(s.delegationOutbox) == 0 {
		s.mu.Unlock()
		return
	}
	text := s.delegationOutbox[0]
	s.delegationOutbox = s.delegationOutbox[1:]
	s.mu.Unlock()
	s.speakDelegation(text)
}

// onDelegationNotice registry 带外通知（watcher 回调）：提交同步失败等不产生
// 总线事件的路径，口播失败原因防用户空等（R12）。回调自工具 detached
// goroutine 触发，线程安全由 s.mu 保证。
func (s *Session) onDelegationNotice(n DelegationNotice) {
	if n.Kind != NoticeDelegationSubmitFailed {
		return
	}
	s.enqueueDelegationSpeech(n.Message)
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

// maybeBroadcastClarification 澄清门触发（step.created kind=clarify）时把问题
// 口播给用户（F2）：澄清卡片只在 UI 呈现，语音会话收不到任何文本 delta，
// turn 挂起后用户会面对静默。此处将信封问题渲染为口播文本喂给 TTS；随后的
// TurnCompleted 走既有 flush/drain 路径收尾回 listening。用户语音作答经
// 自由文本澄清路径续跑（service.resolveClarificationFreeText，与 WS 文字
// 消息同一入口 Execute），语音侧无需特判。
func (s *Session) maybeBroadcastClarification(ev *biz.StepCreatedEvent) {
	if ev == nil || ev.Step.Kind != biz.StepKindClarify {
		return
	}
	var env biz.ClarificationEnvelope
	if err := json.Unmarshal([]byte(ev.Step.Content), &env); err != nil {
		s.lg.Warn("voice clarify: envelope parse failed, skip broadcast",
			loggateway.StepID("voice.clarify.parse_fail"), loggateway.Err(err))
		return
	}
	text := clarificationSpeech(&env)
	if text == "" {
		return
	}
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	if st != StateListening && st != StateThinking && st != StateSpeaking {
		return // idle/interrupted/error 不播报
	}
	if err := s.ensureTTS(); err != nil {
		s.sendError("TTS_UNAVAILABLE", err, true)
		return
	}
	s.flow.LogDone("voice.clarify.broadcast", "澄清问题播报", event.P("questions", len(env.Questions)))
	s.lg.Info("voice clarify questions broadcast",
		loggateway.StepID("voice.clarify.broadcast"),
		loggateway.Any("question_count", len(env.Questions)))
	s.mu.Lock()
	ch := s.chunker
	s.mu.Unlock()
	if ch != nil {
		ch.Write(text)
	}
}

// cnNumerals 口播中文数字（问题序号）。
var cnNumerals = []string{"零", "一", "二", "三", "四", "五", "六", "七", "八", "九"}

func cnOrdinal(n int) string {
	if n >= 1 && n <= 9 {
		return cnNumerals[n]
	}
	return itoa(n)
}

// cnCount 计数用中文数字（2 → 两，口播习惯）。
func cnCount(n int) string {
	if n == 2 {
		return "两"
	}
	return cnOrdinal(n)
}

// clarificationSpeech 把澄清信封渲染为口播文本：引导语 + 逐题（题干 + 可选项）
// + 作答引导。文本含充足句读，供 SentenceChunker 按句切分。
func clarificationSpeech(env *biz.ClarificationEnvelope) string {
	qs := make([]biz.ClarificationQuestion, 0, len(env.Questions))
	for _, q := range env.Questions {
		if strings.TrimSpace(q.Question) != "" {
			qs = append(qs, q)
		}
	}
	if len(qs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("继续之前，需要确认" + cnCount(len(qs)) + "个问题。")
	for i, q := range qs {
		if len(qs) > 1 {
			b.WriteString("第" + cnOrdinal(i+1) + "个，")
		}
		qt := strings.TrimSpace(q.Question)
		b.WriteString(qt)
		if !strings.HasSuffix(qt, "？") && !strings.HasSuffix(qt, "。") && !strings.HasSuffix(qt, "！") &&
			!strings.HasSuffix(qt, "?") && !strings.HasSuffix(qt, ".") && !strings.HasSuffix(qt, "!") {
			b.WriteString("。")
		}
		if len(q.Options) > 0 {
			b.WriteString("可选：" + strings.Join(q.Options, "、") + "。")
		}
	}
	b.WriteString("请直接说出你的回答。")
	return b.String()
}

func (s *Session) ensureTTS() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.chunker != nil {
		return nil
	}
	// L5：优先复用 Start 预解析的 provider（同次解析的会话参数）；
	// 未预解析/解析失败时回退工厂调用（原错误路径不变）。
	provider, cfg := s.ttsProvider, s.ttsCfg
	if provider == nil {
		var err error
		provider, cfg, err = s.deps.NewTTS(s.ctx)
		if err != nil {
			return err
		}
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

// resolveAndPrewarmTTS L5：voice.start 预解析 TTS provider+cfg 存入会话
// （ensureTTS 复用，Turn 内首个文本 delta 不再二次调工厂），并后台预拨一条
// WS 连接（provider 支持 ttsConnPrewarmer 时），首个 Turn 首句免握手。
// 非阻断容错（K3）：解析失败仅 Warn，ensureTTS 回退工厂调用（原错误路径
// 报 TTS_UNAVAILABLE）；provider 无预热能力时静默跳过。
func (s *Session) resolveAndPrewarmTTS() {
	provider, cfg, err := s.deps.NewTTS(s.ctx)
	if err != nil {
		s.lg.Warn("voice tts pre-resolve failed, lazy retry on first delta (K3)",
			loggateway.StepID("voice.tts.preresolve_fail"), loggateway.Err(err))
		return
	}
	s.mu.Lock()
	s.ttsProvider = provider
	s.ttsCfg = cfg
	s.mu.Unlock()
	pw, ok := provider.(ttsConnPrewarmer)
	if !ok {
		return
	}
	s.wg.Add(1)
	safego.Go(s.ctx, "voice.tts_prewarm", func() {
		defer s.wg.Done()
		pw.PrewarmTTSConn(s.ctx)
	})
	s.lg.Info("voice tts conn prewarm triggered", loggateway.StepID("voice.tts.prewarm"))
}

func (s *Session) handleTurnCompleted() {
	s.mu.Lock()
	if s.pendingTurns > 0 {
		s.pendingTurns--
	}
	idleNoText := s.scheduler == nil && s.state == StateThinking
	s.mu.Unlock()
	s.flushTTSTail()
	if idleNoText {
		// 无文本 Turn（纯工具调用等）：thinking --tts_end--> listening（设计 §5）
		if err := s.transition(EvTTSEnd); err == nil {
			s.broadcastState()
		}
	}
	s.drainDelegationOutbox()
}

// flushTTSTail Flush chunker 残余并补 flush 哨兵（尾句已在句边界全部切出、
// Flush 无残余时驱动 OnDrained，否则 tts.end 缺失、状态停在 speaking）。
// handleTurnCompleted 与委派自足播报（speakDelegation）共用。
func (s *Session) flushTTSTail() {
	s.mu.Lock()
	ch := s.chunker
	sch := s.scheduler
	s.mu.Unlock()
	if ch != nil {
		ch.Flush()
	}
	if sch == nil {
		return
	}
	s.mu.Lock()
	tailSent := s.flushEnqueued
	s.mu.Unlock()
	if !tailSent {
		if err := sch.Enqueue(s.ctx, "", true); err != nil {
			s.lg.Warn("voice tts flush sentinel enqueue failed", loggateway.StepID("voice.tts.enqueue_fail"), loggateway.Err(err))
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
	s.drainDelegationOutbox()
}

// ---- 内部：TTS 回调 ----

func (s *Session) onTTSAudio(pcm []byte) {
	s.mu.Lock()
	first := !s.ttsStarted
	if first {
		s.ttsStarted = true
	}
	t0 := s.turnT0
	if first {
		s.turnT0 = time.Time{} // 消费 T0：每个 Turn 只测一次首音频延迟
	}
	s.mu.Unlock()
	if first {
		_ = s.down.SendJSON(map[string]any{"type": "tts.start", "encoding": "pcm_f32le_16k", "sample_rate": 16000})
		// E：首音频延迟（ASR 终稿 → 首帧下行），超预算 Warn（K2）。
		if !t0.IsZero() {
			ms := time.Since(t0).Milliseconds()
			s.flow.LogDone("voice.tts.start", "语音播报开始", event.P("first_audio_ms", ms))
			if time.Duration(ms)*time.Millisecond > firstAudioBudget {
				s.lg.Warn("voice turn first audio over budget",
					loggateway.StepID("voice.turn.first_audio"),
					loggateway.Any("first_audio_ms", ms),
					loggateway.Any("budget_ms", firstAudioBudget.Milliseconds()))
			} else {
				s.lg.Info("voice turn first audio",
					loggateway.StepID("voice.turn.first_audio"),
					loggateway.Any("first_audio_ms", ms))
			}
		} else {
			s.flow.LogDone("voice.tts.start", "语音播报开始")
		}
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
	sleep := s.sleepAfterDrain
	s.sleepAfterDrain = false
	s.mu.Unlock()
	_ = s.down.SendJSON(map[string]any{"type": "tts.end"})
	s.flow.LogDone("voice.tts.end", "语音播报结束")
	if sleep {
		// V10：退出词应答播报完 → dormant（设计 §16.4②，跳过 EvTTSEnd/drain）。
		s.enterDormant(EvExitWord, "voice.sleep.exit_word", "退出词休眠")
		return
	}
	if err := s.transition(EvTTSEnd); err == nil {
		s.broadcastState()
	}
	s.drainDelegationOutbox()
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
	s.manageSleepTimerLocked(to)
	return nil
}

// manageSleepTimerLocked V10：companion 静默休眠计时集中管理（设计 §16.4②）。
// 进入 listening 重置 60s；离开 listening 停止（thinking/speaking 交互中不休眠，
// dormant/idle 无需计时）。听写/非 companion 模式无 dormant 语义，恒不计时。
// 调用方须持有 s.mu。
func (s *Session) manageSleepTimerLocked(to VoiceState) {
	if s.sleepTimer != nil {
		s.sleepTimer.Stop()
		s.sleepTimer = nil
	}
	if to != StateListening || s.params.Mode != ModeCompanion {
		return
	}
	s.sleepTimer = time.AfterFunc(sleepTimeout, s.onSleepTimeout)
}

// resetSleepTimer V10：ASR 活动（partial 流）重置静默休眠计时——用户持续
// 说话时（state 停留 listening 无转换）不被 60s 到期误休眠。
func (s *Session) resetSleepTimer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StateListening || s.params.Mode != ModeCompanion {
		return
	}
	if s.sleepTimer != nil {
		s.sleepTimer.Stop()
	}
	s.sleepTimer = time.AfterFunc(sleepTimeout, s.onSleepTimeout)
}

// onSleepTimeout V10：静默到期 → dormant（关闭 ASR 上游，零占用）。
// 竞态窗口内已离开 listening（交互中）时转换表拒绝即忽略。
func (s *Session) onSleepTimeout() {
	if err := s.transition(EvSleepTimeout); err != nil {
		return
	}
	s.closeASRUpstream()
	s.flow.LogDone("voice.sleep.timeout", "静默休眠")
	s.lg.Info("voice session dormant (sleep timeout)", loggateway.StepID("voice.sleep.timeout"))
	s.broadcastState()
}

// closeASRUpstream 关闭并摘除当前 ASR 上游（dormant 零占用；asrPump 的
// 流终结 CAS 摘表发现已摘除后不重复 Close）。
func (s *Session) closeASRUpstream() {
	s.mu.Lock()
	asr := s.asr
	s.asr = nil
	s.mu.Unlock()
	if asr != nil {
		_ = asr.Close()
	}
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
	s.stopSpeculationLocked() // C2：拆除即停止 partial 稳定追踪
	asr := s.asr
	s.asr = nil
	sch := s.scheduler
	s.scheduler = nil
	s.chunker = nil
	ttsProv := s.ttsProvider
	s.ttsProvider = nil
	s.ttsCfg = biz.TTSSessionConfig{}
	s.utterBuf = nil
	s.utterOverflow = false
	unsub := s.unsub
	s.unsub = nil
	s.mu.Unlock()
	if asr != nil {
		_ = asr.Close()
	}
	if sch != nil {
		sch.Cancel()
	}
	// L5：释放未消费的 TTS 温连接（已消费则槽空 no-op）。
	if pw, ok := ttsProv.(ttsConnPrewarmer); ok {
		pw.ReleaseWarmTTSConn()
	}
	// M74 V9：会话拆除即清委派条目与 watcher（进程内委派跟随会话生命周期）。
	if reg := s.deps.Delegation; reg != nil {
		reg.ClearVoiceSession(s.sessionID)
	}
	if unsub != nil {
		unsub()
	}
}
