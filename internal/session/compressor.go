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
	"aranea-agents/internal/event"
	"aranea-agents/internal/llmcontext"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// MemoryResync clears derived memory entities after snapshot rewrite.
type MemoryResync interface {
	DeleteSessionEventEntities(ctx context.Context, sessionID string) error
}

type AgentKeyLookup interface {
	GetAgentByID(ctx context.Context, id string) (biz.Agent, error)
}

type CompressorDeps interface {
	biz.SessionReader
	biz.MessageReader
	biz.MessageWriter
	biz.SummaryReader
	biz.SummaryWriter
	biz.ContextUpdater
	biz.CompressRepo
}

// compressLevel identifies which compression tier was used.
type compressLevel string

const (
	compressLevelNone   compressLevel = "none"
	compressLevelMicro  compressLevel = "micro_compact"
	compressLevelMemory compressLevel = "memory_compact"
	compressLevelLLM    compressLevel = "llm_compact"
)

type Compressor struct {
	sessionReader  biz.SessionReader
	messageReader  biz.MessageReader
	messageWriter  biz.MessageWriter
	summaryReader  biz.SummaryReader
	summaryWriter  biz.SummaryWriter
	contextUpdater biz.ContextUpdater
	compressRepo   biz.CompressRepo
	agents         AgentKeyLookup
	Runtime        *Runtime
	Compress       compress.Compressor
	Memory         MemoryResync
	EventBus       event.Bus
	memoryReader   biz.MemoryFactReader
	l1Reader       biz.L1AdminReader
	lg             loggateway.Logger

	inFlight sync.Map

	compressing     atomic.Bool
	compressStart   time.Time
	compressMu      sync.Mutex
	compressTimeout time.Duration
}

var _ biz.NativeTurnCompressor  = (*Compressor)(nil)
var _ biz.DurableTurnCompressor = (*Compressor)(nil)
var _ biz.ManualCompressor      = (*Compressor)(nil)

type preserveInstructionKey struct{}

func containsMicroCompactMarker(md string) bool {
	return strings.Contains(md, "[MicroCompact:")
}

func containsMemoryCompactMarker(md string) bool {
	return strings.Contains(md, "## Session Memory Summary")
}

func NewCompressor(
	sessions CompressorDeps,
	agents AgentKeyLookup,
	runtime *Runtime,
	memory MemoryResync,
	comp compress.Compressor,
	eventBus event.Bus,
	memoryReader biz.MemoryFactReader,
	l1Reader biz.L1AdminReader,
	lg loggateway.Logger,
) *Compressor {
	return &Compressor{
		sessionReader:   sessions,
		messageReader:   sessions,
		messageWriter:   sessions,
		summaryReader:   sessions,
		summaryWriter:   sessions,
		contextUpdater:  sessions,
		compressRepo:    sessions,
		agents:          agents,
		Runtime:         runtime,
		Memory:          memory,
		Compress:        comp,
		EventBus:        eventBus,
		memoryReader:    memoryReader,
		l1Reader:        l1Reader,
		lg:              lg,
		compressTimeout: 10 * time.Minute,
	}
}

func (c *Compressor) AfterNativeTurn(ctx context.Context, sessionID string, ag biz.Agent) {
	if c == nil || c.sessionReader == nil || c.Compress == nil {
		return
	}
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return
	}
	trpcUserID := TRPCUserKey(ctx)
	safego.Go(ctx, "session-compress", func() {
		if _, loaded := c.inFlight.LoadOrStore(sid, true); loaded {
			return
		}
		defer c.inFlight.Delete(sid)
		runCtx, cancel := context.WithTimeout(context.Background(), compressRunTimeout)
		defer cancel()
		if err := c.runCompress(runCtx, sid, trpcUserID, ag, false); err != nil && c.EventBus != nil {
			c.lg.Warn("会话压缩失败", loggateway.StepID("session.compress"), loggateway.SessionID(sid), loggateway.Err(err))
		}
	})
}

func (c *Compressor) BeforeDurableTurn(ctx context.Context, sessionID string, ag biz.Agent) error {
	if c == nil || c.sessionReader == nil || c.Compress == nil {
		return nil
	}
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return nil
	}
	if _, loaded := c.inFlight.LoadOrStore(sid, true); loaded {
		return nil
	}
	defer c.inFlight.Delete(sid)
	runCtx, cancel := context.WithTimeout(ctx, compressRunTimeout)
	defer cancel()
	if err := c.runCompress(runCtx, sid, TRPCUserKey(ctx), ag, true); err != nil && c.EventBus != nil {
		c.lg.Warn("Durable turn 前压缩失败", loggateway.StepID("session.compress"), loggateway.SessionID(sid), loggateway.Err(err))
	}
	return nil
}

func (c *Compressor) CompactSession(ctx context.Context, sessionID string, preserveInstruction string) (*biz.CompactResult, error) {
	if c == nil || c.sessionReader == nil || c.Compress == nil {
		return nil, nil
	}
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return nil, kerrors.BadRequest("SESSION", "session_id is required")
	}
	if _, loaded := c.inFlight.LoadOrStore(sid, true); loaded {
		return nil, kerrors.BadRequest("SESSION", "compression already in progress for this session")
	}
	defer c.inFlight.Delete(sid)

	sess, err := c.sessionReader.GetSessionByID(ctx, sid)
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

	sessAfter, err := c.sessionReader.GetSessionByID(ctx, sid)
	if err != nil {
		return &biz.CompactResult{Compacted: true, EstimatedTokensBefore: estBefore, EstimatedTokensAfter: estBefore}, nil
	}

	level := "auto_compact"
	fromTurn, toTurn := 0, 0
	if summaries, sErr := c.summaryReader.ListSessionSummaries(ctx, sid); sErr == nil && len(summaries) > 0 {
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

// tryStartCompress attempts to mark the compressor as active. Returns true if
// this caller won the CAS race. Includes timeout auto-release to prevent stuck flags.
func (c *Compressor) tryStartCompress(sessionID string) bool {
	c.compressMu.Lock()
	defer c.compressMu.Unlock()
	// Timeout auto-release: prevent stuck flag
	if c.compressing.Load() && time.Since(c.compressStart) > c.compressTimeout {
		c.compressing.Store(false)
	}
	if c.compressing.Load() {
		return false
	}
	if c.compressing.CompareAndSwap(false, true) {
		c.compressStart = time.Now()
		return true
	}
	return false
}

func (c *Compressor) finishCompress() {
	c.compressing.Store(false)
}

// CompressStatus returns the current compression status for a session.
func (c *Compressor) CompressStatus(ctx context.Context, sessionID string) (string, error) {
	if c.compressing.Load() {
		return "compressing", nil
	}
	sess, err := c.sessionReader.GetSessionByID(ctx, sessionID)
	if err != nil {
		return "normal", err
	}
	// Check if there's a recent summary
	if ts, err := c.summaryReader.LatestSessionSummaryTime(ctx, sessionID); err == nil && ts != "" {
		if t, err := time.Parse(time.RFC3339, ts); err == nil && time.Since(t) < 2*time.Minute {
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

func (c *Compressor) runCompress(ctx context.Context, sessionID, trpcUserID string, ag biz.Agent, skipMinGap bool) error {
	if !sessionCompressEnabled(ag) {
		return nil
	}
	sess, err := c.sessionReader.GetSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}

	// Calculate effective_budget thresholds.
	window := llmcontext.ResolveWindow(llmcontext.ResolveInput{
		SessionDefaultWindow: sess.LastContextWindowTokens,
		AgentWindow:          ag.ContextWindow,
	})
	softTok := softTriggerTokens(ag, window)
	hardTok := hardTriggerTokens(ag, window)

	usedTokens := sess.ContextUsedTokens

	// Below soft trigger: nothing to do.
	if usedTokens < softTok {
		return nil
	}

	// Debounce check for soft trigger (non-forced).
	if usedTokens < hardTok && !skipMinGap && !atFullContextUsage(sess) {
		minGap := compressMinGapFromAgent(ag)
		if ts, err := c.summaryReader.LatestSessionSummaryTime(ctx, sessionID); err == nil {
			if compressDebounceActive(ts, minGap, time.Now()) {
				return nil
			}
		}
	}

	// Try to acquire compressing flag.
	if !c.tryStartCompress(sessionID) {
		c.lg.Info("压缩已在进行中，跳过", loggateway.StepID("session.compress"), loggateway.SessionID(sessionID))
		return nil
	}
	defer c.finishCompress()

	// Load messages for compression.
	maxSummarized, err := c.summaryReader.MaxSessionSummaryToTurn(ctx, sessionID)
	if err != nil {
		return err
	}
	msgs, err := c.messageReader.ListMessagesAfterTurn(ctx, sessionID, maxSummarized)
	if err != nil {
		return err
	}
	timeline := timelineUserAssistant(msgs)
	if len(timeline) == 0 {
		return nil
	}

	_, keepTurns := compressThresholdAndKeep(ag)
	keepRows := 2 * max(1, keepTurns)
	if len(timeline) <= keepRows {
		return nil
	}
	split := len(timeline) - keepRows
	cutoffTurn := timeline[split-1].TurnNumber

	var body []biz.ChatMessage
	for _, m := range timeline {
		if m.TurnNumber > maxSummarized && m.TurnNumber <= cutoffTurn {
			body = append(body, m)
		}
	}
	if len(body) == 0 {
		return nil
	}

	// Three-level compression cascade: MicroCompact → MemoryCompact → LLM.
	level, md := c.compressCascade(ctx, sess, ag, body, sessionID, cutoffTurn, usedTokens, hardTok)
	if level == compressLevelNone || md == "" {
		return nil
	}

	fromTurn := body[0].TurnNumber
	toTurn := body[len(body)-1].TurnNumber

	versionBeforeCAS := sess.CompressVersion
	oldVersion, casErr := c.compressRepo.TryIncrementCompressVersion(ctx, sessionID)
	if casErr != nil {
		return casErr
	}
	if oldVersion != versionBeforeCAS {
		return nil
	}

	exists, existsErr := c.summaryWriter.SessionSummaryExists(ctx, sessionID, fromTurn, toTurn)
	if existsErr != nil && c.EventBus != nil {
		c.lg.Warn("幂等检查失败",
			loggateway.StepID("session.compress"),
			loggateway.SessionID(sessionID),
			loggateway.Err(existsErr))
	}
	if exists {
		return nil
	}

	var txMerged string
	var txTail []biz.ChatMessage
	var txErr error
	txErr = c.compressRepo.CompressSessionInTx(ctx, sessionID, func(txCtx context.Context) error {
		row := biz.SessionSummary{
			ID:              uuid.NewString(),
			SessionID:       sessionID,
			SummaryMarkdown: md,
			FromTurn:        fromTurn,
			ToTurn:          toTurn,
			TokenEstimate:   roughTokenEstimate(md),
			CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		}
		if err := c.summaryWriter.InsertSessionSummary(txCtx, row); err != nil {
			return err
		}

		allRows, err := c.summaryReader.ListSessionSummaries(txCtx, sessionID)
		if err != nil {
			return err
		}
		txMerged = mergeSessionSummariesMarkdown(allRows)

		for _, m := range timeline {
			if m.TurnNumber > cutoffTurn {
				txTail = append(txTail, m)
			}
		}

		author := c.resolveAgentAuthor(txCtx, ag, sess.AgentID)

		raw, err := RewriteSnapshotWithCompression(sess.RunnerSnapshotJSON, txMerged, txTail, author)
		if err != nil {
			return err
		}
		if err := c.contextUpdater.UpdateRunnerSnapshotJSON(txCtx, sessionID, raw); err != nil {
			return err
		}

		win := llmcontext.ResolveWindow(llmcontext.ResolveInput{
			SessionDefaultWindow: sess.LastContextWindowTokens,
			AgentWindow:          ag.ContextWindow,
		})
		est := estimateCompactedPromptTokens(txMerged, txTail)
		if err := c.contextUpdater.UpdateSessionContextAfterCompression(txCtx, sessionID, est, win); err != nil {
			return err
		}

		preview := firstSummaryLine(txMerged)
		if preview != "" {
			if err := c.summaryWriter.UpdateSessionListSummary(txCtx, sessionID, preview); err != nil {
				c.lg.Warn("update list summary failed", loggateway.StepID("session.compress"), loggateway.SessionID(sessionID), loggateway.Err(err))
			}
		}

		return nil
	})
	if txErr != nil {
		return txErr
	}

	if c.Runtime != nil {
		author := c.resolveAgentAuthor(ctx, ag, sess.AgentID)
		raw, snapErr := RewriteSnapshotWithCompression(sess.RunnerSnapshotJSON, txMerged, txTail, author)
		if snapErr == nil {
			if syncErr := c.Runtime.SyncRunnerSnapshot(ctx, trpcUserID, sessionID, raw, txMerged); syncErr != nil && c.EventBus != nil {
				c.lg.Warn("trpc 快照同步失败",
					loggateway.StepID("session.compress"),
					loggateway.SessionID(sessionID),
					loggateway.Err(syncErr),
					loggateway.Str("user_id", trpcUserID))
			}
		}
	}

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
	}

	return nil
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
	if err := c.messageWriter.AppendChatMessage(ctx, sessionID, sysMsg, false); err != nil {
		c.lg.Warn("append compress notice message failed", loggateway.StepID("session.compress"), loggateway.SessionID(sessionID), loggateway.Err(err))
	}
	if c.EventBus == nil {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeTextDone, "system", sessionID)
	env.Content = &event.EnvelopeContent{Text: text, IsPartial: false}
	env.Metadata = map[string]any{
		"kind":                "system.session.compress",
		"from_turn":           fromTurn,
		"to_turn":             toTurn,
		"context_used_tokens": contextUsedTokens,
		"context_window":      contextWindow,
		"context_used_ratio":  ratio,
		"context_status":      status,
	}
	c.EventBus.Publish(ctx, env)
}

func (c *Compressor) resyncSessionMemory(ctx context.Context, sessionID string) {
	if c == nil || c.Memory == nil {
		return
	}
	if err := c.Memory.DeleteSessionEventEntities(ctx, sessionID); err != nil {
		c.lg.Warn("delete session event entities failed", loggateway.StepID("session.compress"), loggateway.SessionID(sessionID), loggateway.Err(err))
	}
}
