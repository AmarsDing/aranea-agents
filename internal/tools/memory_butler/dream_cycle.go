package memory_butler

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/jsonutil"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// Memory butler business constants — single source of truth for thresholds.
const (
	defaultFactListLimit          = 500
	defaultInactiveThresholdDays  = 30
	defaultSimilarityThreshold    = 0.8
	minHitCountForLowQuality      = 3
	negativeFeedbackRateThreshold = 0.5
	minLengthForSubstringCheck    = 20
)

type dreamCycleInput struct {
	AgentID string `json:"agent_id" jsonschema:"description=Agent ID,required"`
	// P2-b：*bool 三态落地「默认 dry_run」。框架 tag 解析器忽略 default= 键、
	// 值类型 bool 省略参数时零值 false → 模型不传 dry_run 会真实删除记忆。
	// 指针 nil（省略）即 true；仅显式传 false 才进入执行路径。
	DryRun *bool `json:"dry_run" jsonschema:"description=仅预览不实际执行；缺省=true（安全预览），仅显式传 false 才真实删除记忆"`
}

// effectiveDryRun 解析三态 dry_run：nil（省略）→ true 安全预览。
func (in dreamCycleInput) effectiveDryRun() bool {
	return in.DryRun == nil || *in.DryRun
}

type dreamCycleOutput struct {
	QualityBefore  float64  `json:"quality_before"`
	QualityAfter   float64  `json:"quality_after"`
	ActionsTaken   []string `json:"actions_taken"`
	DeletedCount   int      `json:"deleted_count"`
	MergedCount    int      `json:"merged_count"`
	DistilledCount int      `json:"distilled_count"`
	// M4 知识库词条治理：本轮新产高风险 pending 提案数与执行的治理任务。
	KnowledgeProposals int      `json:"knowledge_proposals"`
	KnowledgeActions   []string `json:"knowledge_actions"`
}

func newDreamCycleTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input dreamCycleInput) (dreamCycleOutput, error) {
		if input.AgentID == "" {
			return dreamCycleOutput{}, ErrAgentIDRequired
		}

		// Step 1: Measure quality before.
		reportBefore, err := deps.Analytics.AnalyzeMemoryQuality(ctx, input.AgentID, time.Now().AddDate(0, 0, -30))
		if err != nil {
			return dreamCycleOutput{}, err
		}
		qualityBefore := reportBefore.HealthScore

		if input.effectiveDryRun() {
			actions := []string{"dry_run: would execute forget_low_quality, forget_inactive, deduplicate, consolidate"}
			out := dreamCycleOutput{ActionsTaken: actions}
			out.QualityBefore = qualityBefore
			// M4：dry_run 下知识治理做只读预估（decay 走 COUNT，提案不落库）。
			// 枚举全部团队库逐库预估（多库覆盖盲区修复），报告合并：提案数求和、任务名去重。
			if deps.Knowledge != nil {
				if reps, cerr := deps.Knowledge.CurateAllTeamKnowledge(ctx, bizknowledge.CurateOptions{DryRun: true}); cerr != nil {
					deps.LG.Warn("dream_cycle: knowledge curate dry-run preview failed",
						loggateway.StepID("memory_butler.dream.curate_preview"),
						loggateway.Err(cerr))
				} else {
					proposals, knowledgeActions := aggregateCurateReports(reps)
					out.KnowledgeProposals = proposals
					out.KnowledgeActions = knowledgeActions
					out.ActionsTaken = append(out.ActionsTaken, "dry_run: would execute curate_knowledge")
				}
			}
			return out, nil
		}

		var actions []string
		totalDeleted := 0
		totalMerged := 0
		totalDistilled := 0

		// Collect IDs of facts that are actually deleted for snapshot.
		var deletedFactSnapshots []biz.FactSnapshot

		// Step 2: Execute forget_low_quality logic.
		lowQualIDs := findLowQualityFactIDs(ctx, deps, input.AgentID)
		if len(lowQualIDs) > 0 {
			// Snapshot must be taken BEFORE deletion, otherwise ListFactRows
			// returns nothing and the snapshot is permanently empty.
			lowQualSnapshots := buildFactSnapshotsForIDs(ctx, deps, input.AgentID, lowQualIDs)
			deleted, delErr := deps.MemoryAdmin.DeleteFactRowsByIDs(ctx, lowQualIDs)
			if delErr != nil {
				deps.LG.Warn("dream_cycle: forget_low_quality delete failed",
					loggateway.StepID("memory_butler.dream.forget_low_quality"),
					loggateway.Str("agent_id", input.AgentID),
					loggateway.Err(delErr))
			} else {
				totalDeleted += deleted
				actions = append(actions, "forget_low_quality")
				deletedFactSnapshots = append(deletedFactSnapshots, lowQualSnapshots...)
			}
		}

		// Step 3: Execute forget_inactive logic.
		inactiveIDs := findInactiveFactIDs(ctx, deps, input.AgentID, defaultInactiveThresholdDays)
		if len(inactiveIDs) > 0 {
			inactiveSnapshots := buildFactSnapshotsForIDs(ctx, deps, input.AgentID, inactiveIDs)
			deleted, delErr := deps.MemoryAdmin.DeleteFactRowsByIDs(ctx, inactiveIDs)
			if delErr != nil {
				deps.LG.Warn("dream_cycle: forget_inactive delete failed",
					loggateway.StepID("memory_butler.dream.forget_inactive"),
					loggateway.Str("agent_id", input.AgentID),
					loggateway.Err(delErr))
			} else {
				totalDeleted += deleted
				actions = append(actions, "forget_inactive")
				deletedFactSnapshots = append(deletedFactSnapshots, inactiveSnapshots...)
			}
		}

		// Step 4: Execute deduplicate logic.
		dedupIDs := findDuplicateFactIDs(ctx, deps, input.AgentID, defaultSimilarityThreshold)
		if len(dedupIDs) > 0 {
			dedupSnapshots := buildFactSnapshotsForIDs(ctx, deps, input.AgentID, dedupIDs)
			deleted, delErr := deps.MemoryAdmin.DeleteFactRowsByIDs(ctx, dedupIDs)
			if delErr != nil {
				deps.LG.Warn("dream_cycle: deduplicate delete failed",
					loggateway.StepID("memory_butler.dream.deduplicate"),
					loggateway.Str("agent_id", input.AgentID),
					loggateway.Err(delErr))
			} else {
				totalMerged += deleted
				actions = append(actions, "deduplicate_memories")
				deletedFactSnapshots = append(deletedFactSnapshots, dedupSnapshots...)
			}
		}

		// Step 5: Execute consolidate_episodes logic.
		distilled, consErr := consolidateEpisodesCount(ctx, deps, input.AgentID)
		if consErr != nil {
			deps.LG.Warn("dream_cycle: consolidate_episodes failed",
				loggateway.StepID("memory_butler.dream.consolidate"),
				loggateway.Str("agent_id", input.AgentID),
				loggateway.Err(consErr))
		} else if distilled > 0 {
			totalDistilled += distilled
			actions = append(actions, "consolidate_episodes")
		}

		// Step 5.5: curate knowledge entries (M4 自治理层)——低风险自动应用，
		// 高风险仅产 pending 提案待人工二审；失败 Warn 降级，不中断梦境主流程。
		// 枚举全部团队库逐库治理（多库覆盖盲区修复），报告合并：提案数求和、任务名去重。
		knowledgeProposals := 0
		var knowledgeActions []string
		if deps.Knowledge != nil {
			reps, cerr := deps.Knowledge.CurateAllTeamKnowledge(ctx, bizknowledge.CurateOptions{DryRun: false})
			if cerr != nil {
				deps.LG.Warn("dream_cycle: curate_knowledge failed",
					loggateway.StepID("memory_butler.dream.curate_knowledge"),
					loggateway.Str("agent_id", input.AgentID),
					loggateway.Err(cerr))
			} else {
				proposals, acts := aggregateCurateReports(reps)
				knowledgeProposals = proposals
				knowledgeActions = acts
				actions = append(actions, "curate_knowledge")
			}
		}

		// Step 6: Save dream snapshot with only actually deleted facts.
		snapshot := biz.DreamSnapshot{
			ExecutedAt:   time.Now().UTC().Format(time.RFC3339),
			DeletedFacts: deletedFactSnapshots,
			MergedFacts:  nil,
		}
		snapJSON, marshalErr := json.Marshal(snapshot)
		if marshalErr != nil {
			deps.LG.Warn("dream_cycle: snapshot marshal failed",
				loggateway.StepID("memory_butler.dream.snapshot_marshal"),
				loggateway.Str("agent_id", input.AgentID),
				loggateway.Err(marshalErr))
		} else {
			settings, getErr := deps.Agents.GetAgentRuntimeSettings(ctx, input.AgentID)
			if getErr != nil {
				deps.LG.Warn("dream_cycle: get agent runtime settings failed, cannot save snapshot",
					loggateway.StepID("memory_butler.dream.settings_get"),
					loggateway.Str("agent_id", input.AgentID),
					loggateway.Err(getErr))
			} else {
				settings.DreamSnapshotJSON = string(snapJSON)
				if _, upsertErr := deps.Agents.UpsertAgentRuntimeSettings(ctx, settings); upsertErr != nil {
					deps.LG.Warn("dream snapshot persist failed",
						loggateway.StepID("memory_butler.dream_snapshot"),
						loggateway.Str("agent_id", input.AgentID),
						loggateway.Err(upsertErr))
				}
			}
		}

		// Step 7: Measure quality after.
		reportAfter, err := deps.Analytics.AnalyzeMemoryQuality(ctx, input.AgentID, time.Now().AddDate(0, 0, -30))
		if err != nil {
			return dreamCycleOutput{}, err
		}
		qualityAfter := reportAfter.HealthScore

		return dreamCycleOutput{
			QualityBefore:      qualityBefore,
			QualityAfter:       qualityAfter,
			ActionsTaken:       actions,
			DeletedCount:       totalDeleted,
			MergedCount:        totalMerged,
			DistilledCount:     totalDistilled,
			KnowledgeProposals: knowledgeProposals,
			KnowledgeActions:   knowledgeActions,
		}, nil
	}

	return function.NewFunctionTool(
		execute,
		function.WithName("memory_butler_dream_cycle"),
		function.WithDescription("梦境循环：一次性执行记忆管家全部维护流程（遗忘低质量、遗忘不活跃、去重、整合情景），并对比前后健康评分。支持 dry_run 模式预览。"),
	)
}

// aggregateCurateReports 合并多库治理报告：pending 提案数求和，治理任务名按首现去重。
func aggregateCurateReports(reps []bizknowledge.CurateReport) (proposals int, actions []string) {
	seen := make(map[string]struct{})
	for _, rep := range reps {
		proposals += rep.ProposalsPending
		for _, a := range rep.Actions {
			if _, ok := seen[a]; ok {
				continue
			}
			seen[a] = struct{}{}
			actions = append(actions, a)
		}
	}
	return proposals, actions
}

// findLowQualityFactIDs returns fact IDs that are misaligned (high negative feedback rate).
func findLowQualityFactIDs(ctx context.Context, deps Deps, agentID string) []string {
	rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, biz.ListFactRowsParams{
		ScopeType: "agent",
		ScopeID:   agentID,
		Limit:     defaultFactListLimit,
		Offset:    0,
	})
	if err != nil {
		return nil
	}
	var ids []string
	for _, raw := range rows {
		m, _ := jsonutil.ParseMap(raw)
		if m == nil {
			continue
		}
		factID := jsonutil.IfaceStr(m, "id")
		hitCount := jsonutil.IfaceI32(m, "hit_count")
		negCount := jsonutil.IfaceI32(m, "negative_feedback_count")
		if factID != "" && hitCount >= minHitCountForLowQuality && negCount > 0 && float64(negCount)/float64(hitCount) > negativeFeedbackRateThreshold {
			ids = append(ids, factID)
		}
	}
	return ids
}

// findInactiveFactIDs returns fact IDs that have not been updated within the threshold days.
func findInactiveFactIDs(ctx context.Context, deps Deps, agentID string, thresholdDays int) []string {
	if thresholdDays <= 0 {
		thresholdDays = defaultInactiveThresholdDays
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -thresholdDays)

	rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, biz.ListFactRowsParams{
		ScopeType: "agent",
		ScopeID:   agentID,
		Limit:     defaultFactListLimit,
		Offset:    0,
	})
	if err != nil {
		return nil
	}
	var ids []string
	for _, raw := range rows {
		m, _ := jsonutil.ParseMap(raw)
		if m == nil {
			continue
		}
		factID := jsonutil.IfaceStr(m, "id")
		if factID == "" {
			continue
		}
		updatedAt := jsonutil.IfaceStr(m, "updated_at")
		if updatedAt == "" {
			// No updated_at means the fact was never updated — treat as inactive.
			ids = append(ids, factID)
			continue
		}
		t, parseErr := time.Parse(time.RFC3339, updatedAt)
		if parseErr != nil {
			continue
		}
		if t.Before(cutoff) {
			ids = append(ids, factID)
		}
	}
	return ids
}

// findDuplicateFactIDs returns fact IDs that are duplicates (older version of similar pairs).
func findDuplicateFactIDs(ctx context.Context, deps Deps, agentID string, threshold float64) []string {
	if threshold <= 0 {
		threshold = defaultSimilarityThreshold
	}
	rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, biz.ListFactRowsParams{
		ScopeType: "agent",
		ScopeID:   agentID,
		Limit:     defaultFactListLimit,
		Offset:    0,
	})
	if err != nil {
		return nil
	}

	type factEntry struct {
		ID        string
		Statement string
		UpdatedAt string
	}
	var facts []factEntry
	for _, raw := range rows {
		m, _ := jsonutil.ParseMap(raw)
		if m == nil {
			continue
		}
		id := jsonutil.IfaceStr(m, "id")
		stmt := jsonutil.IfaceStr(m, "statement")
		updated := jsonutil.IfaceStr(m, "updated_at")
		if id == "" || stmt == "" {
			continue
		}
		facts = append(facts, factEntry{ID: id, Statement: stmt, UpdatedAt: updated})
	}

	var toDelete []string
	seen := make(map[string]bool)
	for i := 0; i < len(facts); i++ {
		if seen[facts[i].ID] {
			continue
		}
		for j := i + 1; j < len(facts); j++ {
			if seen[facts[j].ID] {
				continue
			}
			if stringSimilarity(facts[i].Statement, facts[j].Statement) >= threshold {
				if facts[i].UpdatedAt >= facts[j].UpdatedAt {
					toDelete = append(toDelete, facts[j].ID)
					seen[facts[j].ID] = true
				} else {
					toDelete = append(toDelete, facts[i].ID)
					seen[facts[i].ID] = true
					break
				}
			}
		}
	}
	return toDelete
}

// consolidateEpisodesCount consolidates episodic facts and returns the distilled count.
// It also deletes the original episodic facts after distillation, consistent with
// the standalone consolidate_episodes tool.
func consolidateEpisodesCount(ctx context.Context, deps Deps, agentID string) (int, error) {
	rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, biz.ListFactRowsParams{
		ScopeType: "agent",
		ScopeID:   agentID,
		Kind:      "episode",
		Limit:     defaultFactListLimit,
		Offset:    0,
	})
	if err != nil || len(rows) == 0 {
		return 0, err
	}

	seen := make(map[string]bool)
	var unique []string
	var episodeIDs []string
	for _, raw := range rows {
		m, _ := jsonutil.ParseMap(raw)
		if m == nil {
			continue
		}
		stmt := jsonutil.IfaceStr(m, "statement")
		if stmt == "" {
			continue
		}
		if id := jsonutil.IfaceStr(m, "id"); id != "" {
			episodeIDs = append(episodeIDs, id)
		}
		lower := strings.ToLower(stmt)
		if !seen[lower] {
			seen[lower] = true
			unique = append(unique, stmt)
		}
	}

	if len(unique) == 0 {
		return 0, nil
	}

	// Build distilled summary.
	var sb strings.Builder
	sb.WriteString("[Distilled from episodes] ")
	for i, s := range unique {
		if i > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(s)
	}

	_, err = deps.MemoryAdmin.UpsertFactRow(ctx, biz.FactUpsert{
		Statement:  sb.String(),
		ScopeType:  "agent",
		ScopeID:    agentID,
		AgentID:    agentID,
		FactKind:   "semantic",
		SourceKind: "consolidate_episodes",
		Status:     "active",
	})
	if err != nil {
		return 0, err
	}

	// Delete the original episodic facts now that they have been distilled.
	// Best-effort: log but don't fail — the distillation succeeded.
	if len(episodeIDs) > 0 {
		if _, delErr := deps.MemoryAdmin.DeleteFactRowsByIDs(ctx, episodeIDs); delErr != nil {
			deps.LG.Warn("dream_cycle: consolidate_episodes failed to delete original episodic facts",
				loggateway.StepID("memory_butler.dream.consolidate.delete_episodes"),
				loggateway.Str("agent_id", agentID),
				loggateway.Err(delErr))
		}
	}
	return len(unique), nil
}

// buildFactSnapshotsForIDs retrieves fact snapshots for the given IDs.
// It fetches all facts for the agent and filters by the given IDs.
// Errors during individual fact parsing are logged but do not fail the batch.
func buildFactSnapshotsForIDs(ctx context.Context, deps Deps, agentID string, ids []string) []biz.FactSnapshot {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	rows, _, _, _, listErr := deps.MemoryAdmin.ListFactRows(ctx, biz.ListFactRowsParams{
		ScopeType: "agent",
		ScopeID:   agentID,
		Limit:     defaultFactListLimit,
		Offset:    0,
	})
	if listErr != nil {
		return nil
	}
	var snapshots []biz.FactSnapshot
	for _, raw := range rows {
		m, parseErr := jsonutil.ParseMap(raw)
		if parseErr != nil {
			if deps.LG != nil {
				deps.LG.Warn("dream_cycle: failed to parse fact row for snapshot",
					loggateway.StepID("memory_butler.dream.snapshot_parse"),
					loggateway.Err(parseErr))
			}
			continue
		}
		if m == nil {
			continue
		}
		factID := jsonutil.IfaceStr(m, "id")
		if !idSet[factID] {
			continue
		}
		snapshots = append(snapshots, biz.FactSnapshot{
			ID:        factID,
			Statement: jsonutil.IfaceStr(m, "statement"),
			ScopeType: jsonutil.IfaceStr(m, "scope_type"),
			ScopeID:   jsonutil.IfaceStr(m, "scope_id"),
			Kind:      jsonutil.IfaceStr(m, "kind"),
		})
	}
	return snapshots
}
