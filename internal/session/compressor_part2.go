package session

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// llmCallWithRetry calls the LLM compressor up to llmCompressMaxAttempts times.
// 重试条件：瞬态错误 / 空摘要 / 退化摘要；确定性错误与 ctx 取消立即终止。
// 返回最终 md（空=失败）、最后一次成功响应 res、失败终态 kind（none=成功）、
// 以及成功结果是否来自压缩缓存。
func (c *Compressor) llmCallWithRetry(ctx context.Context, sessionID string, req compress.Request, transcriptRunes int) (md string, res compress.Result, fail compressFailureKind, cacheHit bool) {
	for attempt := 1; attempt <= llmCompressMaxAttempts; attempt++ {
		if ctx.Err() != nil {

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
