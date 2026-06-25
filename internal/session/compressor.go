package session

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
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
	Logger       loggateway.Logger
}

// compressLevel identifies which compression tier was used.
type compressLevel string

const (
	compressLevelNone   compressLevel = "none"
	compressLevelMicro  compressLevel = "micro_compact"
	compressLevelMemory compressLevel = "memory_compact"
	compressLevelLLM    compressLevel = "llm_compact"
)

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
	deps         compressDeps
	agents       AgentKeyLookup
	Runtime      *Runtime
	Compress     compress.Compressor
	Memory       MemoryResync
	monitorBus   contract.MonitorBus
	memoryReader biz.MemoryFactReader
	l1Reader     biz.L1AdminReader
	lg           loggateway.Logger

	flight *compressFlightManager
	buf    *compressBufferManager
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

func containsMicroCompactMarker(md string) bool {
	return strings.Contains(md, "[MicroCompact:")
}

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
		agents:       cfg.Agents,
		Runtime:      cfg.Runtime,
		Memory:       cfg.Memory,
		Compress:     cfg.Compress,
		monitorBus:   cfg.MonitorBus,
		memoryReader: cfg.MemoryReader,
		l1Reader:     cfg.L1Reader,
		lg:           cfg.Logger,
		flight:       newCompressFlightManager(),
		buf:          newCompressBufferManager(),
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
		if err := c.runCompress(runCtx, sid, trpcUserID, ag, false); err != nil && c.monitorBus != nil {
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
	if err := c.runCompress(runCtx, sid, TRPCUserKey(ctx), ag, true); err != nil && c.monitorBus != nil {
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
	runCtx = compress.ContextWithSessionID(runCtx, sid)

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
		if containsMicroCompactMarker(latest.SummaryMarkdown) {
			level = "micro_compact"
		} else if containsMemoryCompactMarker(latest.SummaryMarkdown) {
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
	window := llmcontext.ResolveWindow(llmcontext.ResolveInput{
		SessionDefaultWindow: sess.LastContextWindowTokens,
	})
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

func (c *Compressor) runCompress(ctx context.Context, sessionID, trpcUserID string, ag biz.Agent, skipMinGap bool) error {
	if !sessionCompressEnabled(ag) {
		return nil
	}
	sess, err := c.deps.sessionReader.GetSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}

	// Calculate effective_budget thresholds.
	window := llmcontext.ResolveWindow(llmcontext.ResolveInput{
		SessionDefaultWindow: sess.LastContextWindowTokens,
		AgentWindow:          ag.ContextWindow,
	})

	usedTokens := sess.ContextUsedTokens

	// Determine adaptive or static buffer ratio and compute trigger tokens.
	var softTok, hardTok int
	if adaptiveBufferEnabled(ag) {
		ratio := c.buf.getAdaptiveBufferRatio(sessionID, ag, usedTokens, window, sess.ToolCallCount, sess.RunCount)
		softTok = softTriggerTokensWithRatio(ag, window, ratio)
		hardTok = hardTriggerTokensWithRatio(ag, window, ratio)
	} else {
		softTok = softTriggerTokens(ag, window)
		hardTok = hardTriggerTokens(ag, window)
	}

	// Below soft trigger: nothing to do.
	if usedTokens < softTok {
		return nil
	}

	// Debounce check for soft trigger (non-forced).
	if usedTokens < hardTok && !skipMinGap && !atFullContextUsage(sess) {
		minGap := compressMinGapFromAgent(ag)
		if ts, err := c.deps.summaryReader.LatestSessionSummaryTime(ctx, sessionID); err == nil {
			if compressDebounceActive(ts, minGap, time.Now()) {
				return nil
			}
		}
	}

	// Try to acquire compressing flag.
	if !c.flight.tryStartCompress(sessionID) {
		c.lg.Info("压缩已在进行中，跳过", loggateway.StepID("session.compress"), loggateway.SessionID(sessionID))
		return nil
	}
	defer c.flight.finishCompress()

	body, cutoffTurn, err := c.loadCompressBody(ctx, sess, ag, sessionID)
	if err != nil || len(body) == 0 {
		return err
	}

	// Three-level compression cascade: MicroCompact → MemoryCompact → LLM.
	level, md := c.compressCascade(ctx, sess, ag, body, sessionID, cutoffTurn, usedTokens, hardTok)
	if level == compressLevelNone || md == "" {
		return nil
	}

	return c.executeCompression(ctx, sess, ag, body, md, sessionID, trpcUserID, cutoffTurn)
}

// RemoveSessionState cleans up per-session in-memory state when a session ends.
// This prevents unbounded growth of adaptiveBuffer entries over long-running sessions.
func (c *Compressor) RemoveSessionState(sessionID string) {
	if c == nil {
		return
	}
	c.buf.removeSessionState(sessionID)
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
// Returns the body messages and the cutoff turn number.
func (c *Compressor) loadCompressBody(ctx context.Context, sess biz.Session, ag biz.Agent, sessionID string) ([]biz.ChatMessage, int, error) {
	maxSummarized, err := c.deps.summaryReader.MaxSessionSummaryToTurn(ctx, sessionID)
	if err != nil {
		return nil, 0, err
	}
	msgs, err := c.deps.messageReader.ListMessagesAfterTurn(ctx, sessionID, maxSummarized)
	if err != nil {
		return nil, 0, err
	}
	timeline := timelineUserAssistant(msgs)
	if len(timeline) == 0 {
		return nil, 0, nil
	}

	_, keepTurns := compressThresholdAndKeep(ag)
	keepRows := messagesPerTurn * max(1, keepTurns)
	if len(timeline) <= keepRows {
		return nil, 0, nil
	}
	split := len(timeline) - keepRows
	cutoffTurn := timeline[split-1].TurnNumber

	var body []biz.ChatMessage
	for _, m := range timeline {
		if m.TurnNumber > maxSummarized && m.TurnNumber <= cutoffTurn {
			body = append(body, m)
		}
	}
	return body, cutoffTurn, nil
}

// executeCompression performs the CAS-protected transaction to write the compression result,
// syncs the runtime snapshot, and publishes the compression notice.
func (c *Compressor) executeCompression(ctx context.Context, sess biz.Session, ag biz.Agent, body []biz.ChatMessage, md string, sessionID, trpcUserID string, cutoffTurn int) error {
	fromTurn := body[0].TurnNumber
	toTurn := body[len(body)-1].TurnNumber

	versionBeforeCAS := sess.CompressVersion
	oldVersion, casErr := c.deps.compressRepo.TryIncrementCompressVersion(ctx, sessionID)
	if casErr != nil {
		return casErr
	}
	if oldVersion != versionBeforeCAS {
		return nil
	}

	exists, existsErr := c.deps.summaryWriter.SessionSummaryExists(ctx, sessionID, fromTurn, toTurn)
	if existsErr != nil && c.monitorBus != nil {
		c.lg.Warn("幂等检查失败",
			loggateway.StepID("session.compress"),
			loggateway.SessionID(sessionID),
			loggateway.Err(existsErr))
	}
	if exists {
		return nil
	}

	txMerged, txTail, txErr := c.compressInTransaction(ctx, sessionID, ag, sess, body, md, fromTurn, toTurn, cutoffTurn)
	if txErr != nil {
		return txErr
	}

	c.syncRuntimeSnapshot(ctx, sess, ag, sessionID, trpcUserID, txMerged, txTail)
	c.postCompressionSync(ctx, sessionID, trpcUserID, ag, sess, fromTurn, toTurn, txMerged, txTail)

	return nil
}

// compressInTransaction executes the database transaction for compression.
func (c *Compressor) compressInTransaction(ctx context.Context, sessionID string, ag biz.Agent, sess biz.Session, body []biz.ChatMessage, md string, fromTurn, toTurn, cutoffTurn int) (mergedSummary string, tailMsgs []biz.ChatMessage, err error) {
	err = c.deps.compressRepo.CompressSessionInTx(ctx, sessionID, func(txCtx context.Context) error {
		row := biz.SessionSummary{
			ID:              uuid.NewString(),
			SessionID:       sessionID,
			SummaryMarkdown: md,
			FromTurn:        fromTurn,
			ToTurn:          toTurn,
			TokenEstimate:   roughTokenEstimate(md),
			CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		}
		if err := c.deps.summaryWriter.InsertSessionSummary(txCtx, row); err != nil {
			return err
		}

		allRows, err := c.deps.summaryReader.ListSessionSummaries(txCtx, sessionID)
		if err != nil {
			return err
		}
		mergedSummary = mergeSessionSummariesMarkdown(allRows)

		for _, m := range body {
			if m.TurnNumber > cutoffTurn {
				tailMsgs = append(tailMsgs, m)
			}
		}

		author := c.resolveAgentAuthor(txCtx, ag, sess.AgentID)

		raw, err := RewriteSnapshotWithCompression(sess.RunnerSnapshotJSON, mergedSummary, tailMsgs, author)
		if err != nil {
			return err
		}
		if err := c.deps.contextUpdater.UpdateRunnerSnapshotJSON(txCtx, sessionID, raw); err != nil {
			return err
		}

		win := llmcontext.ResolveWindow(llmcontext.ResolveInput{
			SessionDefaultWindow: sess.LastContextWindowTokens,
			AgentWindow:          ag.ContextWindow,
		})
		est := estimateCompactedPromptTokens(mergedSummary, tailMsgs)
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
	return mergedSummary, tailMsgs, err
}

// syncRuntimeSnapshot pushes the compressed snapshot to the trpc-agent-go runtime.
func (c *Compressor) syncRuntimeSnapshot(ctx context.Context, sess biz.Session, ag biz.Agent, sessionID, trpcUserID, txMerged string, txTail []biz.ChatMessage) {
	if c.Runtime == nil {
		return
	}
	author := c.resolveAgentAuthor(ctx, ag, sess.AgentID)
	raw, snapErr := RewriteSnapshotWithCompression(sess.RunnerSnapshotJSON, txMerged, txTail, author)
	if snapErr == nil {
		if syncErr := c.Runtime.SyncRunnerSnapshot(ctx, trpcUserID, sessionID, raw, txMerged); syncErr != nil && c.monitorBus != nil {
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
func (c *Compressor) postCompressionSync(ctx context.Context, sessionID, trpcUserID string, ag biz.Agent, sess biz.Session, fromTurn, toTurn int, txMerged string, txTail []biz.ChatMessage) {
	win := llmcontext.ResolveWindow(llmcontext.ResolveInput{
		SessionDefaultWindow: sess.LastContextWindowTokens,
		AgentWindow:          ag.ContextWindow,
	})
	est := estimateCompactedPromptTokens(txMerged, txTail)
	ratio := llmcontext.ContextRatio(est, win)
	status := llmcontext.ContextStatusForRatio(ratio)

	preview := firstSummaryLine(txMerged)
	c.publishCompressionNotice(ctx, sessionID, fromTurn, toTurn, preview, est, win, ratio, status)
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

// compressCascade tries compression levels in order: MicroCompact → MemoryCompact → LLM.
// Returns the level used and the summary markdown. Returns compressLevelNone if nothing worked.
// usedTokens and hardTok control Level 2→3 fallback: LLM is only invoked when usedTokens >= hardTok.
func (c *Compressor) compressCascade(ctx context.Context, sess biz.Session, ag biz.Agent, body []biz.ChatMessage, sessionID string, cutoffTurn int, usedTokens, hardTok int) (compressLevel, string) {
	// Level 1: MicroCompact (zero API cost — just clear old tool results).
	if microCompactEnabled(ag) {
		currentTurn := cutoffTurn + 1
		mcResult := tryMicroCompact(body, currentTurn)
		if mcResult.didCompact {
			c.lg.Info("MicroCompact 压缩成功",
				loggateway.StepID("session.compress"),
				loggateway.SessionID(sessionID),
				loggateway.Str("compress_level", string(compressLevelMicro)))
			return compressLevelMicro, mcResult.summaryMarkdown
		}
	}

	// Level 2: MemoryCompact (near-zero cost — reuse extracted memory facts + L1 working memory).
	if memoryCompactEnabled(ag) && (c.memoryReader != nil || c.l1Reader != nil) {
		memResult := tryMemoryCompact(ctx, body, c.memoryReader, c.l1Reader, sessionID, c.lg)
		if memResult.didCompact {
			c.lg.Info("MemoryCompact 压缩成功",
				loggateway.StepID("session.compress"),
				loggateway.SessionID(sessionID),
				loggateway.Str("compress_level", string(compressLevelMemory)))
			return compressLevelMemory, memResult.summaryMarkdown
		}
		// Level 2 failed: only escalate to LLM if at or above hard trigger threshold.
		if usedTokens < hardTok {
			c.lg.Info("MemoryCompact 未产出有效摘要，未达 hard trigger，等待下次触发",
				loggateway.StepID("session.compress"),
				loggateway.SessionID(sessionID),
				loggateway.Int("used_tokens", usedTokens),
				loggateway.Int("hard_trigger_tokens", hardTok))
			return compressLevelNone, ""
		}
	}

	// Level 3: LLM compression (full summarization via LLM call).
	return c.llmCompress(ctx, sess, ag, body, sessionID)
}

// llmCompress performs Level 3 LLM-based compression.
func (c *Compressor) llmCompress(ctx context.Context, sess biz.Session, ag biz.Agent, body []biz.ChatMessage, sessionID string) (compressLevel, string) {
	strategy := truncateStrategy(ag)
	filteredBody := filterMessagesForTruncateStrategy(body, strategy)

	switch strategy {
	case "drop_oldest":
		c.lg.Info("LLM 压缩完成（drop_oldest）",
			loggateway.StepID("session.compress"),
			loggateway.SessionID(sessionID),
			loggateway.Str("compress_level", string(compressLevelLLM)),
			loggateway.Str("truncate_strategy", strategy))
		return compressLevelLLM, "[Earlier turns removed per drop_oldest policy]"
	default:
		transcript := buildCompressTranscript(filteredBody)
		cProv, cMod := compressProviderModel(sess, ag)
		t0 := time.Now()
		res, err := c.Compress.Compress(ctx, compress.Request{
			Transcript: transcript,
			Provider:   cProv,
			Model:      cMod,
		})
		if err != nil {
			c.lg.Warn("LLM 压缩失败",
				loggateway.StepID("session.compress"),
				loggateway.SessionID(sessionID),
				loggateway.Err(err))
			return compressLevelNone, ""
		}
		md := strings.TrimSpace(res.Markdown)
		if md == "" && strategy == "hybrid" {
			md = "[Earlier turns trimmed per hybrid policy]"
		}
		if md == "" {
			return compressLevelNone, ""
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
			loggateway.Str("truncate_strategy", strategy))
		return compressLevelLLM, md
	}
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

func (c *Compressor) publishCompressionNotice(ctx context.Context, sessionID string, fromTurn, toTurn int, preview string, contextUsedTokens, contextWindow int, ratio float64, status string) {
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
