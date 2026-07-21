// Package jobs provides cron-style background workers that run alongside the
// main Aranea service.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	memtrpc "aranea-agents/internal/memory/trpc"
	servmetrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// AutoMemoryWorker drains the global auto-memory queue every interval and runs
// memory consolidation for each pending session.
//
// Retry schedule: 30 s / 2 m / 10 m exponential back-off.
// Jobs that exceed maxRetries are marked dead and discarded.
type AutoMemoryWorker struct {
	interval     time.Duration
	sessions     *biz.SessionUsecase
	agents       *biz.AgentUsecase
	writer       biz.MemoryConsolidationWriter
	indexSync    biz.MemoryFactIndexSyncer
	episodeSync  biz.EpisodeIndexSyncer
	l4           biz.L4GraphWriter
	consolidator biz.MemoryConsolidator
	feedback     biz.MemoryConsolidator
	queue        memtrpc.AutoMemoryQueue
	deadLetter   biz.MemoryDeadLetterSink
	memConf      conf.RuntimeAutoMemoryConfig
	stats        *biz.MemoryWorkerStats
	lg           loggateway.Logger
}

// AutoMemoryWorkerConfig holds all dependencies for AutoMemoryWorker.
// Using a config struct instead of positional parameters improves readability
// and makes future additions non-breaking.
type AutoMemoryWorkerConfig struct {
	RuntimeConf  *conf.Runtime
	Interval     time.Duration
	Sessions     *biz.SessionUsecase
	Agents       *biz.AgentUsecase
	Writer       biz.MemoryConsolidationWriter
	IndexSync    biz.MemoryFactIndexSyncer
	EpisodeSync  biz.EpisodeIndexSyncer
	L4           biz.L4GraphWriter
	Consolidator biz.MemoryConsolidator
	Queue        memtrpc.AutoMemoryQueue
	// DeadLetterSink receives jobs that exhausted all retries (P2-03).
	// When nil, retry-exhausted jobs are only logged/metered (legacy behavior).
	DeadLetterSink biz.MemoryDeadLetterSink
	Stats          *biz.MemoryWorkerStats
	Logger         loggateway.Logger
}

// NewAutoMemoryWorker creates an AutoMemoryWorker. // WIRE: needs *conf.Runtime
func NewAutoMemoryWorker(cfg AutoMemoryWorkerConfig) (*AutoMemoryWorker, error) {
	if cfg.Queue == nil {
		return nil, errors.New("jobs: auto memory queue is required")
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	consolidator := cfg.Consolidator
	if consolidator == nil {
		consolidator = biz.DefaultMemoryConsolidator(nil)
	}
	return &AutoMemoryWorker{
		interval:     interval,
		sessions:     cfg.Sessions,
		agents:       cfg.Agents,
		writer:       cfg.Writer,
		indexSync:    cfg.IndexSync,
		episodeSync:  cfg.EpisodeSync,
		l4:           cfg.L4,
		consolidator: consolidator,
		feedback:     biz.NewFeedbackConsolidator(),
		queue:        cfg.Queue,
		deadLetter:   cfg.DeadLetterSink,
		memConf:      cfg.RuntimeConf.AutoMemoryConfig(),
		stats:        cfg.Stats,
		lg:           cfg.Logger,
	}, nil
}

func (w *AutoMemoryWorker) Start(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			w.lg.Error("AutoMemoryWorker panic recovered, worker stopped",
				loggateway.Err(fmt.Errorf("%v", r)))
		}
	}()
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
	for i := 0; i < int(w.memConf.DrainBatchSize); i++ {
		select {
		case req := <-q.Chan():
			if ctx.Err() != nil {
				// AckDone here because we return before entering processWithRetry
				// which has its own defer AckDone — without this, the tenant
				// in-flight counter would permanently leak.
				w.queue.AckDone(req)
				return
			}
			w.processWithRetry(ctx, req)
		default:
			return
		}
	}
}

func (w *AutoMemoryWorker) processWithRetry(ctx context.Context, req memtrpc.AutoMemoryJobRequest) {
	defer w.queue.AckDone(req)

	backoffSchedule := []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}
	var lastErr error
	for attempt := 0; attempt < int(w.memConf.MaxRetries); attempt++ {
		if attempt > 0 {
			bo := backoffSchedule[len(backoffSchedule)-1]
			if attempt-1 < len(backoffSchedule) {
				bo = backoffSchedule[attempt-1]
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(bo):
			}
		}
		t0 := time.Now()
		err := w.extract(ctx, req)
		duration := time.Since(t0).Seconds()
		if err == nil {
			servmetrics.AutoMemoryJobTotal.WithLabelValues("done").Inc()
			servmetrics.AutoMemoryExtractionDuration.Observe(duration)
			w.stats.RecordJobDone(int64(duration * 1000))
			return
		}
		lastErr = err
		w.lg.With(loggateway.SessionID(req.SessionID)).Warn("自动记忆提取失败",
			loggateway.Int("attempt", attempt+1), loggateway.Err(err))
	}
	servmetrics.AutoMemoryJobTotal.WithLabelValues("dead").Inc()
	w.stats.RecordJobDead()
	// P2-03: persist the exhausted job to the dead-letter store so the user
	// can see what failed and optionally replay it. Without this, the job is
	// permanently lost with only a log entry.
	if w.deadLetter != nil {
		w.deadLetter.WriteMemoryDeadLetter(biz.MemoryDeadLetterRequest{
			SessionID:         req.SessionID,
			AppName:           req.AppName,
			UserID:            req.UserID,
			FeedbackMessageID: req.FeedbackMessageID,
			FeedbackRating:    req.FeedbackRating,
			FeedbackComment:   req.FeedbackComment,
			Priority:          req.Priority,
			TenantID:          req.TenantID,
		}, biz.MemoryDeadLetterReasonRetryExhausted, errString(lastErr))
	}
	w.lg.With(loggateway.SessionID(req.SessionID)).Warn("自动记忆提取重试耗尽", loggateway.Err(lastErr))
}

func (w *AutoMemoryWorker) extract(ctx context.Context, req memtrpc.AutoMemoryJobRequest) error {
	if strings.TrimSpace(req.FeedbackMessageID) != "" {
		return w.extractFeedback(ctx, req)
	}
	sid := strings.TrimSpace(req.SessionID)
	if sid == "" {
		return nil
	}
	if w.sessions == nil || w.writer == nil || w.consolidator == nil {
		w.lg.Debug("自动记忆跳过：未注入 sessions/writer/consolidator", loggateway.Str("session_id", sid))
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

	msgs, err := w.sessions.ListMessagesRecent(ctx, sid, int(w.memConf.MaxMessages))
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
		w.stats.RecordLLMFallback()
		if primaryErr != nil && !errors.Is(primaryErr, biz.ErrLLMExtractorUnavailable) {
			w.lg.With(loggateway.SessionID(sid)).Warn("LLM 记忆提取失败，已降级启发式", loggateway.Err(primaryErr))
		}
	})
	if err != nil {
		return err
	}

	lastUserMsgID := lastUserMessageID(msgs)
	// Pre-generate episode ID so facts can reference it as source_episode_id.
	episodeID := uuid.NewString()
	var factInputs []biz.MemoryFactWrite
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
			factInputs = append(factInputs, biz.MemoryFactWrite{
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
				SourceEpisodeID: episodeID,
				SourceSessionID: sid,
				SourceMessageID: msgID,
				Status:          "active",
				MetadataJSON:    `{"source":"auto_memory"}`,
			})
		}
	}

	var ep *biz.EpisodeWrite
	if memoryPolicy.WriteL2Episode && len(factInputs) > 0 {
		// Use structured episode extraction (unified pipeline with L1 archive path)
		structured := biz.ExtractStructuredEpisodeFromMessages(in.Messages)
		added := len(factInputs)
		title := structured.Title
		if title == "" {
			title = "Auto-memory consolidation"
			if added == 1 {
				title = previewText(factInputs[0].Statement, 120)
			}
		}
		summary := structured.OutcomeSummary
		if summary == "" {
			summary = previewText(buildEpisodeSummary(proposals, added), 500)
		}
		decisionsJSON, _ := json.Marshal(structured.KeyDecisions)
		artifactsJSON, _ := json.Marshal(structured.KeyArtifacts)
		ep = &biz.EpisodeWrite{
			ID:                  episodeID,
			SessionID:           sid,
			AgentID:             agentID,
			UserID:              userID,
			Title:               title,
			Goal:                structured.Goal,
			Outcome:             structured.Outcome,
			OutcomeSummary:      summary,
			KeyDecisionsJSON:    string(decisionsJSON),
			KeyArtifactsJSON:    string(artifactsJSON),
			EpisodeKind:         structured.EpisodeKind,
			Importance:          structured.Importance,
			Confidence:          structured.Confidence,
			MessageCount:        len(msgs),
			ConsolidatedL3:      added,
			ConsolidationStatus: "consolidated",
			MetadataJSON:        `{"source":"auto_memory"}`,
		}
	}

	writeResult, err := w.writer.UpsertFactsAndEpisodeBatch(ctx, factInputs, ep)
	if err != nil {
		w.lg.With(loggateway.SessionID(sid)).Warn("自动记忆巩固写入失败", loggateway.Err(err))
		return err
	}
	added := writeResult.FactsWritten
	for _, raw := range writeResult.FactRows {
		if w.indexSync != nil {
			if serr := w.indexSync.SyncFactIndexFromRow(ctx, raw); serr != nil {
				w.lg.Warn("auto_memory index sync failed", loggateway.Err(serr))
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
				if serr := w.episodeSync.SyncEpisodeIndex(ctx, agentID, epID, title, summary); serr != nil {
					w.lg.Warn("auto_memory episode index sync failed", loggateway.Err(serr))
				}
			}
		}
	}

	var l4Written int
	if memoryPolicy.WriteL4Graph && w.l4 != nil && agentID != "" {
		for _, msg := range msgs {
			if msg.Role != "user" {
				continue
			}
			text := strings.TrimSpace(msg.ContentMarkdown)
			if text == "" {
				continue
			}
			n, err := w.l4.WriteFromUserText(ctx, agentID, userID, text)
			if err != nil {
				w.lg.With(loggateway.SessionID(sid)).Warn("L4 图谱写入失败", loggateway.Err(err))
			} else {
				l4Written += n
			}
		}
		if l4Written > 0 {
			cfg := biz.MergeDecayOverrides(biz.DefaultL4DecayConfig(), memoryPolicy.L4DecayOverridesJSON)
			w.l4.RunDecayWithConfig(ctx, agentID, cfg)
		}
	}

	w.lg.With(loggateway.SessionID(sid)).Info("自动记忆提取完成",
		loggateway.Int("messages_scanned", len(msgs)),
		loggateway.Int("facts_added", added),
		loggateway.Int("l4_entities", l4Written),
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
	if w.sessions == nil || w.writer == nil || w.feedback == nil {
		w.lg.Debug("反馈记忆跳过：未注入 sessions/writer", loggateway.Str("session_id", sid))
		return nil
	}
	sess, err := w.sessions.Get(ctx, sid)
	if err != nil {
		return err
	}
	// Respect the same write policy as extract(): feedback must not bypass
	// WriteL3Facts / AnyWrite when the agent has L3 writes disabled.
	agentID := strings.TrimSpace(sess.AgentID)
	var memoryPolicy biz.MemoryRuntimePolicy
	if w.agents != nil && agentID != "" {
		if ag, gerr := w.agents.Get(ctx, agentID); gerr == nil && ag.Settings != nil {
			memoryPolicy = biz.ResolveMemoryRuntimePolicy(ag.Settings)
		}
	}
	if !memoryPolicy.AnyWrite() || !memoryPolicy.WriteL3Facts {
		w.lg.Debug("反馈记忆跳过：L3 写入策略关闭", loggateway.Str("session_id", sid), loggateway.Str("agent_id", agentID))
		return nil
	}
	msgs, err := w.sessions.ListMessagesRecent(ctx, sid, int(w.memConf.MaxMessages))
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
	var facts []biz.MemoryFactWrite
	for _, p := range proposals {
		stmt := strings.TrimSpace(p.Statement)
		if stmt == "" {
			continue
		}
		facts = append(facts, biz.MemoryFactWrite{
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
	writeResult, err := w.writer.UpsertFactsAndEpisodeBatch(ctx, facts, nil)
	if err != nil {
		return err
	}
	for _, raw := range writeResult.FactRows {
		if w.indexSync != nil {
			if serr := w.indexSync.SyncFactIndexFromRow(ctx, raw); serr != nil {
				w.lg.Warn("feedback_memory index sync failed", loggateway.Err(serr))
			}
		}
	}
	w.lg.With(loggateway.SessionID(sid)).Info("反馈偏好记忆已写入",
		loggateway.Str("message_id", msgID), loggateway.Str("rating", rating))
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
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	if max <= 0 {
		return s
	}
	return s[:max] + "…"
}

// errString safely converts an error to its string representation.
// Returns empty string for nil errors.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
