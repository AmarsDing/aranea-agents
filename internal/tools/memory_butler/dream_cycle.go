package memory_butler

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
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
	DryRun  bool   `json:"dry_run" jsonschema:"description=仅预览不实际执行,default=true"`
}

type dreamCycleOutput struct {
	QualityBefore  float64  `json:"quality_before"`
	QualityAfter   float64  `json:"quality_after"`
	ActionsTaken   []string `json:"actions_taken"`
	DeletedCount   int      `json:"deleted_count"`
	MergedCount    int      `json:"merged_count"`
	DistilledCount int      `json:"distilled_count"`
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

		if input.DryRun {
			return dreamCycleOutput{
				QualityBefore:  qualityBefore,
				QualityAfter:   0,
				ActionsTaken:   []string{"dry_run: would execute forget_low_quality, forget_inactive, deduplicate, consolidate"},
				DeletedCount:   0,
				MergedCount:    0,
				DistilledCount: 0,
			}, nil
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
			deleted, delErr := deps.MemoryAdmin.DeleteFactRowsByIDs(ctx, lowQualIDs)
			if delErr != nil {
				deps.LG.Warn("dream_cycle: forget_low_quality delete failed",
					loggateway.StepID("memory_butler.dream.forget_low_quality"),
					loggateway.Str("agent_id", input.AgentID),
					loggateway.Err(delErr))
			} else {
				totalDeleted += deleted
				actions = append(actions, "forget_low_quality")
				deletedFactSnapshots = append(deletedFactSnapshots, buildFactSnapshotsForIDs(ctx, deps, input.AgentID, lowQualIDs)...)
			}
		}

		// Step 3: Execute forget_inactive logic.
		inactiveIDs := findInactiveFactIDs(ctx, deps, input.AgentID, defaultInactiveThresholdDays)
		if len(inactiveIDs) > 0 {
			deleted, delErr := deps.MemoryAdmin.DeleteFactRowsByIDs(ctx, inactiveIDs)
			if delErr != nil {
				deps.LG.Warn("dream_cycle: forget_inactive delete failed",
					loggateway.StepID("memory_butler.dream.forget_inactive"),
					loggateway.Str("agent_id", input.AgentID),
					loggateway.Err(delErr))
			} else {
				totalDeleted += deleted
				actions = append(actions, "forget_inactive")
				deletedFactSnapshots = append(deletedFactSnapshots, buildFactSnapshotsForIDs(ctx, deps, input.AgentID, inactiveIDs)...)
			}
		}

		// Step 4: Execute deduplicate logic.
		dedupIDs := findDuplicateFactIDs(ctx, deps, input.AgentID, defaultSimilarityThreshold)
		if len(dedupIDs) > 0 {
			deleted, delErr := deps.MemoryAdmin.DeleteFactRowsByIDs(ctx, dedupIDs)
			if delErr != nil {
				deps.LG.Warn("dream_cycle: deduplicate delete failed",
					loggateway.StepID("memory_butler.dream.deduplicate"),
					loggateway.Str("agent_id", input.AgentID),
					loggateway.Err(delErr))
			} else {
				totalMerged += deleted
				actions = append(actions, "deduplicate_memories")
				deletedFactSnapshots = append(deletedFactSnapshots, buildFactSnapshotsForIDs(ctx, deps, input.AgentID, dedupIDs)...)
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
			QualityBefore:  qualityBefore,
			QualityAfter:   qualityAfter,
			ActionsTaken:   actions,
			DeletedCount:   totalDeleted,
			MergedCount:    totalMerged,
			DistilledCount: totalDistilled,
		}, nil
	}

	return function.NewFunctionTool(
		execute,
		function.WithName("memory_butler_dream_cycle"),
		function.WithDescription("梦境循环：一次性执行记忆管家全部维护流程（遗忘低质量、遗忘不活跃、去重、整合情景），并对比前后健康评分。支持 dry_run 模式预览。"),
	)
}

// findLowQualityFactIDs returns fact IDs that are misaligned (high negative feedback rate).
func findLowQualityFactIDs(ctx context.Context, deps Deps, agentID string) []string {
	rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, "agent", agentID, "", "", "", defaultFactListLimit, 0)
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

	rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, "agent", agentID, "", "", "", defaultFactListLimit, 0)
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
	rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, "agent", agentID, "", "", "", defaultFactListLimit, 0)
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
func consolidateEpisodesCount(ctx context.Context, deps Deps, agentID string) (int, error) {
	rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, "agent", agentID, "episode", "", "", defaultFactListLimit, 0)
	if err != nil || len(rows) == 0 {
		return 0, err
	}

	seen := make(map[string]bool)
	var unique []string
	for _, raw := range rows {
		m, _ := jsonutil.ParseMap(raw)
		if m == nil {
			continue
		}
		stmt := jsonutil.IfaceStr(m, "statement")
		if stmt == "" {
			continue
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
	rows, _, _, _, listErr := deps.MemoryAdmin.ListFactRows(ctx, "agent", agentID, "", "", "", defaultFactListLimit, 0)
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
