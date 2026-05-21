// Package jobs provides cron-style background workers that run alongside the
// main Aranea service.
package jobs

import (
	"context"
	"regexp"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	memtrpc "aranea-agents/internal/memory/trpc"
	servmetrics "aranea-agents/internal/metrics"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

// autoMemoryMaxRetries is the maximum number of extraction attempts per job.
const autoMemoryMaxRetries = 3

// autoMemoryMaxMessages is the maximum number of recent messages to analyze per job.
const autoMemoryMaxMessages = 40

// heuristicPatterns are lightweight regex patterns for fact extraction from
// user messages without requiring an LLM call (EP-RT-03 initial implementation).
var heuristicPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:my name is|I(?:'m| am) called)\s+([A-Z][a-z]+(?: [A-Z][a-z]+)?)`),
	regexp.MustCompile(`(?i)I(?:'m| am)\s+(?:a |an )?([a-z]+(?:\s+[a-z]+)?)\s*(?:\.|,|$)`),
	regexp.MustCompile(`(?i)I\s+(?:prefer|like|love|hate|dislike)\s+([^.!?\n]+)`),
	regexp.MustCompile(`(?i)(?:please|always|never)\s+(?:call me|refer to me as)\s+([^.!?\n]+)`),
}

// AutoMemoryWorker drains the global auto-memory queue every interval and runs
// keyword-based memory extraction for each pending session.
//
// Retry schedule: 30 s / 2 m / 10 m exponential back-off.
// Jobs that exceed maxRetries are marked dead and discarded.
type AutoMemoryWorker struct {
	interval time.Duration
	sessions *biz.SessionUsecase
	agents   *biz.AgentUsecase
	memory   trpcmemory.Service
	l4       biz.L4GraphWriter
}

func NewAutoMemoryWorker(interval time.Duration, sessions *biz.SessionUsecase, agents *biz.AgentUsecase, memory trpcmemory.Service, l4 biz.L4GraphWriter) *AutoMemoryWorker {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &AutoMemoryWorker{interval: interval, sessions: sessions, agents: agents, memory: memory, l4: l4}
}

// Start blocks until ctx is cancelled, draining the queue on each tick.
func (w *AutoMemoryWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.drain(ctx)
		}
	}
}

func (w *AutoMemoryWorker) drain(ctx context.Context) {
	q := memtrpc.GlobalAutoMemoryQueue()
	// Drain at most 50 jobs per tick to avoid starving other work.
	for i := 0; i < 50; i++ {
		select {
		case req := <-q.Chan():
			if ctx.Err() != nil {
				return
			}
			w.processWithRetry(ctx, req)
		default:
			return
		}
	}
}

func (w *AutoMemoryWorker) processWithRetry(ctx context.Context, req memtrpc.AutoMemoryJobRequest) {
	backoffSchedule := []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}
	var lastErr error
	for attempt := 0; attempt < autoMemoryMaxRetries; attempt++ {
		if attempt > 0 {
			delay := backoffSchedule[attempt-1]
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}
		t0 := time.Now()
		err := w.extract(ctx, req)
		duration := time.Since(t0).Seconds()
		if err == nil {
			servmetrics.AutoMemoryJobTotal.WithLabelValues("done").Inc()
			servmetrics.AutoMemoryExtractionDuration.Observe(duration)
			return
		}
		lastErr = err
		event.SessionSysLogWarn(ctx, req.SessionID, "system.auto_memory.extract_fail", "自动记忆提取失败",
			event.P("attempt", attempt+1), event.P("error", err))
	}
	servmetrics.AutoMemoryJobTotal.WithLabelValues("dead").Inc()
	event.SessionSysLogWarn(ctx, req.SessionID, "system.auto_memory.extract_max_retry", "自动记忆提取重试耗尽", event.P("error", lastErr))
}

// extract performs heuristic keyword-based memory extraction for the session.
// EP-RT-03: reads recent messages via SessionUsecase, applies regex heuristics
// to detect user facts, and writes them to the memory store via trpcmemory.Service.
// When sessions or memory is nil, the function logs and returns without error.
func (w *AutoMemoryWorker) extract(ctx context.Context, req memtrpc.AutoMemoryJobRequest) error {
	sid := strings.TrimSpace(req.SessionID)
	if sid == "" {
		return nil
	}
	if w.sessions == nil || w.memory == nil {
		event.SysLogDebug("system.auto_memory.extract_fail", "自动记忆跳过：未注入 sessions/memory", event.P("session_id", sid))
		return nil
	}

	sess, err := w.sessions.Get(ctx, sid)
	if err != nil {
		return err
	}
	agentID := strings.TrimSpace(sess.AgentID)
	l4Enabled := false
	if w.agents != nil && agentID != "" {
		if ag, err := w.agents.Get(ctx, agentID); err == nil && ag.Settings != nil {
			l4Enabled = ag.Settings.L4Enabled
		}
	}

	msgs, err := w.sessions.ListMessages(ctx, sid)
	if err != nil {
		return err
	}

	// Only examine the last autoMemoryMaxMessages messages.
	if len(msgs) > autoMemoryMaxMessages {
		msgs = msgs[len(msgs)-autoMemoryMaxMessages:]
	}

	uk := trpcmemory.UserKey{AppName: req.AppName, UserID: req.UserID}
	if uk.UserID == "" {
		uk.UserID = strings.TrimSpace(sess.UserID)
	}
	var added int
	var l4Written int
	for _, msg := range msgs {
		if msg.Role != "user" {
			continue
		}
		text := strings.TrimSpace(msg.ContentMarkdown)
		if text == "" {
			continue
		}
		for _, pat := range heuristicPatterns {
			if m := pat.FindStringSubmatch(text); len(m) > 1 {
				fact := strings.TrimSpace(m[0])
				if err := w.memory.AddMemory(ctx, uk, fact, nil); err != nil {
					event.SessionSysLogWarn(ctx, sid, "system.auto_memory.extract_fail", "自动记忆 AddMemory 失败", event.P("fact", fact), event.P("error", err))
				} else {
					added++
				}
			}
		}
		if l4Enabled && w.l4 != nil && agentID != "" {
			n, err := w.l4.WriteFromUserText(ctx, agentID, uk.UserID, text)
			if err != nil {
				event.SessionSysLogWarn(ctx, sid, "system.auto_memory.l4_fail", "L4 图谱写入失败", event.P("error", err))
			} else {
				l4Written += n
			}
		}
	}

	event.SessionSysLogInfo(ctx, sid, "system.auto_memory.done", "自动记忆提取完成",
		event.P("messages_scanned", len(msgs)),
		event.P("facts_added", added),
		event.P("l4_entities", l4Written),
	)
	return nil
}
