package biz

import (
	"context"
	"encoding/json"
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
	// lister + conflict are optional same-slot governors. Chat updates such as
	// "favorite color is red now" must invalidate the previous slot fact in
	// this turn; Sleep-time FactWritePipeline is too late. Do not route the
	// writer through FactWritePipeline: gate ① would drop user_identity /
	// agent_instruction / domain_knowledge.
	lister   MemoryPreferenceLister
	conflict L3ConflictStore
}

var immediateSlotKinds = []string{"preference", "constraint", "user_identity", "user_preference", "profile"}

const immediateSlotLook int32 = 40

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

// SetSlotGovernor wires same-slot supersede after a successful write.
// Either argument nil disables governance (safe degradation).
func (w *ImmediateFactWriter) SetSlotGovernor(lister MemoryPreferenceLister, conflict L3ConflictStore) {
	if w == nil {
		return
	}
	w.lister = lister
	w.conflict = conflict
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
		// Absence meta-statements ("用户询问 X，但暂无此信息") are conversation
		// meta-observations, not durable facts: persisted with importance 0.8 they
		// outrank the true fact on recency, the model parrots "not found", and the
		// reply is saved as yet another absence statement (2026-08-26 domain-B
		// regression pollution loop). Drop them at the write gate.
		if LooksLikeAbsenceMetaStatement(f.Content) {
			w.lg.Info("即时事实跳过：缺失元陈述",
				loggateway.StepID("memory.immediate_fact"),
				loggateway.SessionID(sessionID),
				loggateway.Str("statement_preview", truncateRunes(strings.TrimSpace(f.Content), 60)))
			continue
		}
		// Map fact type to fact_kind, then canonicalize so 工号/我叫/负责
		// cannot land as preference/profile/domain_knowledge duplicates.
		factKind := CanonicalizeFactKind(mapFactTypeToKind(f.Type), f.Content)

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
			Statement:       NormalizeStatementPunctuation(f.Content),
			FactKind:        factKind,
			Confidence:      confidence,
			Importance:      0.8, // High importance for user-explicit facts
			SourceKind:      "immediate_extraction",
			SourceSessionID: sessionID,
			SourceMessageID: sourceMessageID,
			Status:          "active",
		})
	}

	if len(factWrites) == 0 {
		return nil
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
	if res != nil {
		w.applySlotSupersede(ctx, agentID, userID, res.FactRows)
	}
	return nil
}

type factSlotRow struct {
	id, kind, stmt string
}

func parseFactSlotRow(raw []byte) (factSlotRow, bool) {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return factSlotRow{}, false
	}
	id, _ := m["id"].(string)
	kind, _ := m["fact_kind"].(string)
	stmt, _ := m["statement"].(string)
	id = strings.TrimSpace(id)
	if id == "" || strings.TrimSpace(stmt) == "" {
		return factSlotRow{}, false
	}
	return factSlotRow{id: id, kind: kind, stmt: stmt}, true
}

// applySlotSupersede invalidates older active facts that occupy the same
// preference / identity / residence slot as a just-written fact. One list
// call covers the batch (CS-B10); failures are best-effort and never fail
// the write.
func (w *ImmediateFactWriter) applySlotSupersede(ctx context.Context, agentID, userID string, written [][]byte) {
	if w == nil || w.lister == nil || w.conflict == nil || len(written) == 0 {
		return
	}
	keepers := map[string]factSlotRow{}
	var slots []string
	for _, raw := range written {
		row, ok := parseFactSlotRow(raw)
		if !ok {
			continue
		}
		slot := PreferenceSlotKey(row.stmt)
		if slot == "" {
			continue
		}
		if _, seen := keepers[slot]; !seen {
			slots = append(slots, slot)
		}
		keepers[slot] = row
	}
	if len(keepers) == 0 {
		return
	}
	existing, err := w.lister.ListActivePreferenceFacts(ctx, agentID, userID, immediateSlotKinds, immediateSlotLook)
	if err != nil {
		w.lg.Warn("即时事实同槽列举失败",
			loggateway.StepID("memory.immediate_fact_slot"),
			loggateway.Err(err))
		return
	}
	for _, slot := range slots {
		nf := keepers[slot]
		for _, raw := range existing {
			old, ok := parseFactSlotRow(raw)
			if !ok || old.id == nf.id {
				continue
			}
			if !ShouldSupersedeSameSlotFact(old.kind, old.stmt, nf.kind, nf.stmt) {
				continue
			}
			if serr := w.conflict.SupersedeFact(ctx, old.id, nf.id); serr != nil {
				w.lg.Warn("即时事实同槽覆盖失败",
					loggateway.StepID("memory.immediate_fact_slot"),
					loggateway.Err(serr),
					loggateway.Str("old_fact_id", old.id),
					loggateway.Str("new_fact_id", nf.id))
			}
		}
	}
}

// mapFactTypeToKind maps XML fact type to memory_fact fact_kind.
func mapFactTypeToKind(factType string) string {
	switch strings.ToLower(strings.TrimSpace(factType)) {
	case "identity":
		return "user_identity"
	case "preference":
		return "preference"
	case "instruction":
		return "agent_instruction"
	case "domain_knowledge":
		return "domain_knowledge"
	default:
		return "general"
	}
}

// mapFactKindToScope maps fact_kind to the owning recall scope.
// user_identity/preference belong to the user (aligned with the remember
// tool, cross-session visible); everything else is an agent asset. Falls back
// to agent scope when userID is empty so facts stay recallable.
func mapFactKindToScope(factKind, agentID, userID string) (scopeType, scopeID string) {
	if UserScopedFactKind(factKind) {
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
