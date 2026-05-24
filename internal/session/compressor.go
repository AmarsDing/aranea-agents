package session

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

// MemoryResync clears derived memory entities after snapshot rewrite.
type MemoryResync interface {
	DeleteSessionEventEntities(ctx context.Context, sessionID string) error
}

// Compressor runs asynchronous session-context compression and syncs snapshots to trpc session state.
type Compressor struct {
	Sessions *biz.SessionUsecase
	Agents   biz.AgentRepository
	Runtime  *Runtime
	Compress compress.Compressor
	Memory   MemoryResync
	EventBus event.Bus

	inFlight sync.Map
}

var _ biz.NativeTurnCompressor = (*Compressor)(nil)
var _ biz.DurableTurnCompressor = (*Compressor)(nil)

func NewCompressor(
	sessions *biz.SessionUsecase,
	agents biz.AgentRepository,
	runtime *Runtime,
	memory MemoryResync,
	comp compress.Compressor,
	eventBus event.Bus,
) *Compressor {
	return &Compressor{
		Sessions: sessions,
		Agents:   agents,
		Runtime:  runtime,
		Memory:   memory,
		Compress: comp,
		EventBus: eventBus,
	}
}

func (c *Compressor) AfterNativeTurn(ctx context.Context, sessionID string, ag biz.Agent) {
	if c == nil || c.Sessions == nil || c.Compress == nil {
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
			event.SessionSysLogWarn(runCtx, sid, "system.session.compress", "会话压缩失败",
				event.P("error", err.Error()))
		}
	})
}

func (c *Compressor) BeforeDurableTurn(ctx context.Context, sessionID string, ag biz.Agent) error {
	if c == nil || c.Sessions == nil || c.Compress == nil {
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
		event.SessionSysLogWarn(runCtx, sid, "system.session.compress", "Durable turn 前压缩失败",
			event.P("error", err.Error()))
	}
	return nil
}

func (c *Compressor) runCompress(ctx context.Context, sessionID, trpcUserID string, ag biz.Agent, skipMinGap bool) error {
	sess, err := c.Sessions.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	threshold, keepTurns := compressThresholdAndKeep(ag)
	if sess.ContextUsedRatio < threshold {
		return nil
	}

	if !skipMinGap && !atFullContextUsage(sess) {
		minGap := compressMinGapFromAgent(ag)
		if ts, err := c.Sessions.LatestSessionSummaryTime(ctx, sessionID); err == nil {
			if compressDebounceActive(ts, minGap, time.Now()) {
				return nil
			}
		}
	}

	maxSummarized, err := c.Sessions.MaxSessionSummaryToTurn(ctx, sessionID)
	if err != nil {
		return err
	}
	msgs, err := c.Sessions.ListMessagesAfterTurn(ctx, sessionID, maxSummarized)
	if err != nil {
		return err
	}
	timeline := timelineUserAssistant(msgs)
	if len(timeline) == 0 {
		return nil
	}

	keepRows := 2 * max(1, keepTurns)
	if len(timeline) <= keepRows {
		return nil
	}
	split := len(timeline) - keepRows
	cutoffTurn := timeline[split-1].TurnIndex

	var body []biz.ChatMessage
	for _, m := range timeline {
		if m.TurnIndex > maxSummarized && m.TurnIndex <= cutoffTurn {
			body = append(body, m)
		}
	}
	if len(body) == 0 {
		return nil
	}

	transcript := buildCompressTranscript(body)
	cProv, cMod := compressProviderModel(sess, ag)
	t0 := time.Now()
	res, err := c.Compress.Compress(ctx, compress.Request{
		Transcript: transcript,
		Provider:   cProv,
		Model:      cMod,
	})
	if err != nil {
		return err
	}
	md := strings.TrimSpace(res.Markdown)
	if md == "" {
		return nil
	}
	if c.EventBus != nil {
		event.SessionSysLogInfo(ctx, sessionID, "system.session.compress", "会话压缩完成",
			event.P("compress_provider", res.Provider),
			event.P("compress_model", res.Model),
			event.P("prompt_tokens", res.PromptTokens),
			event.P("completion_tokens", res.CompletionTokens),
			event.P("duration_ms", time.Since(t0).Milliseconds()),
			event.P("prompt_ver", res.PromptVersion))
	}

	fromTurn := body[0].TurnIndex
	toTurn := body[len(body)-1].TurnIndex
	row := biz.SessionSummary{
		ID:              uuid.NewString(),
		SessionID:       sessionID,
		SummaryMarkdown: md,
		FromTurn:        fromTurn,
		ToTurn:          toTurn,
		TokenEstimate:   roughTokenEstimate(md + transcript),
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	if err := c.Sessions.InsertSessionSummary(ctx, row); err != nil {
		return err
	}

	allRows, err := c.Sessions.ListSessionSummaries(ctx, sessionID)
	if err != nil {
		return err
	}
	merged := mergeSessionSummariesMarkdown(allRows)

	var tail []biz.ChatMessage
	for _, m := range timeline {
		if m.TurnIndex > cutoffTurn {
			tail = append(tail, m)
		}
	}

	author := strings.TrimSpace(ag.AgentKey)
	if c.Agents != nil && strings.TrimSpace(sess.AgentID) != "" {
		if a, e := c.Agents.GetAgentByID(ctx, sess.AgentID); e == nil {
			if k := strings.TrimSpace(a.AgentKey); k != "" {
				author = k
			}
		}
	}
	if author == "" {
		author = "agent"
	}

	raw, err := RewriteSnapshotWithCompression(sess.RunnerSnapshotJSON, merged, tail, author)
	if err != nil {
		return err
	}
	if err := c.Sessions.UpdateRunnerSnapshotJSON(ctx, sessionID, raw); err != nil {
		return err
	}

	if c.Runtime != nil {
		if syncErr := c.Runtime.SyncRunnerSnapshot(ctx, trpcUserID, sessionID, raw, merged); syncErr != nil && c.EventBus != nil {
			event.SessionSysLogWarn(ctx, sessionID, "system.session.compress", "trpc 快照同步失败",
				event.P("error", syncErr.Error()), event.P("user_id", trpcUserID))
		}
	}

	win := sess.LastContextWindowTokens
	if win <= 0 {
		win = ag.ContextWindow
	}
	if win <= 0 {
		win = 128000
	}
	est := estimateCompactedPromptTokens(merged, tail)
	_ = c.Sessions.UpdateSessionContextAfterCompression(ctx, sessionID, est, win)

	preview := firstSummaryLine(merged)
	if preview != "" {
		_ = c.Sessions.UpdateSessionListSummary(ctx, sessionID, preview)
	}
	c.publishCompressionNotice(ctx, sessionID, fromTurn, toTurn, preview)
	c.resyncSessionMemory(ctx, sessionID)
	return nil
}

func (c *Compressor) publishCompressionNotice(ctx context.Context, sessionID string, fromTurn, toTurn int, preview string) {
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
	_ = c.Sessions.AppendChatMessage(ctx, sessionID, sysMsg, false)
	if c.EventBus == nil {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeTextDone, "system", sessionID)
	env.Content = &event.EnvelopeContent{Text: text, IsPartial: false}
	env.Metadata = map[string]any{
		"kind":      "system.session.compress",
		"from_turn": fromTurn,
		"to_turn":   toTurn,
	}
	c.EventBus.Publish(ctx, env)
}

func (c *Compressor) resyncSessionMemory(ctx context.Context, sessionID string) {
	if c == nil || c.Memory == nil {
		return
	}
	_ = c.Memory.DeleteSessionEventEntities(ctx, sessionID)
}
