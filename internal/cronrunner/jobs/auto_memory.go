// Package jobs provides cron-style background workers that run alongside the
// main Aranea service.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
	"aranea-agents/internal/event"
	memtrpc "aranea-agents/internal/memory/trpc"
	servmetrics "aranea-agents/internal/metrics"
)

// autoMemoryMaxRetries is the maximum number of extraction attempts per job.
const autoMemoryMaxRetries = 3

// autoMemoryMaxMessages is the maximum number of recent messages to analyze per job.
const autoMemoryMaxMessages = 40

const autoMemoryDrainBatchSize = 50

// AutoMemoryWorker drains the global auto-memory queue every interval and runs
// memory consolidation for each pending session.
//
// Retry schedule: 30 s / 2 m / 10 m exponential back-off.
// Jobs that exceed maxRetries are marked dead and discarded.
type AutoMemoryWorker struct {
	interval     time.Duration
	sessions     *biz.SessionUsecase
	agents       *biz.AgentUsecase
	memStore     *sessionmemory.Store
	indexSync    biz.MemoryFactIndexSyncer
	episodeSync  biz.EpisodeIndexSyncer
	l4           biz.L4GraphWriter
	consolidator biz.MemoryConsolidator
	feedback     biz.MemoryConsolidator
	queue        memtrpc.AutoMemoryQueue
}

func NewAutoMemoryWorker(
	interval time.Duration,
	sessions *biz.SessionUsecase,
	agents *biz.AgentUsecase,
	memStore *sessionmemory.Store,
	indexSync biz.MemoryFactIndexSyncer,
	episodeSync biz.EpisodeIndexSyncer,
	l4 biz.L4GraphWriter,
	consolidator biz.MemoryConsolidator,
	queue memtrpc.AutoMemoryQueue,
) (*AutoMemoryWorker, error) {
	if queue == nil {
		return nil, errors.New("jobs: auto memory queue is required")
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if consolidator == nil {
		consolidator = biz.DefaultMemoryConsolidator(nil)
	}
	return &AutoMemoryWorker{
		interval:     interval,
		sessions:     sessions,
		agents:       agents,
		memStore:     memStore,
		indexSync:    indexSync,
		episodeSync:  episodeSync,
		l4:           l4,
		consolidator: consolidator,
		feedback:     biz.NewFeedbackConsolidator(),
		queue:        queue,
	}, nil
}

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
	q := w.queue
	if q == nil {
		return
	}
	for i := 0; i < autoMemoryDrainBatchSize; i++ {
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
			biz.MemoryWorkerStatsGlobal().RecordJobDone(int64(duration * 1000))
			return
		}
		lastErr = err
		event.SessionSysLogWarn(ctx, req.SessionID, "system.auto_memory.extract_fail", "自动记忆提取失败",
			event.P("attempt", attempt+1), event.P("error", err))
	}
	servmetrics.AutoMemoryJobTotal.WithLabelValues("dead").Inc()
	biz.MemoryWorkerStatsGlobal().RecordJobDead()
	event.SessionSysLogWarn(ctx, req.SessionID, "system.auto_memory.extract_max_retry", "自动记忆提取重试耗尽", event.P("error", lastErr))
}

func (w *AutoMemoryWorker) extract(ctx context.Context, req memtrpc.AutoMemoryJobRequest) error {
	if strings.TrimSpace(req.FeedbackMessageID) != "" {
		return w.extractFeedback(ctx, req)
	}
	sid := strings.TrimSpace(req.SessionID)
	if sid == "" {
		return nil
	}
	if w.sessions == nil || w.memStore == nil || w.consolidator == nil {
		event.SysLogDebug("system.auto_memory.extract_fail", "自动记忆跳过：未注入 sessions/memStore/consolidator", event.P("session_id", sid))
		return nil
	}

	sess, err := w.sessions.Get(ctx, sid)
	if err != nil {
		return err
	}
	agentID := strings.TrimSpace(sess.AgentID)
	var memoryPolicy biz.MemoryRuntimePolicy
	if w.agents != nil && agentID != "" {
		if ag, err := w.agents.Get(ctx, agentID); err == nil && ag.Settings != nil {
			memoryPolicy = biz.ResolveMemoryRuntimePolicy(ag.Settings)
		}
	}
	if !memoryPolicy.AnyWrite() {
		return nil
	}

	msgs, err := w.sessions.ListMessagesRecent(ctx, sid, autoMemoryMaxMessages)
	if err != nil {
		return err
	}

	appName := strings.TrimSpace(agentID)
	if appName == "" {
		appName = strings.TrimSpace(req.AppName)
	}
	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		userID = strings.TrimSpace(sess.UserID)
	}

	in := biz.ConsolidateInput{
		SessionID: sid,
		AgentID:   agentID,
		UserID:    userID,
		AppName:   appName,
	}
	for _, msg := range msgs {
		in.Messages = append(in.Messages, biz.ConsolidateMessage{
			Role:      msg.Role,
			Content:   msg.ContentMarkdown,
			MessageID: msg.ID,
		})
	}

	proposals, err := biz.ExtractWithFallbackHook(w.consolidator, ctx, in, func(primaryErr error) {
		servmetrics.AutoMemoryLLMFallbackTotal.Inc()
		biz.MemoryWorkerStatsGlobal().RecordLLMFallback()
		if primaryErr != nil && !errors.Is(primaryErr, biz.ErrLLMExtractorUnavailable) {
			event.SessionSysLogWarn(ctx, sid, "system.auto_memory.llm_fallback", "LLM 记忆提取失败，已降级启发式",
				event.P("error", primaryErr))
		}
	})
	if err != nil {
		return err
	}

	lastUserMsgID := lastUserMessageID(msgs)
	msgIDsWithL3 := make(map[string]struct{})
	var factInputs []sessionmemory.MemoryFactUpsert
	if memoryPolicy.WriteL3Facts {
		for _, p := range proposals {
			stmt := strings.TrimSpace(p.Statement)
			if stmt == "" {
				continue
			}
			msgID := strings.TrimSpace(p.SourceMessageID)
			if msgID == "" {
				msgID = lastUserMsgID
			}
			if msgID != "" {
				msgIDsWithL3[msgID] = struct{}{}
			}
			factInputs = append(factInputs, sessionmemory.MemoryFactUpsert{
				ScopeType:       "agent",
				ScopeID:         appName,
				UserID:          userID,
				AgentID:         appName,
				Statement:       stmt,
				DetailsMarkdown: stmt,
				FactKind:        "fact",
				TagsJSON:        topicsJSON(p.Topics),
				Confidence:      0.85,
				Importance:      0.6,
				SourceKind:      "auto_memory",
				SourceSessionID: sid,
				SourceMessageID: msgID,
				Status:          "active",
				MetadataJSON:    `{"source":"auto_memory"}`,
			})
		}
	}

	var ep *sessionmemory.EpisodeInsert
	if memoryPolicy.WriteL2Episode && len(factInputs) > 0 {
		added := len(factInputs)
		title := "Auto-memory consolidation"
		if added == 1 {
			title = previewText(factInputs[0].Statement, 120)
		}
		summary := previewText(buildEpisodeSummary(proposals, added), 500)
		ep = &sessionmemory.EpisodeInsert{
			SessionID:      sid,
			AgentID:        agentID,
			UserID:         userID,
			Title:          title,
			OutcomeSummary: summary,
			Importance:     0.55,
			MessageCount:   len(msgs),
			ConsolidatedL3: added,
			MetadataJSON:   `{"source":"auto_memory"}`,
		}
	}

	writeResult, err := w.memStore.UpsertFactsAndEpisodeBatch(ctx, factInputs, ep)
	if err != nil {
		event.SessionSysLogWarn(ctx, sid, "system.auto_memory.extract_fail", "自动记忆巩固写入失败",
			event.P("error", err))
		return err
	}
	added := writeResult.FactsWritten
	for _, raw := range writeResult.FactRows {
		if w.indexSync != nil {
			if serr := w.indexSync.SyncFactIndexFromRow(ctx, raw); serr != nil {
				event.SysLogWarn("system.auto_memory.l4_fail", "auto_memory index sync failed", event.P("error", serr.Error()))
			}
		}
	}
	if w.episodeSync != nil && len(writeResult.EpisodeRow) > 0 {
		var epRow map[string]any
		if json.Unmarshal(writeResult.EpisodeRow, &epRow) == nil {
			epID, _ := epRow["id"].(string)
			title, _ := epRow["title"].(string)
			summary, _ := epRow["outcome_summary"].(string)
			if epID != "" {
				_ = w.episodeSync.SyncEpisodeIndex(ctx, agentID, epID, title, summary)
			}
		}
	}

	var l4Written int
	if memoryPolicy.WriteL4Graph && w.l4 != nil && agentID != "" {
		for _, msg := range msgs {
			if msg.Role != "user" {
				continue
			}
			if _, skip := msgIDsWithL3[msg.ID]; skip {
				continue
			}
			text := strings.TrimSpace(msg.ContentMarkdown)
			if text == "" {
				continue
			}
			n, err := w.l4.WriteFromUserText(ctx, agentID, userID, text)
			if err != nil {
				event.SessionSysLogWarn(ctx, sid, "system.auto_memory.l4_fail", "L4 图谱写入失败", event.P("error", err))
			} else {
				l4Written += n
			}
		}
		if l4Written > 0 {
			w.l4.RunDecay(ctx, agentID)
		}
	}

	event.SessionSysLogInfo(ctx, sid, "system.auto_memory.done", "自动记忆提取完成",
		event.P("messages_scanned", len(msgs)),
		event.P("facts_added", added),
		event.P("l4_entities", l4Written),
	)
	return nil
}

func (w *AutoMemoryWorker) extractFeedback(ctx context.Context, req memtrpc.AutoMemoryJobRequest) error {
	sid := strings.TrimSpace(req.SessionID)
	msgID := strings.TrimSpace(req.FeedbackMessageID)
	rating := strings.TrimSpace(req.FeedbackRating)
	comment := strings.TrimSpace(req.FeedbackComment)
	if sid == "" || msgID == "" || rating == "" {
		return nil
	}
	if w.sessions == nil || w.memStore == nil || w.feedback == nil {
		event.SysLogDebug("system.auto_memory.feedback_skip", "反馈记忆跳过：未注入 sessions/memStore", event.P("session_id", sid))
		return nil
	}
	sess, err := w.sessions.Get(ctx, sid)
	if err != nil {
		return err
	}
	msgs, err := w.sessions.ListMessagesRecent(ctx, sid, autoMemoryMaxMessages)
	if err != nil {
		return err
	}
	var assistantPreview string
	for _, m := range msgs {
		if m.ID == msgID {
			assistantPreview = previewText(m.ContentMarkdown, 200)
			break
		}
	}
	statement := biz.BuildFeedbackStatement(rating, comment, assistantPreview)
	if statement == "" {
		return nil
	}
	appName := strings.TrimSpace(req.AppName)
	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		userID = strings.TrimSpace(sess.UserID)
	}
	if appName == "" {
		appName = strings.TrimSpace(sess.AgentID)
	}

	proposals, err := w.feedback.Extract(ctx, biz.ConsolidateInput{
		SessionID: sid,
		Messages:  []biz.ConsolidateMessage{{Role: "feedback", Content: statement, MessageID: msgID}},
	})
	if err != nil {
		return err
	}
	var facts []sessionmemory.MemoryFactUpsert
	for _, p := range proposals {
		stmt := strings.TrimSpace(p.Statement)
		if stmt == "" {
			continue
		}
		facts = append(facts, sessionmemory.MemoryFactUpsert{
			ScopeType:       "agent",
			ScopeID:         appName,
			UserID:          userID,
			AgentID:         appName,
			Statement:       stmt,
			DetailsMarkdown: stmt,
			FactKind:        "fact",
			TagsJSON:        topicsJSON(p.Topics),
			Confidence:      0.85,
			Importance:      0.6,
			SourceKind:      "auto_memory",
			SourceSessionID: sid,
			SourceMessageID: msgID,
			Status:          "active",
			MetadataJSON:    `{"source":"auto_memory"}`,
		})
	}
	if len(facts) == 0 {
		return nil
	}
	writeResult, err := w.memStore.UpsertFactsAndEpisodeBatch(ctx, facts, nil)
	if err != nil {
		return err
	}
	for _, raw := range writeResult.FactRows {
		if w.indexSync != nil {
			if serr := w.indexSync.SyncFactIndexFromRow(ctx, raw); serr != nil {
				event.SysLogWarn("system.auto_memory.l4_fail", "feedback_memory index sync failed", event.P("error", serr.Error()))
			}
		}
	}
	event.SessionSysLogInfo(ctx, sid, "system.auto_memory.feedback_done", "反馈偏好记忆已写入",
		event.P("message_id", msgID), event.P("rating", rating))
	return nil
}

func lastUserMessageID(msgs []biz.ChatMessage) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" && strings.TrimSpace(msgs[i].ID) != "" {
			return msgs[i].ID
		}
	}
	return ""
}

func buildEpisodeSummary(proposals []biz.MemoryProposal, added int) string {
	parts := make([]string, 0, added)
	for _, p := range proposals {
		if s := strings.TrimSpace(p.Statement); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "; ")
}

func topicsJSON(topics []string) string {
	if len(topics) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(topics)
	return string(b)
}

func previewText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
