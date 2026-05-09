package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/adkdeps"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/adksvc"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"

	"github.com/google/uuid"
)

const (
	sessionCompressMinGap     = 10 * time.Minute
	sessionCompressRunTimeout = 8 * time.Minute
)

// SessionCompressor runs asynchronous session-context compression (SummaryService).
type SessionCompressor struct {
	Sessions *biz.SessionUsecase
	Agents   biz.AgentRepository
	Compress compress.Compressor
	ADK      *adkdeps.Runtime

	MonitorLogs *biz.MonitorLogBroker

	inFlight sync.Map // session id -> bool
}

var _ biz.NativeTurnCompressor = (*SessionCompressor)(nil)

// NewSessionCompressor wires optional SQLite memory through ADK runtime (compress may be nil to disable).
func NewSessionCompressor(
	sessions *biz.SessionUsecase,
	agents biz.AgentRepository,
	adk *adkdeps.Runtime,
	comp compress.Compressor,
	monitorLogs *biz.MonitorLogBroker,
) *SessionCompressor {
	return &SessionCompressor{
		Sessions:    sessions,
		Agents:      agents,
		Compress:    comp,
		ADK:         adk,
		MonitorLogs: monitorLogs,
	}
}

// AfterNativeTurn schedules compression when context usage crosses the agent threshold.
func (c *SessionCompressor) AfterNativeTurn(ctx context.Context, sessionID string, ag biz.Agent) {
	if c == nil || c.Sessions == nil || c.Compress == nil {
		return
	}
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return
	}
	go func() {
		if _, loaded := c.inFlight.LoadOrStore(sid, true); loaded {
			return
		}
		defer c.inFlight.Delete(sid)
		runCtx, cancel := context.WithTimeout(context.Background(), sessionCompressRunTimeout)
		defer cancel()
		if err := c.runCompress(runCtx, sid, ag); err != nil {
			log.Printf("session_compress: session=%s err=%v", sid, err)
			if c.MonitorLogs != nil {
				c.MonitorLogs.Publish(runCtx, "ERROR", fmt.Sprintf("session_compress: session=%s err=%v", sid, err), "session_compress")
			}
		}
	}()
}

func (c *SessionCompressor) runCompress(ctx context.Context, sessionID string, ag biz.Agent) error {
	sess, err := c.Sessions.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	threshold := 0.6
	keepTurns := 4
	if ag.Settings != nil {
		if ag.Settings.L0SummaryThreshold > 0 {
			threshold = ag.Settings.L0SummaryThreshold
		}
		if ag.Settings.L0SummaryKeepTurns > 0 {
			keepTurns = ag.Settings.L0SummaryKeepTurns
		}
	}
	if sess.ContextUsedRatio < threshold {
		return nil
	}

	// At 100% window usage: compress immediately (do not apply min-gap debounce).
	if !atFullContextUsage(sess) {
		if ts, err := c.Sessions.LatestSessionSummaryTime(ctx, sessionID); err == nil && strings.TrimSpace(ts) != "" {
			if t, e := time.Parse(time.RFC3339, strings.TrimSpace(ts)); e == nil {
				if time.Since(t) < sessionCompressMinGap {
					return nil
				}
			}
		}
	}

	msgs, err := c.Sessions.ListMessages(ctx, sessionID)
	if err != nil {
		return err
	}
	timeline := timelineUserAssistant(msgs)
	if len(timeline) == 0 {
		return nil
	}

	maxSummarized, err := c.Sessions.MaxSessionSummaryToTurn(ctx, sessionID)
	if err != nil {
		return err
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
	line := fmt.Sprintf("session_compress: session=%s ok compress_provider=%s compress_model=%s prompt_tokens=%d completion_tokens=%d duration_ms=%d prompt_ver=%s",
		sessionID, res.Provider, res.Model, res.PromptTokens, res.CompletionTokens, time.Since(t0).Milliseconds(), res.PromptVersion)
	log.Println(line)
	if c.MonitorLogs != nil {
		c.MonitorLogs.Publish(ctx, "INFO", line, "session_compress")
	}

	fromTurn := body[0].TurnIndex
	toTurn := body[len(body)-1].TurnIndex
	row := biz.SessionSummary{
		ID:              uuid.NewString(),
		SessionID:       sessionID,
		SummaryMarkdown: md,
		FromTurn:        fromTurn,
		ToTurn:          toTurn,
		TokenEstimate:   chatagent.RoughTokenEstimate(md + transcript),
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

	raw, err := adksvc.RewriteSnapshotWithCompression(sess.AdkSnapshotJSON, merged, tail, author)
	if err != nil {
		return err
	}
	if err := c.Sessions.UpdateAdkSnapshotJSON(ctx, sessionID, raw); err != nil {
		return err
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

	if preview := firstSummaryLine(merged); preview != "" {
		_ = c.Sessions.UpdateSessionListSummary(ctx, sessionID, preview)
	}

	c.resyncSessionMemory(ctx, sessionID)
	return nil
}

func compressProviderModel(sess biz.Session, ag biz.Agent) (prov, mod string) {
	if ag.Settings != nil {
		p := strings.TrimSpace(ag.Settings.L0CompressProvider)
		m := strings.TrimSpace(ag.Settings.L0CompressModel)
		if p != "" && m != "" {
			return p, m
		}
	}
	return firstNonEmpty(sess.Provider, ag.Provider), firstNonEmpty(sess.Model, ag.Model)
}

func (c *SessionCompressor) resyncSessionMemory(ctx context.Context, sessionID string) {
	if c == nil || c.ADK == nil || c.ADK.SessionMemory == nil {
		return
	}
	_ = c.ADK.SessionMemory.DeleteADKSessionEventEntities(ctx, sessionID)

	resolve := func(ctx context.Context, aid string) (string, error) {
		if c.Agents == nil || strings.TrimSpace(aid) == "" {
			return "agent", nil
		}
		a, err := c.Agents.GetAgentByID(ctx, aid)
		if err != nil {
			return "", err
		}
		k := strings.TrimSpace(a.AgentKey)
		if k == "" {
			return "agent", nil
		}
		return k, nil
	}
	ss := adksvc.NewBizSessionForUsecase(c.Sessions, resolve)
	mem := chatagent.NewSessionSQLiteMemoryService(c.ADK.SessionMemory)
	_ = chatagent.SyncPersistedADKSessionToMemory(ctx, ss, mem, adksvc.DefaultAppName, adksvc.DefaultUserID, sessionID)
}

func timelineUserAssistant(msgs []biz.ChatMessage) []biz.ChatMessage {
	var out []biz.ChatMessage
	for _, m := range msgs {
		r := strings.ToLower(strings.TrimSpace(m.Role))
		if r != "user" && r != "assistant" {
			continue
		}
		if strings.TrimSpace(m.ContentMarkdown) == "" {
			continue
		}
		out = append(out, m)
	}
	return out
}

func mergeSessionSummariesMarkdown(rows []biz.SessionSummary) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	for i, r := range rows {
		if i > 0 {
			b.WriteString("\n\n---\n\n")
		}
		b.WriteString(strings.TrimSpace(r.SummaryMarkdown))
	}
	return strings.TrimSpace(b.String())
}

func buildCompressTranscript(msgs []biz.ChatMessage) string {
	var b strings.Builder
	for _, m := range msgs {
		role := strings.ToUpper(strings.TrimSpace(m.Role))
		b.WriteString(role)
		b.WriteString(": ")
		b.WriteString(strings.TrimSpace(m.ContentMarkdown))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func firstSummaryLine(md string) string {
	md = strings.TrimSpace(md)
	if md == "" {
		return ""
	}
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
		t = strings.TrimSpace(t)
		if t != "" {
			r := []rune(t)
			if len(r) > 160 {
				return string(r[:160]) + "…"
			}
			return t
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// atFullContextUsage is true when the last turn filled the model context window (ratio capped at 1 or tokens >= window).
func atFullContextUsage(sess biz.Session) bool {
	if sess.ContextUsedRatio >= 1.0 {
		return true
	}
	if sess.LastContextWindowTokens > 0 && sess.ContextUsedTokens >= sess.LastContextWindowTokens {
		return true
	}
	return false
}

func estimateCompactedPromptTokens(mergedSummary string, tail []biz.ChatMessage) int {
	var b strings.Builder
	b.WriteString(mergedSummary)
	b.WriteString("\n")
	for _, m := range tail {
		b.WriteString(m.ContentMarkdown)
		b.WriteString("\n")
	}
	return chatagent.RoughTokenEstimate(b.String())
}
