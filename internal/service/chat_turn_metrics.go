package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	bizsession "aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"
	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"
)

// turnRecorder is the interface for recording turn usage and session turn data.
type turnRecorder interface {
	RecordTurnUsage(ctx context.Context, p TurnUsageParams)
	RecordSessionTurn(ctx context.Context, p SessionTurnRecordParams)
	// RecordAuxUsage records auxiliary (non-turn) LLM usage such as the intent
	// pass (P1-2, 2026-08-19).
	RecordAuxUsage(ctx context.Context, in biz.AuxLLMUsageInput)
}

// chatTurnMetrics implements turnRecorder.
//
// Part of the TECH-DEBT(BL8) resolution: separating metrics recording from
// the orchestrator's core turn logic.
type chatTurnMetrics struct {
	sessions biz.SessionTurnManager
	usage    *biz.UsageUsecase
	monitor  *biz.MonitorUsecase
	lg       loggateway.Logger
}

func newChatTurnMetrics(sessions biz.SessionTurnManager, usage *biz.UsageUsecase, monitor *biz.MonitorUsecase, lg loggateway.Logger) *chatTurnMetrics {
	return &chatTurnMetrics{sessions: sessions, usage: usage, monitor: monitor, lg: lg}
}

// Compile-time interface check.
var _ turnRecorder = (*chatTurnMetrics)(nil)

// TurnUsageParams groups parameters for recording turn token usage.
type TurnUsageParams struct {
	Emitter       *event.TraceEmitter
	SessionID     string
	RunID         string
	AgentKey      string
	AgentID       string
	Provider      string
	Model         string
	Status        string
	PromptTok     int
	CompletionTok int
	// CachedTok is the cache-hit portion of PromptTok (billed at cache-read price).
	CachedTok int
	// UsageSource records how PromptTok/CompletionTok were obtained
	// ("streaming"/"runner_completion"/"estimated"); persisted into
	// metadata_json["usage_source"] so estimated rows stay identifiable.
	UsageSource string
	// Rounds 是本 turn 的 LLM 调用轮数（流内按 response ID 去重实测），
	// 落入 usage.metadata_json["llm_rounds"] 使跨轮求和的 token 行可解释。
	// <=0 表示未观测，不落 metadata。
	Rounds  int
	Latency time.Duration
	ErrMsg  string
	// FirstTokenMs is stream-observed TTFT; 0 means unobserved.
	FirstTokenMs int
	// WaitMS is HITL confirmation wait accumulated on the turn ctx.
	WaitMS int
}

// RecordTurnUsage records token usage for a turn.
func (m *chatTurnMetrics) RecordTurnUsage(ctx context.Context, p TurnUsageParams) {
	if m == nil {
		return
	}
	// R4-Q10：失败 run 计入 session_metrics.error_count（此前该列无写入方恒 0，
	// S09 first_byte_timeout critical 失败 error_count 仍为 0）。cancelled 是
	// 用户主动行为、timeout_degraded 有产出，均不计入错误。该计数只依赖
	// m.sessions，放在 usage nil 早退之前，保证失败一定被计数。
	if (p.Status == "error" || p.Status == "failed" || p.Status == "orphaned") && m.sessions != nil && strings.TrimSpace(p.SessionID) != "" {
		m.sessions.AccumulateMetricsDelta(bizsession.SessionMetricsDelta{SessionID: p.SessionID, ErrorCount: 1})
	}
	if m.usage == nil {
		return
	}
	// 落库与执行 ctx 解耦：客户端断连/用户取消时 usage 行仍须写入，
	// 否则计费与慢查询归因丢数据（P1，2026-08-20）。
	ctx, cancel := appctx.Detach(ctx)
	defer cancel()
	// E 预算表分解（P0-A，2026-08-11）：per-turn token/缓存命中入进程日志，
	// 前缀稳定化效果直接由 cache_hit_ratio 验证（此前只落库，排查需查表）。
	if p.PromptTok > 0 {
		m.lg.Info("turn token usage",
			loggateway.StepID("chat.turn_usage"),
			loggateway.SessionID(p.SessionID),
			loggateway.RunID(p.RunID),
			loggateway.AgentKey(p.AgentKey),
			loggateway.Str("model", p.Model),
			loggateway.Int("prompt_tokens", p.PromptTok),
			loggateway.Int("completion_tokens", p.CompletionTok),
			loggateway.Int("cached_tokens", p.CachedTok),
			loggateway.Float64("cache_hit_ratio", float64(p.CachedTok)/float64(p.PromptTok)),
			loggateway.Duration(p.Latency.Milliseconds()))
	}
	m.recordContextBudgetLog(ctx, p)
	meta := "{}"
	if p.Emitter != nil {
		meta = p.Emitter.MetadataJSON()
	}
	meta = mergeContextBudgetMetadata(ctx, meta)
	meta = biz.MergeUsageSourceMetadata(meta, p.UsageSource)
	meta = biz.MergeLLMRoundsMetadata(meta, p.Rounds)
	meta = biz.MergeWaitMSMetadata(meta, p.WaitMS, int(p.Latency.Milliseconds()))
	traceID := ""
	if p.Emitter != nil {
		traceID = p.Emitter.TraceID()
	}
	if err := m.usage.RecordTurnUsage(ctx, biz.TurnUsageInput{
		SessionID:          p.SessionID,
		RunID:              p.RunID,
		AgentKey:           p.AgentKey,
		AgentID:            p.AgentID,
		Provider:           p.Provider,
		Model:              p.Model,
		Status:             p.Status,
		PromptTok:          p.PromptTok,
		CompletionTok:      p.CompletionTok,
		CachedTok:          p.CachedTok,
		Latency:            p.Latency,
		ErrMsg:             p.ErrMsg,
		MetadataJSON:       meta,
		TraceID:            traceID,
		TimeToFirstTokenMS: p.FirstTokenMs,
		WaitMS:             p.WaitMS,
	}); err != nil && p.Emitter != nil {
		p.Emitter.LogError("chat.usage_record", "turn usage record failed",
			event.P("error", err.Error()),
			event.P("run_id", p.RunID),
			event.P("usage_kind", biz.UsageKindChatTurn),
			event.P("status", p.Status),
		)
	}
	m.recordRunnerCompletion(ctx, p, traceID)
}

// RecordAuxUsage records auxiliary LLM call usage (intent pass etc.).
// Zero-token rows are skipped: a skipped/failed aux call consumed nothing
// observable, and recording it would only add noise to aggregates.
func (m *chatTurnMetrics) RecordAuxUsage(ctx context.Context, in biz.AuxLLMUsageInput) {
	if m == nil || m.usage == nil {
		return
	}
	if in.PromptTok <= 0 && in.CompletionTok <= 0 {
		return
	}
	// aux usage（intent/memory extract）同样是计费数据，断连时仍须落库（P1）。
	ctx, cancel := appctx.Detach(ctx)
	defer cancel()
	if err := m.usage.RecordAuxLLMUsage(ctx, in); err != nil {
		m.lg.Warn("aux usage record failed",
			loggateway.StepID("chat.usage_record_fail"),
			loggateway.Str("usage_kind", in.Kind),
			loggateway.SessionID(in.SessionID),
			loggateway.Err(err))
	}
}

// recordContextBudgetLog emits the per-turn context budget ledger
// (29-token.design.md §9.6, 任务 0.1). Process log only — no flow log, no
// stepTitleRegistry entry. No-op when the turn ctx carries no ContextBudget
// (non-chat paths) or nothing was recorded (LLM never reached, e.g. early
// admission failure).
func (m *chatTurnMetrics) recordContextBudgetLog(ctx context.Context, p TurnUsageParams) {
	if m == nil {
		return
	}
	budget := chatagent.ContextBudgetFromContext(ctx)
	if budget == nil {
		return
	}
	snap := budget.Snapshot()
	if snap.EstTotalInput == 0 && snap.ToolsCount == 0 {
		return
	}
	// 台账数据同步观测进 Prometheus 直方图（按 category），否则纯日志无法
	// 聚合分析各分桶的 token 占比趋势。
	for cat, tokens := range snap.EstTokens {
		if tokens > 0 {
			metrics.ContextBudgetTokens.WithLabelValues(cat).Observe(float64(tokens))
		}
	}
	staticRatio := 0.0
	if snap.EstTotalInput > 0 {
		staticRatio = float64(snap.EstTokens[chatagent.ContextBudgetCategoryStaticPrefix]) / float64(snap.EstTotalInput)
	}
	fields := []loggateway.Field{
		loggateway.StepID("chat.context_budget"),
		loggateway.SessionID(p.SessionID),
		loggateway.RunID(p.RunID),
		loggateway.AgentKey(p.AgentKey),
		loggateway.Int("static_prefix_tokens", snap.EstTokens[chatagent.ContextBudgetCategoryStaticPrefix]),
		loggateway.Int("tools_schema_tokens", snap.EstTokens[chatagent.ContextBudgetCategoryToolsSchema]),
		loggateway.Int("tools_count", snap.ToolsCount),
		loggateway.Int("history_tokens", snap.EstTokens[chatagent.ContextBudgetCategoryHistory]),
		loggateway.Int("memory_l1_tokens", snap.EstTokens[chatagent.ContextBudgetCategoryMemoryL1]),
		loggateway.Int("memory_l4_tokens", snap.EstTokens[chatagent.ContextBudgetCategoryMemoryL4]),
		loggateway.Int("memory_composite_tokens", snap.EstTokens[chatagent.ContextBudgetCategoryMemoryComposite]),
		loggateway.Int("knowledge_cue_tokens", snap.EstTokens[chatagent.ContextBudgetCategoryKnowledgeCue]),
		loggateway.Int("skill_guidance_tokens", snap.EstTokens[chatagent.ContextBudgetCategorySkillGuidance]),
		loggateway.Int("skill_overview_tokens", snap.EstTokens[chatagent.ContextBudgetCategorySkillOverview]),
		loggateway.Int("other_dynamic_tokens", snap.EstTokens[chatagent.ContextBudgetCategoryOtherDynamic]),
		loggateway.Int("tool_catalog_cue_tokens", snap.EstTokens[chatagent.ContextBudgetCategoryToolCatalogCue]),
		loggateway.Int("est_total_input", snap.EstTotalInput),
		loggateway.Float64("static_ratio", staticRatio),
	}
	if p.PromptTok > 0 {
		fields = append(fields, loggateway.Float64("cache_hit_ratio", float64(p.CachedTok)/float64(p.PromptTok)))
	}
	// N6: name the top-5 largest tool schemas so operators can see WHICH
	// tools dominate tools_schema_tokens (the aggregate alone cannot).
	if len(snap.TopTools) > 0 {
		tops := make([]map[string]any, 0, len(snap.TopTools))
		for _, tt := range snap.TopTools {
			tops = append(tops, map[string]any{"name": tt.Name, "est_tokens": tt.EstTokens})
		}
		fields = append(fields, loggateway.Any("top_tool_schemas", tops))
	}
	m.lg.Info("context budget ledger", fields...)
}

// mergeContextBudgetMetadata merges the per-turn context budget snapshot into
// usage.metadata_json under the "context_budget" key (S2, 29-token.design.md
// §9.6). The process log / Prometheus histogram are point-in-time signals;
// persisting the ledger enables DB-side aggregation of token composition
// across turns (e.g. avg tools_schema_tokens per agent over time).
// Passthrough when no budget is mounted (non-chat paths) or nothing was
// recorded (LLM never reached); existing metadata keys are preserved.
func mergeContextBudgetMetadata(ctx context.Context, meta string) string {
	// Shared payload shape with the context_usage WS event meta
	// (chatagent.ContextBudgetPayload) — nil means "no ledger, passthrough".
	cb := chatagent.ContextBudgetPayload(ctx)
	if cb == nil {
		return meta
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(meta), &payload); err != nil || payload == nil {
		payload = map[string]any{}
	}
	payload["context_budget"] = cb
	raw, err := json.Marshal(payload)
	if err != nil {
		return meta
	}
	return string(raw)
}

// recordRunnerCompletion writes the runner.completion monitor event for a
// terminal chat turn. This restores the data stream behind the Runner
// metrics panel and the runner.error_rate alert rule — the legacy writer
// (EventBus runner-completion handler) was removed in the Activity-First
// migration (ab9ee9e07) without a replacement, leaving the stream dark
// since 2026-06-13.
func (m *chatTurnMetrics) recordRunnerCompletion(ctx context.Context, p TurnUsageParams, traceID string) {
	if m == nil || m.monitor == nil {
		return
	}
	de := biz.DomainEvent{
		Type:       biz.DomainEventRunnerCompletion,
		SessionID:  p.SessionID,
		RunID:      p.RunID,
		Author:     p.AgentKey,
		AgentID:    p.AgentID,
		TraceID:    traceID,
		DurationMS: p.Latency.Milliseconds(),
		Timestamp:  time.Now().UTC(),
		RunKind:    "chat",
	}
	if p.Status == "error" {
		de.Error = &biz.DomainError{Message: p.ErrMsg}
	}
	recCtx, cancel := appctx.Detach(ctx)
	defer cancel()
	if err := biz.RecordRunnerCompletion(recCtx, m.monitor, de); err != nil {
		m.lg.Warn("runner.completion 监控事件落库失败",
			loggateway.StepID("chat.runner_completion_fail"),
			loggateway.Str("session_id", p.SessionID),
			loggateway.Str("run_id", p.RunID),
			loggateway.Err(err))
	}
}

// SessionTurnRecordParams groups parameters for recording a completed session turn.
type SessionTurnRecordParams struct {
	SessionID      string
	OwnerType      string // "agent" or "team"
	OwnerID        string // AgentID when OwnerType="agent", TeamID when OwnerType="team"
	UserMsgID      string
	AssistantMsgID string
	Provider       string
	Model          string
	PromptTok      int
	CompletionTok  int
	// CachedTok is the cache-hit portion of PromptTok (provider-reported).
	// Persisted to session_turns.cached_input_tokens alongside the other token
	// counters — zero is a meaningful "no cache hit" observation, so it is
	// written unconditionally like InputTokens/OutputTokens.
	CachedTok      int
	ContentPreview string
	// DurationMs 是整轮耗时（admission → postProcess）。<=0 表示未测量，不落库。
	DurationMs int
	// FirstTokenMs 是 TTFT（首个模型字节）。<=0 表示流内未观测到首字节，不落库。
	FirstTokenMs int
	// ModelCallCount / ToolCallCount 是流内实测的 LLM 轮次 / 工具调用数。
	// <=0 表示未观测（如早退、无流事件），不落库，保留零值语义。
	ModelCallCount int
	ToolCallCount  int
}

// RecordSessionTurn records a completed agent or team turn.
func (m *chatTurnMetrics) RecordSessionTurn(ctx context.Context, p SessionTurnRecordParams) {
	if m == nil || m.sessions == nil {
		return
	}
	// 与 RecordTurnUsage 同理：turn 收尾落库须独立于客户端连接存活（P1）。
	// admittedTurnID 等 ctx values 由 Detach 保留。
	ctx, cancel := appctx.Detach(ctx)
	defer cancel()
	now := time.Now().UTC().Format(time.RFC3339)
	preview := strutil.ProtoPreview(p.ContentPreview, 200)
	// 观测事务统一（SP-1，2026-08-29）：message_count/run_count 在 turn 完成点
	// 统一累加。agent 路径消息改走 steps_v2 投影（tasks_v2+steps_v2），不再经过
	// message_usecase.AppendChatTurn，message_count 在此补 +2（user+assistant）；
	// team 路径消息仍由 runner AppendChatMessage 逐条累加（runner_team_trpc.go），
	// 此处仅补 run_count，避免双计。
	if sid := strings.TrimSpace(p.SessionID); sid != "" {
		msgCount := 0
		if p.OwnerType == "agent" {
			msgCount = 2
		}
		m.sessions.AccumulateMetricsDelta(bizsession.SessionMetricsDelta{
			SessionID:     sid,
			MessageCount:  msgCount,
			RunCount:      1,
			LatencySumMs:  int64(max(p.DurationMs, 0)),
			LastMessageAt: now,
		})
		// SP-1d：turn（run）结束点强刷该 session 的累积 delta，不等 200ms
		// ticker——run 结束后 session_metrics 立即可查（此前观测查询在
		// ticker 落库前读到 message_count=0/run_count=0 的恒空假影）。
		m.sessions.FlushSessionMetrics(sid)
	}
	if turnID := admittedTurnIDFromContext(ctx); turnID != "" {
		updates := biz.SessionTurnUpdateFields{
			Status:              ptrString("completed"),
			EndedAt:             ptrString(now),
			UserMessageID:       ptrString(p.UserMsgID),
			AssistantMessageID:  ptrString(p.AssistantMsgID),
			OwnerType:           ptrString(p.OwnerType),
			InputTokens:         ptrInt(p.PromptTok),
			OutputTokens:        ptrInt(p.CompletionTok),
			TotalTokens:         ptrInt(p.PromptTok + p.CompletionTok),
			CachedInputTokens:   ptrInt(p.CachedTok),
			FinalProvider:       ptrString(p.Provider),
			FinalModel:          ptrString(p.Model),
			FinalContentPreview: ptrString(preview),
		}
		// context_budget 台账随 turn 元数据落库：前端会话曲线悬停面板按
		// session_turns.metadata_json.context_budget 渲染该轮 prompt 构成
		// （此前只写 model_token_usage_events，悬停恒为回退视图）。无台账时透传
		// "{}"，保持 nil 不更新，避免覆盖并发写入的既有元数据。
		if meta := mergeContextBudgetMetadata(ctx, "{}"); meta != "{}" {
			updates.MetadataJSON = ptrString(meta)
		}
		// 实测指标仅在观测到时落库（>0），避免用零值覆盖并发/前序写入。
		if p.DurationMs > 0 {
			updates.DurationMs = ptrInt(p.DurationMs)
		}
		if p.FirstTokenMs > 0 {
			updates.FirstTokenMs = ptrInt(p.FirstTokenMs)
		}
		if p.ModelCallCount > 0 {
			updates.ModelCallCount = ptrInt(p.ModelCallCount)
		}
		if p.ToolCallCount > 0 {
			updates.ToolCallCount = ptrInt(p.ToolCallCount)
		}
		switch p.OwnerType {
		case "agent":
			updates.AgentID = ptrString(p.OwnerID)
		case "team":
			updates.TeamID = ptrString(p.OwnerID)
		}
		if _, err := m.sessions.UpdateTurn(ctx, turnID, updates); err != nil {
			m.lg.Warn("session turn update failed",
				loggateway.StepID("chat.usage_record_fail"),
				loggateway.Str("session_id", p.SessionID),
				loggateway.Str("turn_id", turnID),
				loggateway.Err(err))
		}
		return
	}
	turn := biz.SessionTurn{
		SessionID:           p.SessionID,
		UserMessageID:       p.UserMsgID,
		AssistantMessageID:  p.AssistantMsgID,
		OwnerType:           p.OwnerType,
		Status:              "completed",
		StartedAt:           now,
		EndedAt:             now,
		DurationMs:          p.DurationMs,
		FirstTokenMs:        p.FirstTokenMs,
		ModelCallCount:      p.ModelCallCount,
		ToolCallCount:       p.ToolCallCount,
		InputTokens:         p.PromptTok,
		OutputTokens:        p.CompletionTok,
		TotalTokens:         p.PromptTok + p.CompletionTok,
		CachedInputTokens:   p.CachedTok,
		FinalProvider:       p.Provider,
		FinalModel:          p.Model,
		FinalContentPreview: preview,
	}
	// 同 Update 路径：context_budget 台账落 session_turns.metadata_json，
	// 供前端悬停面板渲染该轮 prompt 构成；无台账则留空。
	if meta := mergeContextBudgetMetadata(ctx, ""); meta != "" {
		turn.MetadataJSON = meta
	}
	switch p.OwnerType {
	case "agent":
		turn.AgentID = p.OwnerID
	case "team":
		turn.TeamID = p.OwnerID
	}
	if _, err := m.sessions.CreateTurn(ctx, turn); err != nil {
		m.lg.Warn("session turn record failed",
			loggateway.StepID("chat.usage_record_fail"),
			loggateway.Str("session_id", p.SessionID),
			loggateway.Err(err))
	}
}
