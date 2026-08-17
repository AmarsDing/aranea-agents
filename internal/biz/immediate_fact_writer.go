package biz

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// ImmediateFactWriter persists parsed facts to memory_fact immediately.
// This bridges the async gap between conversation and Sleep-time consolidation.
type ImmediateFactWriter struct {
	lg         loggateway.Logger
	factWriter MemoryConsolidationWriter
	// indexSync 回采写入行的 embedding 索引（P2-2）：auto_memory 正常路径在
	// UpsertFactsAndEpisodeBatch 后逐行 SyncFactIndexFromRow，即时事实此前丢弃
	// 返回值 → embedding 恒 pending，向量召回要等 reconciler cron 兜底才可见。
	// nil 时降级跳过（reconciler 仍是最终一致兜底）。
	indexSync MemoryFactIndexSyncer
}

// NewImmediateFactWriter creates a new ImmediateFactWriter.
func NewImmediateFactWriter(factWriter MemoryConsolidationWriter, indexSync MemoryFactIndexSyncer, lg loggateway.Logger) *ImmediateFactWriter {
	if factWriter == nil {
		return nil
	}
	return &ImmediateFactWriter{
		lg:         lg,
		factWriter: factWriter,
		indexSync:  indexSync,
	}
}

// WriteFacts persists facts asynchronously (fire-and-forget).
// The goroutine uses context.WithoutCancel to avoid being cancelled by request context.
func (w *ImmediateFactWriter) WriteFacts(ctx context.Context, sessionID, agentID, userID, sourceMessageID string, facts []FactMark) {
	if w == nil || len(facts) == 0 {
		return
	}

	// Fire-and-forget write so the runner turn is not blocked.
	// 红线 #13：必须走 safego，writeFactsSync 内 panic 不得导致进程崩溃。
	bgCtx := context.WithoutCancel(ctx)
	bgCtx, cancel := context.WithTimeout(bgCtx, 10*time.Second)
	safego.Go(bgCtx, "memory.immediate_fact", func() {
		defer cancel()

		if err := w.writeFactsSync(bgCtx, sessionID, agentID, userID, sourceMessageID, facts); err != nil {
			w.lg.Warn("即时事实写入失败",
				loggateway.StepID("memory.immediate_fact"),
				loggateway.Err(err),
				loggateway.SessionID(sessionID),
				loggateway.Int("fact_count", len(facts)))
			return
		}

		w.lg.Info("即时事实写入成功",
			loggateway.StepID("memory.immediate_fact"),
			loggateway.SessionID(sessionID),
			loggateway.Int("fact_count", len(facts)))
	})
}

// writeFactsSync performs the actual fact persistence.
func (w *ImmediateFactWriter) writeFactsSync(ctx context.Context, sessionID, agentID, userID, sourceMessageID string, facts []FactMark) error {
	if w.factWriter == nil {
		return nil
	}

	// Convert FactMark to MemoryFactWrite
	factWrites := make([]MemoryFactWrite, 0, len(facts))
	for _, f := range facts {
		// Map fact type to fact_kind
		factKind := mapFactTypeToKind(f.Type)

		// Map confidence string to float
		confidence := mapConfidenceToFloat(f.Confidence)

		// 按 fact_kind 归属召回作用域：identity/preference 属用户（跨会话），
		// 其余属 agent。严禁写 session scope——L3ScopeTargets 无 session case，
		// session 事实是任何会话都召不回的死数据（R1 根因）。
		scopeType, scopeID := mapFactKindToScope(factKind, agentID, userID)

		factWrites = append(factWrites, MemoryFactWrite{
			ScopeType:       scopeType,
			ScopeID:         scopeID,
			UserID:          userID,
			AgentID:         agentID,
			Statement:       f.Content,
			FactKind:        factKind,
			Confidence:      confidence,
			Importance:      0.8, // High importance for user-explicit facts
			SourceKind:      "immediate_extraction",
			SourceSessionID: sessionID,
			SourceMessageID: sourceMessageID,
			Status:          "active",
		})
	}

	// Use existing consolidation writer with nil episode (facts only, no episode)
	res, err := w.factWriter.UpsertFactsAndEpisodeBatch(ctx, factWrites, nil)
	if err != nil {
		return err
	}
	// P2-2：写入成功后即同步 embedding 索引（best-effort，对齐 auto_memory
	// 回采范式）；失败不阻断——reconciler cron 扫描 index_status=pending 兜底。
	if w.indexSync != nil && res != nil {
		for _, raw := range res.FactRows {
			if serr := w.indexSync.SyncFactIndexFromRow(ctx, raw); serr != nil {
				w.lg.Warn("即时事实索引同步失败",
					loggateway.StepID("memory.immediate_fact_index"),
					loggateway.Err(serr))
			}
		}
	}
	return nil
}

// mapFactTypeToKind maps XML fact type to memory_fact fact_kind.
func mapFactTypeToKind(factType string) string {
	switch strings.ToLower(strings.TrimSpace(factType)) {
	case "identity":
		return "user_identity"
	case "preference":
		return "user_preference"
	case "instruction":
		return "agent_instruction"
	case "domain_knowledge":
		return "domain_knowledge"
	default:
		return "general"
	}
}

// mapFactKindToScope maps fact_kind to the owning recall scope.
// user_identity/user_preference belong to the user (aligned with the remember
// tool, cross-session visible); everything else is an agent asset. Falls back
// to agent scope when userID is empty so facts stay recallable.
func mapFactKindToScope(factKind, agentID, userID string) (scopeType, scopeID string) {
	switch factKind {
	case "user_identity", "user_preference":
		if id := strings.TrimSpace(userID); id != "" {
			return "user", id
		}
	}
	return "agent", strings.TrimSpace(agentID)
}

// mapConfidenceToFloat maps confidence string to float value.
func mapConfidenceToFloat(confidence string) float64 {
	switch strings.ToLower(strings.TrimSpace(confidence)) {
	case "high":
		return 0.95
	case "medium":
		return 0.7
	case "low":
		return 0.4
	default:
		return 0.6
	}
}
