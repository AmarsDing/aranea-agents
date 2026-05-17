package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/safego"
	"aranea-agents/pkg/strutil"

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
	Persist  rt.PersistenceSet
	EventBus event.Bus

	inFlight sync.Map
}

var _ biz.NativeTurnCompressor = (*SessionCompressor)(nil)

func NewSessionCompressor(
	sessions *biz.SessionUsecase,
	agents biz.AgentRepository,
	persist rt.PersistenceSet,
	comp compress.Compressor,
	eventBus event.Bus,
) *SessionCompressor {
	return &SessionCompressor{
		Sessions: sessions,
		Agents:   agents,
		Compress: comp,
		Persist:  persist,
		EventBus: eventBus,
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
	safego.Go(ctx, "session-compress", func() {
		if _, loaded := c.inFlight.LoadOrStore(sid, true); loaded {
			return
		}
		defer c.inFlight.Delete(sid)
		runCtx, cancel := context.WithTimeout(context.Background(), sessionCompressRunTimeout)
		defer cancel()
		if err := c.runCompress(runCtx, sid, ag); err != nil {
			_ = fmt.Sprintf("session_compress: session=%s err=%v", sid, err) // logged via EventBus below
			if c.EventBus != nil {
				env := event.NewEnvelope(event.EnvelopeTypeLog, "session_compress", sid)
				env.Metadata = map[string]any{"level": "ERROR", "source": "session_compress"}
				env.Content = &event.EnvelopeContent{Text: fmt.Sprintf("session_compress: session=%s err=%v", sid, err)}
				c.EventBus.Publish(runCtx, env)
			}
		}
	})
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
	if c.EventBus != nil {
		env := event.NewEnvelope(event.EnvelopeTypeLog, "session_compress", sessionID)
		env.Metadata = map[string]any{"level": "INFO", "source": "session_compress"}
		env.Content = &event.EnvelopeContent{Text: line}
		c.EventBus.Publish(ctx, env)
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

	raw, err := rewriteSnapshotWithCompression(sess.RunnerSnapshotJSON, merged, tail, author)
	if err != nil {
		return err
	}
	if err := c.Sessions.UpdateRunnerSnapshotJSON(ctx, sessionID, raw); err != nil {
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
	return strutil.FirstNonEmpty(sess.DefaultProvider, ag.Provider), strutil.FirstNonEmpty(sess.DefaultModel, ag.Model)
}

func (c *SessionCompressor) resyncSessionMemory(ctx context.Context, sessionID string) {
	if c == nil || c.Persist.Memory == nil {
		return
	}
	_ = c.Persist.Memory.DeleteSessionEventEntities(ctx, sessionID)
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

func rewriteSnapshotWithCompression(snapshotJSON string, mergedSummariesMarkdown string, tail []biz.ChatMessage, assistantAuthor string) (string, error) {
	snapshotJSON = strings.TrimSpace(snapshotJSON)
	if snapshotJSON == "" {
		snapshotJSON = "{}"
	}
	var bundle map[string]any
	if err := json.Unmarshal([]byte(snapshotJSON), &bundle); err != nil {
		return "", err
	}
	if bundle == nil {
		bundle = map[string]any{}
	}
	summaryEvent := map[string]any{
		"author":    "user",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"content":   "[Conversation summary — earlier turns compressed]\n\n" + strings.TrimSpace(mergedSummariesMarkdown),
		"role":      "system",
	}
	var tailEvents []any
	for _, m := range tail {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		author := role
		if role == "assistant" {
			author = strings.TrimSpace(assistantAuthor)
			if author == "" {
				author = "agent"
			}
		}
		tailEvents = append(tailEvents, map[string]any{
			"author":    author,
			"timestamp": m.CreatedAt,
			"content":   strings.TrimSpace(m.ContentMarkdown),
			"role":      role,
		})
	}
	var events []any
	events = append(events, summaryEvent)
	events = append(events, tailEvents...)
	bundle["events"] = events
	bundle["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	out, err := json.Marshal(bundle)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func NewCompressHTTPClient() *http.Client {
	return &http.Client{Timeout: 120 * time.Second}
}
