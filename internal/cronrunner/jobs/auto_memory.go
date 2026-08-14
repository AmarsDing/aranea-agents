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
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
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
	factPipeline *biz.FactWritePipeline
	memConf      conf.RuntimeAutoMemoryConfig
	stats        *biz.MemoryWorkerStats
	monitorBus   contract.MonitorBus
	// P3 M2: Agent Case 经验提取（用户记忆主流程之后的增强分支）。
	caseExtractor biz.AgentCaseExtractor
	caseReader    biz.AgentCaseReader
	caseWriter    biz.AgentCaseWriter
	writeBack     biz.KnowledgeWriteBack
	reviewQueue   biz.KnowledgeWriteBackReview
	memoryProj    biz.KnowledgeAgentMemoryProjector
	lg            loggateway.Logger
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
	// FactPipeline is the P1-3 unified write pipeline: gates → neighbor
	// recall → LLM adjudication → bi-temporal writes. It replaces the old
	// inline conflict governance. When nil, fact (and dependent episode)
	// writes are skipped.
	FactPipeline *biz.FactWritePipeline
	Stats        *biz.MemoryWorkerStats
	// MonitorBus enables flow-log emission (memory.auto.extract / system.auto_memory.*).
	// When nil, flow-log emission is skipped.
	MonitorBus contract.MonitorBus
	// CaseExtractor/CaseReader/CaseWriter wire the P3 M2 Agent Case pipeline.
	// All three nil → case branch skipped entirely (legacy behavior).
	CaseExtractor biz.AgentCaseExtractor
	CaseReader    biz.AgentCaseReader
	CaseWriter    biz.AgentCaseWriter
	// WriteBack 将过验证门的 L3 事实沉淀到团队知识库（SP7 G2；nil 跳过）。
	WriteBack biz.KnowledgeWriteBack
	// ReviewQueue 低置信白名单事实进入 pending（US-44；nil 跳过）。
	ReviewQueue biz.KnowledgeWriteBackReview
	// MemoryProjector L3 → agents/{id}.md 只读投影（SP7 G1；nil 跳过）。
	MemoryProjector biz.KnowledgeAgentMemoryProjector
	Logger          loggateway.Logger
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
		interval:      interval,
		sessions:      cfg.Sessions,
		agents:        cfg.Agents,
		writer:        cfg.Writer,
		indexSync:     cfg.IndexSync,
		episodeSync:   cfg.EpisodeSync,
		l4:            cfg.L4,
		consolidator:  consolidator,
		feedback:      biz.NewFeedbackConsolidator(),
		queue:         cfg.Queue,
		deadLetter:    cfg.DeadLetterSink,
		factPipeline:  cfg.FactPipeline,
		memConf:       cfg.RuntimeConf.AutoMemoryConfig(),
		stats:         cfg.Stats,
		monitorBus:    cfg.MonitorBus,
		caseExtractor: cfg.CaseExtractor,
		caseReader:    cfg.CaseReader,
		caseWriter:    cfg.CaseWriter,
		writeBack:     cfg.WriteBack,
		reviewQueue:   cfg.ReviewQueue,
		memoryProj:    cfg.MemoryProjector,
		lg:            cfg.Logger,
	}, nil
}

// flowEmitter builds a session-scoped flow-log emitter for auto-memory events.
// Returns nil when the monitor bus is not wired (emission skipped).
func (w *AutoMemoryWorker) flowEmitter(ctx context.Context, sessionID string) *event.TraceEmitter {
	if w == nil || w.monitorBus == nil {
		return nil
	}
	return event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:       ctx,
		SessionID: sessionID,
		Domain:    event.TraceDomainSystem,
		LG:        w.lg,
		Infra:     event.NewInfraFromBus(w.monitorBus),
	})
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

	flow := w.flowEmitter(ctx, req.SessionID)
	if flow != nil {
		flow.LogStart("memory.auto.extract", "自动记忆提取",
			event.P("session_id", req.SessionID))
	}
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
			if flow != nil {
				flow.LogDone("memory.auto.extract", "自动记忆提取完成",
					event.P("session_id", req.SessionID),
					event.P("duration_ms", int64(duration*1000)),
					event.P("attempt", attempt+1))
			}
			return
		}
		lastErr = err
		w.lg.With(loggateway.SessionID(req.SessionID)).Warn("自动记忆提取失败",
			loggateway.Int("attempt", attempt+1), loggateway.Err(err))
		if flow != nil {
			flow.LogWarn("system.auto_memory.extract_fail", "自动记忆提取失败", "",
				event.P("session_id", req.SessionID),
				event.P("attempt", attempt+1),
				event.P("error", err.Error()))
		}
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
	if flow != nil {
		flow.LogError("system.auto_memory.extract_max_retry", "自动记忆提取重试耗尽",
			event.P("session_id", req.SessionID),
			event.P("max_retries", int(w.memConf.MaxRetries)),
			event.P("error", errString(lastErr)))
	}
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
			ToolName:  chatMessageToolName(msg),
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
	var candidates []biz.FactWriteCandidate
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
			scopeType, scopeID := resolveFactScope(p.Scope, userID, appName)
			confidence := p.Confidence
			if confidence <= 0 {
				confidence = 0.85
			}
			candidates = append(candidates, biz.FactWriteCandidate{
				Statement:       stmt,
				FactKind:        biz.FactKindForSubjectType(p.SubjectType),
				Confidence:      confidence,
				Importance:      0.6,
				TagsJSON:        topicsJSON(p.Topics),
				ScopeType:       scopeType,
				ScopeID:         scopeID,
				UserID:          userID,
				AgentID:         appName,
				SourceKind:      "auto_memory",
				SourceEpisodeID: episodeID,
				SourceSessionID: sid,
				SourceMessageID: msgID,
			})
		}
	}

	// Unified write pipeline (P1-3): gates → neighbor recall → adjudication →
	// bi-temporal writes. Replaces the old inline conflict-governance block.
	var writeRes biz.FactWriteBatchResult
	if len(candidates) > 0 {
		if w.factPipeline == nil {
			w.lg.Debug("自动记忆跳过事实写入：未注入 fact pipeline", loggateway.Str("session_id", sid))
		} else {
			writeRes = w.factPipeline.Apply(ctx, candidates)
		}
	}
	factsWritten := writeRes.Added + writeRes.Updated

	var ep *biz.EpisodeWrite
	if memoryPolicy.WriteL2Episode && factsWritten > 0 {
		// Use structured episode extraction (unified pipeline with L1 archive path)
		structured := biz.ExtractStructuredEpisodeFromMessages(in.Messages)
		title := structured.Title
		if title == "" {
			title = "Auto-memory consolidation"
			if factsWritten == 1 {
				title = previewText(candidates[0].Statement, 120)
			}
		}
		summary := structured.OutcomeSummary
		if summary == "" {
			summary = previewText(buildEpisodeSummary(proposals, factsWritten), 500)
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
			ConsolidatedL3:      factsWritten,
			ConsolidationStatus: "consolidated",
			MetadataJSON:        `{"source":"auto_memory"}`,
		}
	}

	// Episode-only write: facts were already persisted by the pipeline.
	var episodeRow []byte
	if ep != nil {
		epRes, err := w.writer.UpsertFactsAndEpisodeBatch(ctx, nil, ep)
		if err != nil {
			w.lg.With(loggateway.SessionID(sid)).Warn("自动记忆 episode 写入失败", loggateway.Err(err))
			return err
		}
		if epRes != nil {
			episodeRow = epRes.EpisodeRow
		}
	}
	added := factsWritten
	for _, raw := range writeRes.FactRows {
		if w.indexSync != nil {
			if serr := w.indexSync.SyncFactIndexFromRow(ctx, raw); serr != nil {
				w.lg.Warn("auto_memory index sync failed", loggateway.Err(serr))
			}
		}
	}
	if w.episodeSync != nil && len(episodeRow) > 0 {
		var epRow map[string]any
		if json.Unmarshal(episodeRow, &epRow) == nil {
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

	w.maybeWriteBack(ctx, sess, candidates, writeRes)
	w.maybeEnqueueReview(ctx, sess, candidates, writeRes)
	w.maybeProjectMemory(ctx, sess)

	var l4Written int
	if memoryPolicy.WriteL4Graph && w.l4 != nil && agentID != "" {
		l4Flow := w.flowEmitter(ctx, sid)
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
				if l4Flow != nil {
					l4Flow.LogWarn("system.auto_memory.l4_fail", "L4 图谱写入失败", "",
						event.P("session_id", sid),
						event.P("agent_id", agentID),
						event.P("error", err.Error()))
				}
			} else {
				l4Written += n
			}
		}
		if l4Written > 0 {
			cfg := biz.MergeDecayOverrides(biz.DefaultL4DecayConfig(), memoryPolicy.L4DecayOverridesJSON)
			w.l4.RunDecayWithConfig(ctx, agentID, cfg)
		}
	}

	// P3 M2: Agent Case 经验提取（增强分支，失败只 Warn 不阻断主流程/不重试）。
	if memoryPolicy.WriteL2Episode {
		w.extractAgentCase(ctx, sid, agentID, userID, in)
	}

	w.lg.With(loggateway.SessionID(sid)).Info("自动记忆提取完成",
		loggateway.Int("messages_scanned", len(msgs)),
		loggateway.Int("facts_added", added),
		loggateway.Int("l4_entities", l4Written),
	)
	return nil
}

// extractAgentCase 是 P3 M2 的 Case 提取分支：LLM 优先、启发式保底、skip
// 信号整条跳过、(agent_id, source_session_id) 幂等。所有失败路径只记日志。
func (w *AutoMemoryWorker) extractAgentCase(ctx context.Context, sid, agentID, userID string, in biz.ConsolidateInput) {
	if w.caseWriter == nil || strings.TrimSpace(agentID) == "" {
		return
	}
	if !biz.ShouldExtractAgentCase(in.Messages) {
		return
	}
	if w.caseReader != nil {
		existing, err := w.caseReader.GetAgentCaseBySession(ctx, agentID, sid)
		if err != nil {
			w.lg.With(loggateway.SessionID(sid)).Warn("Agent Case 幂等检查失败，按未提取继续", loggateway.Err(err))
		} else if existing != nil {
			return
		}
	}
	var c *biz.AgentCase
	if w.caseExtractor != nil {
		cc, err := w.caseExtractor.ExtractCase(ctx, in)
		switch {
		case err == nil:
			c = cc
		case errors.Is(err, biz.ErrAgentCaseSkip):
			return
		default:
			w.lg.With(loggateway.SessionID(sid)).Warn("LLM Agent Case 提取失败，已降级启发式", loggateway.Err(err))
		}
	}
	if c == nil {
		c = biz.HeuristicAgentCase(in)
	}
	if c == nil {
		return
	}
	c.AgentID = agentID
	c.UserID = userID
	c.SourceSessionID = sid
	if err := w.caseWriter.UpsertAgentCase(ctx, *c); err != nil {
		w.lg.With(loggateway.SessionID(sid)).Warn("Agent Case 写入失败", loggateway.Err(err))
		return
	}
	if flow := w.flowEmitter(ctx, sid); flow != nil {
		flow.LogDone("memory.auto.case_extract", "Agent Case 经验提取",
			event.P("session_id", sid),
			event.P("agent_id", agentID),
			event.P("outcome", c.Outcome))
	}
	w.lg.With(loggateway.SessionID(sid)).Info("Agent Case 经验已沉淀",
		loggateway.Str("agent_id", agentID),
		loggateway.Str("outcome", c.Outcome),
		loggateway.Int("tools_used", len(c.ToolsUsed)),
	)
}

// chatMessageToolName 从 ChatMessage.OptionsJSON 解析工具名（role=tool 时由
// activity 适配器写入 {"tool_name":"..."}），供 Case 提取收集 tools_used。
func chatMessageToolName(msg biz.ChatMessage) string {
	if msg.Role != "tool" || strings.TrimSpace(msg.OptionsJSON) == "" {
		return ""
	}
	var opts struct {
		ToolName string `json:"tool_name"`
	}
	if json.Unmarshal([]byte(msg.OptionsJSON), &opts) != nil {
		return ""
	}
	return opts.ToolName
}

func (w *AutoMemoryWorker) extractFeedback(ctx context.Context, req memtrpc.AutoMemoryJobRequest) error {
	sid := strings.TrimSpace(req.SessionID)
	msgID := strings.TrimSpace(req.FeedbackMessageID)
	rating := strings.TrimSpace(req.FeedbackRating)
	comment := strings.TrimSpace(req.FeedbackComment)
	if sid == "" || msgID == "" || rating == "" {
		return nil
	}
	if w.sessions == nil || w.factPipeline == nil || w.feedback == nil {
		w.lg.Debug("反馈记忆跳过：未注入 sessions/factPipeline/feedback", loggateway.Str("session_id", sid))
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
	var candidates []biz.FactWriteCandidate
	for _, p := range proposals {
		stmt := strings.TrimSpace(p.Statement)
		if stmt == "" {
			continue
		}
		scopeType, scopeID := resolveFactScope(p.Scope, userID, appName)
		confidence := p.Confidence
		if confidence <= 0 {
			confidence = 0.85
		}
		candidates = append(candidates, biz.FactWriteCandidate{
			Statement:       stmt,
			FactKind:        biz.FactKindForSubjectType(p.SubjectType),
			Confidence:      confidence,
			Importance:      0.6,
			TagsJSON:        topicsJSON(p.Topics),
			ScopeType:       scopeType,
			ScopeID:         scopeID,
			UserID:          userID,
			AgentID:         appName,
			SourceKind:      "auto_memory",
			SourceSessionID: sid,
			SourceMessageID: msgID,
		})
	}
	if len(candidates) == 0 {
		return nil
	}
	// Feedback facts flow through the same unified write pipeline (P1-3).
	writeRes := w.factPipeline.Apply(ctx, candidates)
	for _, raw := range writeRes.FactRows {
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

// resolveFactScope maps the proposal scope vocabulary to the storage
// scope_type/scope_id pair. "user" scope requires a non-empty userID;
// otherwise it falls back to the agent scope to avoid writing orphan rows.
func resolveFactScope(scope, userID, appName string) (scopeType, scopeID string) {
	if strings.TrimSpace(scope) == "user" && strings.TrimSpace(userID) != "" {
		return "user", strings.TrimSpace(userID)
	}
	return "agent", appName
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

func (w *AutoMemoryWorker) maybeWriteBack(ctx context.Context, sess biz.Session, candidates []biz.FactWriteCandidate, writeRes biz.FactWriteBatchResult) {
	if w == nil || w.writeBack == nil {
		return
	}
	if writeRes.Added+writeRes.Updated == 0 {
		return
	}
	facts := writeBackFactsFromPipeline(candidates, writeRes.FactRows)
	if _, err := w.writeBack.WriteBackSessionFacts(ctx, biz.KnowledgeWriteBackInput{
		Workspace: sess.WorkspaceID,
		SessionID: sess.ID,
		AgentID:   sess.AgentID,
		UserID:    sess.UserID,
		Facts:     facts,
	}); err != nil {
		w.lg.With(loggateway.SessionID(sess.ID)).Warn("知识库写回飞轮失败（记忆主流程不受影响）",
			loggateway.StepID("knowledge.writeback"),
			loggateway.Err(err),
		)
	}
}

func (w *AutoMemoryWorker) maybeEnqueueReview(ctx context.Context, sess biz.Session, candidates []biz.FactWriteCandidate, writeRes biz.FactWriteBatchResult) {
	if w == nil || w.reviewQueue == nil {
		return
	}
	if writeRes.Added+writeRes.Updated == 0 {
		return
	}
	facts := writeBackFactsFromPipeline(candidates, writeRes.FactRows)
	if _, err := w.reviewQueue.EnqueueWriteBackReview(ctx, biz.KnowledgeWriteBackInput{
		Workspace: sess.WorkspaceID,
		SessionID: sess.ID,
		AgentID:   sess.AgentID,
		UserID:    sess.UserID,
		Facts:     facts,
	}); err != nil {
		w.lg.With(loggateway.SessionID(sess.ID)).Warn("知识库待确认写回入队失败（记忆主流程不受影响）",
			loggateway.StepID("knowledge.writeback.pending"),
			loggateway.Err(err),
		)
	}
}

func (w *AutoMemoryWorker) maybeProjectMemory(ctx context.Context, sess biz.Session) {
	if w == nil || w.memoryProj == nil {
		return
	}
	if strings.TrimSpace(sess.AgentID) == "" {
		return
	}
	if err := w.memoryProj.ProjectAgentMemory(ctx, sess.WorkspaceID, sess.AgentID); err != nil {
		w.lg.With(loggateway.SessionID(sess.ID)).Warn("agent 记忆投影失败（记忆主流程不受影响）",
			loggateway.StepID("knowledge.memory.project"),
			loggateway.Err(err),
		)
	}
}

func writeBackFactsFromPipeline(candidates []biz.FactWriteCandidate, rows [][]byte) []biz.KnowledgeWriteBackFact {
	fromRows := writeBackFactsFromRows(rows)
	if len(fromRows) == 0 {
		out := make([]biz.KnowledgeWriteBackFact, 0, len(candidates))
		for _, c := range candidates {
			out = append(out, biz.KnowledgeWriteBackFact{
				Statement:  c.Statement,
				FactKind:   c.FactKind,
				Confidence: c.Confidence,
				SourceKind: c.SourceKind,
			})
		}
		return out
	}
	byStmt := make(map[string]biz.FactWriteCandidate, len(candidates))
	for _, c := range candidates {
		byStmt[strings.ToLower(strings.TrimSpace(c.Statement))] = c
	}
	for i := range fromRows {
		c, ok := byStmt[strings.ToLower(strings.TrimSpace(fromRows[i].Statement))]
		if !ok {
			continue
		}
		if fromRows[i].FactKind == "" {
			fromRows[i].FactKind = c.FactKind
		}
		if fromRows[i].Confidence <= 0 {
			fromRows[i].Confidence = c.Confidence
		}
		if fromRows[i].SourceKind == "" {
			fromRows[i].SourceKind = c.SourceKind
		}
	}
	return fromRows
}

func writeBackFactsFromRows(rows [][]byte) []biz.KnowledgeWriteBackFact {
	out := make([]biz.KnowledgeWriteBackFact, 0, len(rows))
	for _, raw := range rows {
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		stmt, _ := m["statement"].(string)
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		kind, _ := m["fact_kind"].(string)
		id, _ := m["id"].(string)
		src, _ := m["source_kind"].(string)
		conf := jsonNumber(m["confidence"])
		out = append(out, biz.KnowledgeWriteBackFact{
			FactID:     id,
			Statement:  stmt,
			FactKind:   kind,
			Confidence: conf,
			SourceKind: src,
		})
	}
	return out
}

func jsonNumber(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

// errString safely converts an error to its string representation.
// Returns empty string for nil errors.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
