// TECH-DEBT(COG): file_lines=981, 上限=500 — 待拆分为 cascade/llm/tx 子文件。
package session

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/compress"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/llmcontext"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

// MemoryResync clears derived memory entities after snapshot rewrite.
type MemoryResync interface {
	DeleteSessionEventEntities(ctx context.Context, sessionID string) error
}

type AgentKeyLookup interface {
	GetAgentByID(ctx context.Context, id string) (biz.Agent, error)
}

// CompressReadDeps groups read-only data access interfaces for compression.
type CompressReadDeps interface {
	biz.SessionReader
	biz.MessageReader
	biz.SummaryReader
}

// CompressWriteDeps groups write data access interfaces for compression.
type CompressWriteDeps interface {
	biz.MessageWriter
	biz.SummaryWriter
	biz.ContextUpdater
}

// CompressTxDeps provides transaction and CAS support for compression.
type CompressTxDeps interface {
	biz.CompressRepo
}

// CompressorConfig holds the dependencies for constructing a Compressor.
type CompressorConfig struct {
	ReadDeps     CompressReadDeps
	WriteDeps    CompressWriteDeps
	TxDeps       CompressTxDeps
	Agents       AgentKeyLookup
	Runtime      *Runtime
	Memory       MemoryResync
	Compress     compress.Compressor
	MonitorBus   contract.MonitorBus
	MemoryReader biz.MemoryFactReader
	L1Reader     biz.L1AdminReader
	// L1BoardWriter 可选：非空时压缩产出的 TaskState 回写 L1 task_board，
	// 使 L1 prompt cue 与快照注入渲染同一份进度（单一权威源）。
	L1BoardWriter biz.L1TaskBoardWriter
	Logger        loggateway.Logger
	// Collector 是 M80 决策记录入口（P1-④，2026-08-30）：每次真实压缩
	// 尝试（成功/失败）写 system_guard 决策记录（trigger=context_compacted）。
	// nil = 决策记录静默降级（单测/精简装配）。
	Collector decision.Collector
}

// compressLevel identifies which compression tier was used.
type compressLevel string

const (
	compressLevelNone   compressLevel = "none"
	compressLevelMemory compressLevel = "memory_compact"
	compressLevelLLM    compressLevel = "llm_compact"
)

// compressFailureKind classifies a failed compression attempt (Phase 3 wires
// the real classification logic).
type compressFailureKind int

const (
	compressFailureNone compressFailureKind = iota
	compressFailureTransient
	compressFailureDeterministic
)

// compressOutcome carries the cascade result: which level produced markdown,
// whether prior summaries were absorbed (LLM recursive merge), and the failure
// kind when nothing was produced.
type compressOutcome struct {
	level          compressLevel
	markdown       string
	absorbedPriors bool
	fail           compressFailureKind
	// taskState 是 v4 压缩契约的结构化任务状态段（叙事摘要之外的双段化产物），
	// 由 compress.ExtractTaskState 从 LLM 产出末尾的 task_state 块拆出；nil = 无。
	taskState *biz.TaskState
	// cacheHit reports whether the LLM result came from the compression cache
	// (no live LLM call). Surfaced in the compression notice metadata.
	cacheHit bool
}

// cacheHitAwareCompressor is an optional capability of compress.Compressor
// implementations that report whether a call was served from cache.
type cacheHitAwareCompressor interface {
	CompressWithCacheHit(ctx context.Context, req compress.Request) (compress.Result, bool, error)
}

// compressDeps groups the data-access dependencies for the Compressor.
// Extracted from Compressor to reduce field count (AS-COG-01).
type compressDeps struct {
	sessionReader  biz.SessionReader
	messageReader  biz.MessageReader
	messageWriter  biz.MessageWriter
	summaryReader  biz.SummaryReader
	summaryWriter  biz.SummaryWriter
	contextUpdater biz.ContextUpdater
	compressRepo   biz.CompressRepo
}

type Compressor struct {
	deps          compressDeps
	agents        AgentKeyLookup
	Runtime       *Runtime
	Compress      compress.Compressor
	Memory        MemoryResync
	monitorBus    contract.MonitorBus
	memoryReader  biz.MemoryFactReader
	l1Reader      biz.L1AdminReader
	l1BoardWriter biz.L1TaskBoardWriter
	lg            loggateway.Logger
	// collector 是 M80 决策记录入口（P1-④）：emitCompressDecision 经此写
	// system_guard 决策记录；nil 时静默降级。
	collector decision.Collector

	flight   *compressFlightManager
	buf      *compressBufferManager
	suppress *compressSuppressManager
}

// compressFlightManager manages per-session in-flight deduplication and the
// global compressing CAS lock. Extracted from Compressor to reduce field count
// and eliminate sync.Map at the Compressor level (AS-COG-01).
type compressFlightManager struct {
	inFlight        sync.Map // map[sessionID]bool
	compressing     atomic.Bool
	compressStart   time.Time
	compressMu      sync.Mutex
	compressTimeout time.Duration
}

func newCompressFlightManager() *compressFlightManager {
	return &compressFlightManager{
		compressTimeout: defaultCompressTimeout,
	}
}

// markInFlight attempts to mark a session as in-flight. Returns true if this
// caller won the race (session was not already in-flight).
func (f *compressFlightManager) markInFlight(sessionID string) bool {
	_, loaded := f.inFlight.LoadOrStore(sessionID, true)
	return !loaded
}

// markDone removes the in-flight mark for a session.
func (f *compressFlightManager) markDone(sessionID string) {
	f.inFlight.Delete(sessionID)
}

// tryStartCompress attempts to mark the compressor as active. Returns true if
// this caller won the CAS race. Includes timeout auto-release to prevent stuck flags.
func (f *compressFlightManager) tryStartCompress(sessionID string) bool {
	f.compressMu.Lock()
	defer f.compressMu.Unlock()
	// Timeout auto-release: prevent stuck flag
	if f.compressing.Load() && time.Since(f.compressStart) > f.compressTimeout {
		f.compressing.Store(false)
	}
	if f.compressing.Load() {
		return false
	}
	if f.compressing.CompareAndSwap(false, true) {
		f.compressStart = time.Now()
		return true
	}
	return false
}

func (f *compressFlightManager) finishCompress() {
	f.compressing.Store(false)
}

func (f *compressFlightManager) isCompressing() bool {
	return f.compressing.Load()
}

// compressBufferManager manages per-session adaptive buffer state and its
// background GC goroutine. Extracted from Compressor to reduce field count
// and eliminate sync.Map at the Compressor level (AS-COG-01).
type compressBufferManager struct {
	buffer   sync.Map // map[sessionID]*AdaptiveBufferState
	gcCancel chan struct{}
}

func newCompressBufferManager() *compressBufferManager {
	return &compressBufferManager{
		gcCancel: make(chan struct{}),
	}
}

func (b *compressBufferManager) getAdaptiveBufferRatio(sessionID string, ag biz.Agent, usedTokens, contextWindow int, toolCallCount, turnCount int) float64 {
	initialRatio := compressionBufferRatio(ag)
	val, loaded := b.buffer.LoadOrStore(sessionID, NewAdaptiveBufferState(initialRatio))
	state := val.(*AdaptiveBufferState)
	if !loaded {
		// First call for this session — seed LastUsedTokens so the first increment
		// is measured from the current usedTokens rather than from zero.
		state.LastUsedTokens = usedTokens
		return state.CurrentRatio
	}
	mode := DetectConversationMode(toolCallCount, turnCount)
	return state.UpdateAdaptiveBuffer(usedTokens, contextWindow, mode)
}

func (b *compressBufferManager) removeSessionState(sessionID string) {
	b.buffer.Delete(sessionID)
}

func (b *compressBufferManager) startGC() {
	safego.Go(appctx.Ctx(), "compressor-gc", b.gcLoop)
}

func (b *compressBufferManager) stopGC() {
	select {
	case <-b.gcCancel:
	default:
		close(b.gcCancel)
	}
}

func (b *compressBufferManager) gcLoop() {
	ticker := time.NewTicker(adaptiveBufferGCInterval)
	defer ticker.Stop()
	for {
		select {
		case <-b.gcCancel:
			return
		case <-ticker.C:
			b.gcAdaptiveBuffer()
		}
	}
}

func (b *compressBufferManager) gcAdaptiveBuffer() {
	now := time.Now()
	b.buffer.Range(func(key, value any) bool {
		if state, ok := value.(*AdaptiveBufferState); ok {
			if now.Sub(state.LastAccessed()) > adaptiveBufferMaxAge {
				b.buffer.Delete(key)
			}
		}
		return true
	})
}

var _ biz.NativeTurnCompressor = (*Compressor)(nil)
var _ biz.DurableTurnCompressor = (*Compressor)(nil)
var _ biz.ManualCompressor = (*Compressor)(nil)

type preserveInstructionKey struct{}

func containsMemoryCompactMarker(md string) bool {
	return strings.Contains(md, "## Session Memory Summary")
}

func NewCompressor(cfg CompressorConfig) *Compressor {
	c := &Compressor{
		deps: compressDeps{
			sessionReader:  cfg.ReadDeps,
			messageReader:  cfg.ReadDeps,
			summaryReader:  cfg.ReadDeps,
			messageWriter:  cfg.WriteDeps,
			summaryWriter:  cfg.WriteDeps,
			contextUpdater: cfg.WriteDeps,
			compressRepo:   cfg.TxDeps,
		},
		agents:        cfg.Agents,
		Runtime:       cfg.Runtime,
		Memory:        cfg.Memory,
		Compress:      cfg.Compress,
		monitorBus:    cfg.MonitorBus,
		memoryReader:  cfg.MemoryReader,
		l1Reader:      cfg.L1Reader,
		l1BoardWriter: cfg.L1BoardWriter,
		lg:            cfg.Logger,
		collector:     cfg.Collector,
		flight:        newCompressFlightManager(),
		buf:           newCompressBufferManager(),
		suppress:      newCompressSuppressManager(),
	}
	c.buf.startGC()
	return c
}

func (c *Compressor) AfterNativeTurn(ctx context.Context, sessionID string, ag biz.Agent) {
	if c == nil || c.deps.sessionReader == nil || c.Compress == nil {
		return
	}
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return
	}
	trpcUserID := TRPCUserKey(ctx)
	safego.Go(ctx, "session-compress", func() {
		if !c.flight.markInFlight(sid) {
			return
		}
		defer c.flight.markDone(sid)
		runCtx, cancel := context.WithTimeout(context.Background(), compressRunTimeout)
		defer cancel()
		if err := c.runCompress(runCtx, sid, trpcUserID, ag, false); err != nil {
			c.lg.Warn("会话压缩失败", loggateway.StepID("session.compress"), loggateway.SessionID(sid), loggateway.Err(err))
		}
	})
}

func (c *Compressor) BeforeDurableTurn(ctx context.Context, sessionID string, ag biz.Agent) error {
	if c == nil || c.deps.sessionReader == nil || c.Compress == nil {
		return nil
	}
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return nil
	}
	if !c.flight.markInFlight(sid) {
		return nil
	}
	defer c.flight.markDone(sid)
	runCtx, cancel := context.WithTimeout(ctx, compressRunTimeout)
	defer cancel()
	if err := c.runCompress(runCtx, sid, TRPCUserKey(ctx), ag, true); err != nil {
		c.lg.Warn("Durable turn 前压缩失败", loggateway.StepID("session.compress"), loggateway.SessionID(sid), loggateway.Err(err))
	}
	return nil
}

func (c *Compressor) CompactSession(ctx context.Context, sessionID string, preserveInstruction string) (*biz.CompactResult, error) {
	if c == nil || c.deps.sessionReader == nil || c.Compress == nil {
		return nil, nil
	}
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return nil, apierror.BadRequest(apierror.DomainSession, "session_id is required")
	}
	if !c.flight.markInFlight(sid) {
		return nil, apierror.BadRequest(apierror.DomainSession, "compression already in progress for this session")
	}
	defer c.flight.markDone(sid)

	sess, err := c.deps.sessionReader.GetSessionByID(ctx, sid)
	if err != nil {
		return nil, err
	}
	ag, err := c.agents.GetAgentByID(ctx, sess.AgentID)
	if err != nil {
		return nil, err
	}
	if !sessionCompressEnabled(ag) {
		return &biz.CompactResult{Compacted: false}, nil
	}

	estBefore := sess.ContextUsedTokens

	runCtx, cancel := context.WithTimeout(ctx, compressRunTimeout)
	defer cancel()

	if preserveInstruction != "" {
		runCtx = context.WithValue(runCtx, preserveInstructionKey{}, preserveInstruction)
	}

	err = c.runCompress(runCtx, sid, TRPCUserKey(ctx), ag, true)
	if err != nil {
		return nil, err
	}

	sessAfter, err := c.deps.sessionReader.GetSessionByID(ctx, sid)
	if err != nil {
		return &biz.CompactResult{Compacted: true, EstimatedTokensBefore: estBefore, EstimatedTokensAfter: estBefore}, nil
	}

	level := "auto_compact"
	fromTurn, toTurn := 0, 0
	if summaries, sErr := c.deps.summaryReader.ListSessionSummaries(ctx, sid); sErr == nil && len(summaries) > 0 {
		latest := summaries[len(summaries)-1]
		fromTurn = latest.FromTurn
		toTurn = latest.ToTurn
		if containsMemoryCompactMarker(latest.SummaryMarkdown) {
			level = "memory_compact"
		}
	}

	return &biz.CompactResult{
		Compacted:             true,
		FromTurn:              fromTurn,
		ToTurn:                toTurn,
		EstimatedTokensBefore: estBefore,
		EstimatedTokensAfter:  sessAfter.ContextUsedTokens,
		CompressionLevel:      level,
	}, nil
}

// CompressStatus returns the current compression status for a session.
func (c *Compressor) CompressStatus(ctx context.Context, sessionID string) (string, error) {
	if c.flight.isCompressing() {
		return "compressing", nil
	}
	sess, err := c.deps.sessionReader.GetSessionByID(ctx, sessionID)
	if err != nil {
		return "normal", err
	}
	// Check if there's a recent summary
	if ts, err := c.deps.summaryReader.LatestSessionSummaryTime(ctx, sessionID); err == nil && ts != "" {
		if t, err := time.Parse(time.RFC3339, ts); err == nil && time.Since(t) < recentlyOptimizedWindow {
			return "optimized", nil
		}
	}
	window := llmcontext.ResolveWindow(llmcontext.ResolveInput{})
	if window <= 0 {
		return "normal", nil
	}
	ag, _ := c.agents.GetAgentByID(ctx, sess.AgentID)
	softTok := softTriggerTokens(ag, window)
	if sess.ContextUsedTokens >= softTok {
		return "optimizing", nil
	}
	return "normal", nil
}

// messagesPerTurn represents the typical number of message rows per turn (user + assistant).
const messagesPerTurn = 2

// recentlyOptimizedWindow is the time window after which a summary is no longer considered "recent".
const recentlyOptimizedWindow = 2 * time.Minute

// defaultCompressTimeout is the maximum duration a compression operation can take before auto-release.
const defaultCompressTimeout = 10 * time.Minute

func (c *Compressor) runCompress(ctx context.Context, sessionID, trpcUserID string, ag biz.Agent, forced bool) error {
	if !sessionCompressEnabled(ag) {
		return nil
	}
	// 缓存键需要 sessionID 隔离（自动压缩路径的 ctx 未携带）；per-agent 开关关闭时
	// 完全绕过缓存（不读不写）。
	ctx = compress.ContextWithSessionID(ctx, sessionID)
	if !compressLLMCacheEnabled(ag) {
		ctx = compress.ContextWithCacheDisabled(ctx)
	}
	sess, err := c.deps.sessionReader.GetSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}

	// Calculate effective_budget thresholds.
	window := llmcontext.ResolveWindow(llmcontext.ResolveInput{})

	usedTokens := sess.ContextUsedTokens

	// Determine adaptive or static buffer ratio and compute trigger tokens.
	softTok, hardTok := c.compressTriggerThresholds(sessionID, sess, ag, usedTokens, window)

	// Below soft trigger: nothing to do.
	if usedTokens < softTok {
		return nil
	}

	// Debounce check for soft trigger (non-forced).
	if usedTokens < hardTok && !forced && !atFullContextUsage(sess) {
		minGap := compressMinGapFromAgent(ag)
		if ts, err := c.deps.summaryReader.LatestSessionSummaryTime(ctx, sessionID); err == nil {
			if compressDebounceActive(ts, minGap, time.Now()) {
				return nil
			}
		}
	}

	// 失败抑制（非 forced）：确定性失败 sticky 到模型切换，瞬态失败按 minGap 退避。
	if !forced {
		provMod := compressProviderModelKey(sess, ag)
		if suppressed, reason := c.suppress.check(sessionID, provMod, compressMinGapFromAgent(ag), time.Now()); suppressed {
			c.lg.Info("压缩被失败抑制跳过",
				loggateway.StepID("session.compress"), loggateway.SessionID(sessionID),
				loggateway.Str("suppress_reason", reason))
			return nil
		}
	}

	// Try to acquire compressing flag.
	if !c.flight.tryStartCompress(sessionID) {
		c.lg.Info("压缩已在进行中，跳过", loggateway.StepID("session.compress"), loggateway.SessionID(sessionID))
		return nil
	}
	defer c.flight.finishCompress()

	body, tail, toolBody, cutoffTurn, err := c.loadCompressBody(ctx, sess, ag, sessionID)
	if err != nil || len(body) == 0 {
		return err
	}

	// 流程日志：仅在实际执行压缩时发射（软触发以下/防抖/抑制等早退路径不打）。
	flow := c.newFlowEmitter(ctx, sessionID)
	tokensBefore := usedTokens
	messagesBefore := len(body) + len(tail)
	if flow != nil {
		flow.LogStart("system.session.compress", "开始压缩会话上下文",
			event.P("tokens_before", tokensBefore),
			event.P("messages_before", messagesBefore))
	}

	// Two-level compression cascade: MemoryCompact → LLM.
	outcome := c.compressCascade(ctx, sess, ag, body, toolBody, sessionID, cutoffTurn, usedTokens, hardTok)
	if outcome.level == compressLevelNone || outcome.markdown == "" {
		if outcome.fail != compressFailureNone {
			c.suppress.record(sessionID, outcome.fail, compressProviderModelKey(sess, ag), time.Now())
			if flow != nil {
				flow.LogError("system.session.compress", "会话上下文压缩失败",
					event.P("error", "compression cascade failed"),
					event.P("tokens_before", tokensBefore),
					event.P("messages_before", messagesBefore))
			}
			c.emitCompressDecision(sessionID, "tripped", hardTok, tokensBefore, messagesBefore, 0, outcome, forced,
				fmt.Sprintf("压缩级联失败（fail_kind=%d）", outcome.fail), "compress_failed")
			return apierror.Internal(apierror.DomainSession, "compression cascade failed")
		}
		return nil
	}

	wrote, err := c.executeCompression(ctx, sess, ag, body, tail, outcome, sessionID, trpcUserID)
	if err != nil {
		c.suppress.record(sessionID, compressFailureTransient, compressProviderModelKey(sess, ag), time.Now())
		if flow != nil {
			flow.LogError("system.session.compress", "会话上下文压缩失败",
				event.P("error", err.Error()),
				event.P("tokens_before", tokensBefore),
				event.P("messages_before", messagesBefore))
		}
		c.emitCompressDecision(sessionID, "tripped", hardTok, tokensBefore, messagesBefore, 0, outcome, forced,
			"压缩写入失败: "+err.Error(), "compress_failed")
		return err
	}
	if !wrote {
		// CAS 冲突/幂等命中：未真实写入，保留既有抑制记录、不打成功日志。
		return nil
	}
	c.suppress.clear(sessionID)
	tokensAfter := 0
	if flow != nil {
		pairs := []event.Pair{
			event.P("tokens_before", tokensBefore),
			event.P("messages_before", messagesBefore),
			event.P("messages_after", len(tail)),
			event.P("compress_level", string(outcome.level)),
			event.P("cache_hit", outcome.cacheHit),
		}
		// 压缩后的 context_used_tokens 已在事务内更新，尽力而为重读一次供对比展示。
		if sessAfter, e := c.deps.sessionReader.GetSessionByID(ctx, sessionID); e == nil {
			tokensAfter = sessAfter.ContextUsedTokens
			pairs = append(pairs, event.P("tokens_after", tokensAfter))
		}
		flow.LogDone("system.session.compress", "会话上下文压缩完成", pairs...)
	}
	// P1-④（2026-08-30）：压缩成功强制写 system_guard 决策记录——此前仅
	// flowlog + 进程日志，run 结束后 decision_records 无三方互证证据
	// （R4-Q4「压缩事件无 decisions 留痕」）。flowlog 不在此重复发：
	// 压缩走自带 newFlowEmitter（turn 外异步 ctx 无 TraceEmitter）。
	c.emitCompressDecision(sessionID, "truncated", hardTok, tokensBefore, messagesBefore, tokensAfter, outcome, forced,
		fmt.Sprintf("会话上下文水位 %d tokens 触线（硬阈 %d），%s 压缩产出 %d rune 摘要", tokensBefore, hardTok, outcome.level, utf8.RuneCountInString(outcome.markdown)), "compress")
	return nil
}

// emitCompressDecision 把一次真实压缩尝试（成功/失败）写为 system_guard
// 决策记录（trigger=context_compacted，GuardName=session_compressor 与
// turn 内终审闸 context_compression 区分）。collector 为 nil 时静默降级。
// tokensAfter 为 0 表示未观测到（失败路径）。
func (c *Compressor) emitCompressDecision(sessionID, outcome string, hardTok, tokensBefore, messagesBefore, tokensAfter int, oc compressOutcome, forced bool, reasoning, action string) {
	if c.collector == nil {
		return
	}
	extra := map[string]any{
		"compress_level":  string(oc.level),
		"messages_before": messagesBefore,
		"cache_hit":       oc.cacheHit,
		"forced":          forced,
	}
	if tokensAfter > 0 {
		extra["tokens_after"] = tokensAfter
	}
	if oc.fail != compressFailureNone {
		extra["fail_kind"] = int(oc.fail)
	}
	decision.EmitGate(context.Background(), c.collector, decision.GateDecision{
		TriggerRule:   decision.TriggerContextCompacted,
		Outcome:       outcome,
		Scenario:      "会话上下文持久压缩",
		Reasoning:     reasoning,
		GuardName:     "session_compressor",
		SessionID:     sessionID,
		ObservedValue: tokensBefore,
		Threshold:     hardTok,
		Action:        action,
		Extra:         extra,
	})
}

// newFlowEmitter builds a system-domain flow emitter for compression events.
// Returns nil when the monitor bus is not wired (tests), so callers must
// nil-check before emitting.
func (c *Compressor) newFlowEmitter(ctx context.Context, sessionID string) *event.TraceEmitter {
	if c == nil || c.monitorBus == nil {
		return nil
	}
	return event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:       ctx,
		SessionID: sessionID,
		Domain:    event.TraceDomainSystem,
		LG:        c.lg,
		Infra:     event.NewInfraFromBus(c.monitorBus),
	})
}

// compressTriggerThresholds computes soft/hard trigger tokens (adaptive or static buffer).
// P3: absolute ceiling MaxTriggerTokens caps hardTok; softTok is scaled
// proportionally so the hysteresis band survives cap compression.
func (c *Compressor) compressTriggerThresholds(sessionID string, sess biz.Session, ag biz.Agent, usedTokens, window int) (softTok, hardTok int) {
	p := CompressPolicyFromAgent(ag)
	if adaptiveBufferEnabled(ag) {
		ratio := c.buf.getAdaptiveBufferRatio(sessionID, ag, usedTokens, window, sess.ToolCallCount, sess.RunCount)
		softTok, hardTok = softTriggerTokensWithRatio(ag, window, ratio), hardTriggerTokensWithRatio(ag, window, ratio)
	} else {
		softTok, hardTok = softTriggerTokens(ag, window), hardTriggerTokens(ag, window)
	}
	return capTriggerTokens(p, softTok, hardTok)
}

// RemoveSessionState cleans up per-session in-memory state when a session ends.
// This prevents unbounded growth of adaptiveBuffer entries over long-running sessions.
func (c *Compressor) RemoveSessionState(sessionID string) {
	if c == nil {
		return
	}
	c.buf.removeSessionState(sessionID)
	c.suppress.clear(sessionID)
}

// Close stops the background GC goroutine.
func (c *Compressor) Close() {
	if c == nil {
		return
	}
	c.buf.stopGC()
}

const (
	adaptiveBufferGCInterval = 10 * time.Minute
	adaptiveBufferMaxAge     = 30 * time.Minute
)

// loadCompressBody loads and splits messages for compression.
// Returns the body messages to compress, the tail messages to keep verbatim,
// the tool messages inside the compressed turn range (for the L3 transcript),
// and the cutoff turn number.
func (c *Compressor) loadCompressBody(ctx context.Context, sess biz.Session, ag biz.Agent, sessionID string) (body, tail, toolBody []biz.ChatMessage, cutoffTurn int, err error) {
	maxSummarized, err := c.deps.summaryReader.MaxSessionSummaryToTurn(ctx, sessionID)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	msgs, err := c.deps.messageReader.ListMessagesAfterTurn(ctx, sessionID, maxSummarized)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	timeline := timelineUserAssistant(msgs)
	if len(timeline) == 0 {
		return nil, nil, nil, 0, nil
	}

	_, keepTurns := compressThresholdAndKeep(ag)
	keepRows := messagesPerTurn * max(1, keepTurns)
	if len(timeline) <= keepRows {
		return nil, nil, nil, 0, nil
	}
	split := len(timeline) - keepRows
	cutoffTurn = timeline[split-1].TurnNumber

	for _, m := range timeline {
		if m.TurnNumber > maxSummarized && m.TurnNumber <= cutoffTurn {
			body = append(body, m)
		}
	}
	tail = timeline[split:]
	// 压缩轮次范围内的工具消息一并捞出，供 L3 transcript 渲染
	// （summary 策略），让摘要覆盖"实际执行了什么工具、返回了什么"。
	for _, m := range msgs {
		if !strings.EqualFold(strings.TrimSpace(m.Role), "tool") {
			continue
		}
		if m.TurnNumber > maxSummarized && m.TurnNumber <= cutoffTurn {
			toolBody = append(toolBody, m)
		}
	}
	return body, tail, toolBody, cutoffTurn, nil
}

// executeCompression performs the CAS-protected transaction to write the compression result,
// syncs the runtime snapshot, and publishes the compression notice.
// wrote=false 表示未写入（CAS 冲突/幂等命中）：调用方不得按成功处理
// （清除失败抑制、打"压缩完成"日志都以真实写入为前提）。
func (c *Compressor) executeCompression(ctx context.Context, sess biz.Session, ag biz.Agent, body, tail []biz.ChatMessage, outcome compressOutcome, sessionID, trpcUserID string) (wrote bool, err error) {
	fromTurn := body[0].TurnNumber
	toTurn := body[len(body)-1].TurnNumber

	versionBeforeCAS := sess.CompressVersion
	oldVersion, casErr := c.deps.compressRepo.TryIncrementCompressVersion(ctx, sessionID)
	if casErr != nil {
		return false, casErr
	}
	if oldVersion != versionBeforeCAS {
		// K5: 压缩版本 CAS 冲突——并发压缩已抢先写入，本次放弃。
		c.lg.Warn("压缩版本 CAS 冲突，跳过本次压缩",
			loggateway.StepID("session.compress"),
			loggateway.SessionID(sessionID),
			loggateway.Int64("version_expected", versionBeforeCAS),
			loggateway.Int64("version_actual", oldVersion))
		return false, nil
	}

	exists, existsErr := c.deps.summaryWriter.SessionSummaryExists(ctx, sessionID, fromTurn, toTurn)
	if existsErr != nil {
		c.lg.Warn("幂等检查失败",
			loggateway.StepID("session.compress"),
			loggateway.SessionID(sessionID),
			loggateway.Err(existsErr))
	}
	if exists {
		return false, nil
	}

	txMerged, txTail, txTaskState, txStateTurn, txErr := c.compressInTransaction(ctx, sessionID, ag, sess, tail, outcome, fromTurn, toTurn)
	if txErr != nil {
		return false, txErr
	}

	c.syncRuntimeSnapshot(ctx, sess, ag, sessionID, trpcUserID, txMerged, txTail, txTaskState, txStateTurn)
	c.writebackL1TaskBoard(ctx, sess, ag, txTaskState)
	c.postCompressionSync(ctx, sessionID, trpcUserID, ag, sess, fromTurn, toTurn, txMerged, txTail, outcome.cacheHit)

	return true, nil
}

// compressInTransaction executes the database transaction for compression.
// 返回合并后的叙事摘要、tail、本次解析出的任务状态及其时点轮次（供快照同步与 L1 回写复用）。
func (c *Compressor) compressInTransaction(ctx context.Context, sessionID string, ag biz.Agent, sess biz.Session, tail []biz.ChatMessage, outcome compressOutcome, fromTurn, toTurn int) (mergedSummary string, tailMsgs []biz.ChatMessage, taskState *biz.TaskState, stateAsOfTurn int, err error) {
	err = c.deps.compressRepo.CompressSessionInTx(ctx, sessionID, func(txCtx context.Context) error {
		priorRows, err := c.deps.summaryReader.ListSessionSummaries(txCtx, sessionID)
		if err != nil {
			return err
		}

		// 递归滚动摘要：LLM 已把历史摘要吸收进新摘要时，删除被吸收的旧行，
		// 用单条合并行替换，防止摘要无限拼接增长。
		absorb := outcome.absorbedPriors && len(priorRows) > 0
		if absorb {
			if err := c.deps.summaryWriter.DeleteSessionSummaries(txCtx, sessionID); err != nil {
				return err
			}
			for _, pr := range priorRows {
				if pr.FromTurn < fromTurn {
					fromTurn = pr.FromTurn
				}
			}
		}

		row := biz.SessionSummary{
			ID:              uuid.NewString(),
			SessionID:       sessionID,
			SummaryMarkdown: outcome.markdown,
			TaskStateJSON:   marshalTaskState(outcome.taskState),
			FromTurn:        fromTurn,
			ToTurn:          toTurn,
			TokenEstimate:   roughTokenEstimate(outcome.markdown),
			CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		}
		if err := c.deps.summaryWriter.InsertSessionSummary(txCtx, row); err != nil {
			return err
		}

		if absorb {
			mergedSummary = outcome.markdown
			taskState = outcome.taskState
			stateAsOfTurn = toTurn
		} else {
			allRows, err := c.deps.summaryReader.ListSessionSummaries(txCtx, sessionID)
			if err != nil {
				return err
			}
			mergedSummary = mergeSessionSummariesMarkdown(allRows)
			taskState, stateAsOfTurn = latestTaskState(allRows)
		}

		// tail 直接来自 loadCompressBody 的保留区（近期轮次原样保留）。
		tailMsgs = tail

		author := c.resolveAgentAuthor(txCtx, ag, sess.AgentID)

		raw, err := RewriteSnapshotWithCompression(sess.RunnerSnapshotJSON, mergedSummary, tailMsgs, author, taskState, stateAsOfTurn)
		if err != nil {
			return err
		}
		if err := c.deps.contextUpdater.UpdateRunnerSnapshotJSON(txCtx, sessionID, raw); err != nil {
			return err
		}

		win := llmcontext.ResolveWindow(llmcontext.ResolveInput{})
		est := estimateCompactedPromptTokens(mergedSummary, tailMsgs, calculateReservedSystem(ag))
		if taskState != nil {
			// 注入快照的结构化状态块也占 prompt 体积，计入水位估计。
			est += roughTokenEstimate(taskState.RenderBlockAsOf(stateAsOfTurn))
		}
		if err := c.deps.contextUpdater.UpdateSessionContextAfterCompression(txCtx, sessionID, est, win); err != nil {
			return err
		}

		preview := firstSummaryLine(mergedSummary)
		if preview != "" {
			if err := c.deps.summaryWriter.UpdateSessionListSummary(txCtx, sessionID, preview); err != nil {
				c.lg.Warn("update list summary failed", loggateway.StepID("session.compress"), loggateway.SessionID(sessionID), loggateway.Err(err))
			}
		}

		return nil
	})
	return mergedSummary, tailMsgs, taskState, stateAsOfTurn, err
}

// syncRuntimeSnapshot pushes the compressed snapshot to the trpc-agent-go runtime.
func (c *Compressor) syncRuntimeSnapshot(ctx context.Context, sess biz.Session, ag biz.Agent, sessionID, trpcUserID, txMerged string, txTail []biz.ChatMessage, taskState *biz.TaskState, stateAsOfTurn int) {
	if c.Runtime == nil {
		return
	}
	author := c.resolveAgentAuthor(ctx, ag, sess.AgentID)
	raw, snapErr := RewriteSnapshotWithCompression(sess.RunnerSnapshotJSON, txMerged, txTail, author, taskState, stateAsOfTurn)
	if snapErr == nil {
		if syncErr := c.Runtime.SyncRunnerSnapshot(ctx, trpcUserID, sessionID, raw, txMerged); syncErr != nil {
			c.lg.Warn("trpc 快照同步失败",
				loggateway.StepID("session.compress"),
				loggateway.SessionID(sessionID),
				loggateway.Err(syncErr),
				loggateway.Str("user_id", trpcUserID))
		}
	}
}

// postCompressionSync handles post-compression side effects: notice, memory resync, L0 force-snapshot,
// and framework summary sync.
func (c *Compressor) postCompressionSync(ctx context.Context, sessionID, trpcUserID string, ag biz.Agent, sess biz.Session, fromTurn, toTurn int, txMerged string, txTail []biz.ChatMessage, cacheHit bool) {
	win := llmcontext.ResolveWindow(llmcontext.ResolveInput{})
	est := estimateCompactedPromptTokens(txMerged, txTail, calculateReservedSystem(ag))
	ratio := llmcontext.ContextRatio(est, win)
	status := llmcontext.ContextStatusForRatio(ratio)

	preview := firstSummaryLine(txMerged)
	c.publishCompressionNotice(ctx, sessionID, fromTurn, toTurn, preview, est, win, ratio, status, cacheHit)
	c.resyncSessionMemory(ctx, sessionID)

	// Mark session for forced L0 snapshot on next model call (bypasses throttle).
	if c.Runtime != nil {
		c.Runtime.MarkForceL0Snapshot(sessionID)

		// Sync framework session summary after compression.
		// The framework's async summarizer will update session.Summaries,
		// providing a secondary summary path that complements the project's
		// three-level compression cascade.
		if err := c.Runtime.EnqueueFrameworkSummary(ctx, trpcUserID, sessionID, true); err != nil {
			c.lg.Warn("framework summary enqueue failed",
				loggateway.StepID("session.compress"),
				loggateway.SessionID(sessionID),
				loggateway.Err(err))
		}
	}
}

// compressCascade tries compression levels in order: MemoryCompact → LLM.
// Returns the outcome (level + markdown + absorb flag). level=compressLevelNone means nothing worked.
// usedTokens and hardTok control Level 2→3 fallback: LLM is only invoked when usedTokens >= hardTok.
// tools carries the tool messages inside the compressed turn range for the L3 transcript.
func (c *Compressor) compressCascade(ctx context.Context, sess biz.Session, ag biz.Agent, body, tools []biz.ChatMessage, sessionID string, cutoffTurn int, usedTokens, hardTok int) compressOutcome {
	// Level 2: MemoryCompact (near-zero cost — reuse extracted memory facts + L1 working memory).
	if memoryCompactEnabled(ag) && (c.memoryReader != nil || c.l1Reader != nil) {
		// 摘要行数累积上限：达到上限时跳过 L2 强制 L3，由 LLM 吸收合并全部历史摘要
		// （行数归一），防止 MemoryCompact 标记行在长会话中无限累积。
		if c.summaryRowsExceeded(ctx, ag, sessionID) {
			c.lg.Info("摘要行数超限，跳过 MemoryCompact 强制 LLM 压缩",
				loggateway.StepID("session.compress"),
				loggateway.SessionID(sessionID),
				loggateway.Int("max_rows", summaryMaxRows(ag)))
			return c.llmCompress(ctx, sess, ag, body, tools, sessionID)
		}
		memResult := tryMemoryCompact(ctx, body, c.memoryReader, c.l1Reader, sessionID, c.lg)
		if memResult.didCompact {
			c.lg.Info("MemoryCompact 压缩成功",
				loggateway.StepID("session.compress"),
				loggateway.SessionID(sessionID),
				loggateway.Str("compress_level", string(compressLevelMemory)))
			narrative, ts := compress.ExtractTaskState(memResult.summaryMarkdown)
			return compressOutcome{level: compressLevelMemory, markdown: narrative, taskState: ts}
		}
		// Level 2 failed: only escalate to LLM if at or above hard trigger threshold.
		if usedTokens < hardTok {
			c.lg.Info("MemoryCompact 未产出有效摘要，未达 hard trigger，等待下次触发",
				loggateway.StepID("session.compress"),
				loggateway.SessionID(sessionID),
				loggateway.Int("used_tokens", usedTokens),
				loggateway.Int("hard_trigger_tokens", hardTok))
			return compressOutcome{level: compressLevelNone}
		}
	}

	// Level 3: LLM compression (full summarization via LLM call).
	return c.llmCompress(ctx, sess, ag, body, tools, sessionID)
}

// summaryRowsExceeded reports whether the stored rolling-summary row count has
// reached the configured cap (CompressThreshold.SummaryMaxRows). Read failures
// are non-fatal: fall back to Level 2 (compression must not be blocked by an
// observability query).
func (c *Compressor) summaryRowsExceeded(ctx context.Context, ag biz.Agent, sessionID string) bool {
	maxRows := summaryMaxRows(ag)
	if maxRows <= 0 || c.deps.summaryReader == nil {
		return false
	}
	rows, err := c.deps.summaryReader.ListSessionSummaries(ctx, sessionID)
	if err != nil {
		c.lg.Warn("读取摘要行数失败，按未超限处理",
			loggateway.StepID("session.compress"),
			loggateway.SessionID(sessionID),
			loggateway.Err(err))
		return false
	}
	return len(rows) >= maxRows
}

// llmCompress performs Level 3 LLM-based compression.
// tools 为压缩轮次范围内的工具消息：summary 策略下交织进 transcript，
// hybrid / drop_tool_results 策略下不喂给 LLM（与过滤语义一致）。
func (c *Compressor) llmCompress(ctx context.Context, sess biz.Session, ag biz.Agent, body, tools []biz.ChatMessage, sessionID string) compressOutcome {
	strategy := truncateStrategy(ag)
	filteredBody := filterMessagesForTruncateStrategy(body, strategy)

	if strategy == "drop_oldest" {
		c.lg.Info("LLM 压缩完成（drop_oldest）",
			loggateway.StepID("session.compress"),
			loggateway.SessionID(sessionID),
			loggateway.Str("compress_level", string(compressLevelLLM)),
			loggateway.Str("truncate_strategy", strategy))
		return compressOutcome{level: compressLevelLLM, markdown: "[Earlier turns removed per drop_oldest policy]"}
	}
	transcriptMsgs := filteredBody
	if compressStrategyRendersToolResults(strategy) && len(tools) > 0 {
		transcriptMsgs = mergeTranscriptMessages(filteredBody, tools)
	}
	return c.llmSummarize(ctx, sess, ag, transcriptMsgs, strategy, sessionID)
}

// llmSummarize runs the summary-strategy LLM compression flow: recursive
// prior-summary merge, chunked rolling summarization for oversized transcripts,
// retry-guarded LLM calls, hybrid fallback, reduction guard.
func (c *Compressor) llmSummarize(ctx context.Context, sess biz.Session, ag biz.Agent, filteredBody []biz.ChatMessage, strategy, sessionID string) compressOutcome {
	cProv, cMod := compressProviderModel(sess, ag)

	// 递归滚动摘要：历史摘要交给 LLM 吸收合并，防止事后拼接无限增长。
	var priorMerged string
	if c.deps.summaryReader != nil {
		if rows, err := c.deps.summaryReader.ListSessionSummaries(ctx, sessionID); err == nil {
			priorMerged = mergeSessionSummariesMarkdown(rows)
		} else {
			c.lg.Warn("读取历史摘要失败，按无历史摘要压缩",
				loggateway.StepID("session.compress"), loggateway.SessionID(sessionID), loggateway.Err(err))
		}
	}

	// 分块滚动摘要：超大 transcript 逐块摘要，前一块产出作为下一块的
	// PriorSummary 滚动吸收——避免单次巨型 transcript 撑爆压缩模型上下文
	// （上下文溢出 = 确定性失败 → sticky 抑制死锁）。
	chunks := splitMessagesForCompress(filteredBody, compressChunkMaxRunes)
	if len(chunks) == 0 {
		chunks = [][]biz.ChatMessage{{}}
	}

	t0 := time.Now()
	rolling := priorMerged
	totalRunes := 0
	var md string
	var res compress.Result
	// cacheHit 聚合全块：仅当每一次 LLM 调用都命中缓存才上报 true
	// （部分命中谎称整次零 LLM 调用会污染监控/计费口径）。
	cacheHit := true
	llmSucceeded := false
	for _, chunk := range chunks {
		transcript := buildCompressTranscript(chunk)
		transcriptRunes := utf8.RuneCountInString(transcript)
		totalRunes += transcriptRunes
		var fail compressFailureKind
		var hit bool
		md, res, fail, hit = c.llmCallWithRetry(ctx, sessionID, compress.Request{
			Transcript:   transcript,
			PriorSummary: rolling,
			Provider:     cProv,
			Model:        cMod,
		}, transcriptRunes)
		if fail == compressFailureDeterministic {
			// 任一块确定性失败（上下文溢出/鉴权/参数错误）：整个级联按确定性中止，
			// 不再发送后续块（重发必然再败）。
			return compressOutcome{level: compressLevelNone, fail: compressFailureDeterministic}
		}
		if md == "" {
			if fail == compressFailureNone {
				// ctx 取消（进程关闭/请求超时）不是压缩失败：静默中止——不记失败抑制，
				// hybrid 策略同样不得写兜底标记（写入事务在 detach ctx 上仍会提交，
				// 未摘要内容会被永久跳过）。
				return compressOutcome{level: compressLevelNone, fail: compressFailureNone}
			}
			// 瞬态失败（重试耗尽）：hybrid 策略回退兜底标记，其余上抛瞬态失败。
			if strategy == "hybrid" {
				md = "[Earlier turns trimmed per hybrid policy]"
			}
			if md == "" {
				return compressOutcome{level: compressLevelNone, fail: compressFailureTransient}
			}
			// hybrid 兜底标记不含历史内容：停止分块级联，不吸收旧摘要（防丢数据）。
			llmSucceeded = false
			cacheHit = false
			break
		}
		rolling = md
		cacheHit = cacheHit && hit
		llmSucceeded = true
	}

	// 减量守卫：压缩无实质收益则丢弃（hybrid 兜底标记除外）。
	if strategy != "hybrid" {
		// 分母计入被吸收的历史摘要：滚动吸收场景下 LLM 产出覆盖 (prior+body)，
		// 只比 body 会在成熟长会话中必然误杀（丢弃结果 + 误记瞬态失败）。
		bodyTokens := llmcontext.EstimateTokensFromChars(totalRunes + utf8.RuneCountInString(priorMerged))
		mdTokens := llmcontext.EstimateTokensFromChars(utf8.RuneCountInString(md))
		if !passesReductionGuard(mdTokens, bodyTokens) {
			c.lg.Warn("压缩减量不足，丢弃结果",
				loggateway.StepID("session.compress"),
				loggateway.SessionID(sessionID),
				loggateway.Int("summary_tokens", mdTokens),
				loggateway.Int("body_tokens", bodyTokens))
			return compressOutcome{level: compressLevelNone, fail: compressFailureTransient}
		}
	}
	c.lg.Info("LLM 压缩完成",
		loggateway.StepID("session.compress"),
		loggateway.SessionID(sessionID),
		loggateway.Str("compress_level", string(compressLevelLLM)),
		loggateway.Str("compress_provider", res.Provider),
		loggateway.Str("compress_model", res.Model),
		loggateway.Int("prompt_tokens", res.PromptTokens),
		loggateway.Int("completion_tokens", res.CompletionTokens),
		loggateway.Duration(time.Since(t0).Milliseconds()),
		loggateway.Str("prompt_ver", res.PromptVersion),
		loggateway.Int("compress_chunks", len(chunks)),
		loggateway.Str("truncate_strategy", strategy))
	// v4 双段化：从最终叙事产出末尾拆出 task_state 结构化段（剥块后叙事入库）。
	narrative, taskState := compress.ExtractTaskState(md)
	return compressOutcome{
		level:     compressLevelLLM,
		markdown:  narrative,
		taskState: taskState,
		// 仅当 LLM 真实产出摘要时才算吸收（hybrid 兜底标记不含历史内容，删除旧行会丢数据）。
		absorbedPriors: priorMerged != "" && llmSucceeded,
		cacheHit:       cacheHit,
	}
}

// llmCallWithRetry calls the LLM compressor up to llmCompressMaxAttempts times.
// 重试条件：瞬态错误 / 空摘要 / 退化摘要；确定性错误与 ctx 取消立即终止。
// 返回最终 md（空=失败）、最后一次成功响应 res、失败终态 kind（none=成功）、
// 以及成功结果是否来自压缩缓存。
func (c *Compressor) llmCallWithRetry(ctx context.Context, sessionID string, req compress.Request, transcriptRunes int) (md string, res compress.Result, fail compressFailureKind, cacheHit bool) {
	for attempt := 1; attempt <= llmCompressMaxAttempts; attempt++ {
		if ctx.Err() != nil {
			// ctx 取消（进程关闭/请求超时）不是压缩失败，不记入抑制。
			return "", res, compressFailureNone, false
		}
		var err error
		var hit bool
		if cc, ok := c.Compress.(cacheHitAwareCompressor); ok {
			res, hit, err = cc.CompressWithCacheHit(ctx, req)
		} else {
			res, err = c.Compress.Compress(ctx, req)
		}
		if err != nil {
			kind := classifyCompressError(err)
			c.lg.Warn("LLM 压缩调用失败",
				loggateway.StepID("session.compress"),
				loggateway.SessionID(sessionID),
				loggateway.Int("attempt", attempt),
				loggateway.Err(err))
			if kind == compressFailureDeterministic {
				// 确定性失败（上下文溢出/鉴权/参数错误）：重发必然再败，不重试。
				return "", res, compressFailureDeterministic, false
			}
			continue
		}
		md = strings.TrimSpace(res.Markdown)
		if md == "" {
			c.lg.Warn("LLM 压缩返回空摘要，重试",
				loggateway.StepID("session.compress"),
				loggateway.SessionID(sessionID),
				loggateway.Int("attempt", attempt))
			continue
		}
		if isDegenerateSummary(md, transcriptRunes) {
			c.lg.Warn("LLM 压缩摘要退化，重试",
				loggateway.StepID("session.compress"),
				loggateway.SessionID(sessionID),
				loggateway.Int("attempt", attempt),
				loggateway.Int("summary_runes", utf8.RuneCountInString(md)),
				loggateway.Int("transcript_runes", transcriptRunes))
			md = ""
			continue
		}
		return md, res, compressFailureNone, hit
	}
	return "", res, compressFailureTransient, false
}

func (c *Compressor) resolveAgentAuthor(ctx context.Context, ag biz.Agent, agentID string) string {
	author := strings.TrimSpace(ag.AgentKey)
	if c.agents != nil && strings.TrimSpace(agentID) != "" {
		if a, e := c.agents.GetAgentByID(ctx, agentID); e == nil {
			if k := strings.TrimSpace(a.AgentKey); k != "" {
				author = k
			}
		}
	}
	if author == "" {
		author = "agent"
	}
	return author
}

func (c *Compressor) publishCompressionNotice(ctx context.Context, sessionID string, fromTurn, toTurn int, preview string, contextUsedTokens, contextWindow int, ratio float64, status string, cacheHit bool) {
	if c == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	preview = strings.TrimSpace(preview)
	text := "会话上下文已自动压缩"
	if fromTurn > 0 && toTurn >= fromTurn {
		text += "（turn " + strconv.Itoa(fromTurn) + "–" + strconv.Itoa(toTurn) + "）"
	}
	if preview != "" {
		text += "。摘要：" + preview
	}
	now := time.Now().UTC().Format(time.RFC3339)
	sysMsg := biz.ChatMessage{
		ID:              uuid.NewString(),
		SessionID:       sessionID,
		Role:            "system",
		ContentMarkdown: text,
		Status:          "ok",
		CreatedAt:       now,
	}
	if err := c.deps.messageWriter.AppendChatMessage(ctx, sessionID, sysMsg, false); err != nil {
		c.lg.Warn("append compress notice message failed", loggateway.StepID("session.compress"), loggateway.SessionID(sessionID), loggateway.Err(err))
	}
	if c.monitorBus == nil {
		return
	}
	ev := contract.NewMonitorEvent(contract.MonitorEventTypeAlertNotify, "system")
	ev.SessionID = sessionID
	ev.Message = text
	ev.Metadata = map[string]any{
		"alert_kind":          "session_compress",
		"message":             text,
		"kind":                "system.session.compress",
		"from_turn":           fromTurn,
		"to_turn":             toTurn,
		"context_used_tokens": contextUsedTokens,
		"context_window":      contextWindow,
		"context_used_ratio":  ratio,
		"context_status":      status,
		"cache_hit":           cacheHit,
	}
	c.monitorBus.Publish(ctx, ev)
}

func (c *Compressor) resyncSessionMemory(ctx context.Context, sessionID string) {
	if c == nil || c.Memory == nil {
		return
	}
	if err := c.Memory.DeleteSessionEventEntities(ctx, sessionID); err != nil {
		c.lg.Warn("delete session event entities failed", loggateway.StepID("session.compress"), loggateway.SessionID(sessionID), loggateway.Err(err))
	}
}
