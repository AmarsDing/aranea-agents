package memory_butler

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/jsonutil"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type dreamCycleInput struct {
	AgentID string `json:"agent_id" jsonschema:"description=Agent ID,required"`
	DryRun  bool   `json:"dry_run" jsonschema:"description=仅预览不实际执行,default=true"`
}

type dreamCycleOutput struct {
	QualityBefore float64  `json:"quality_before"`
	QualityAfter  float64  `json:"quality_after"`
	ActionsTaken  []string `json:"actions_taken"`
	DeletedCount  int      `json:"deleted_count"`
	MergedCount   int      `json:"merged_count"`
	DistilledCount int     `json:"distilled_count"`
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
				QualityBefore: qualityBefore,
				QualityAfter:  0,
				ActionsTaken:  []string{"dry_run: would execute forget_low_quality, forget_inactive, deduplicate, consolidate"},
				DeletedCount:  0,
				MergedCount:   0,
				DistilledCount: 0,
			}, nil
		}

		var actions []string
		totalDeleted := 0
		totalMerged := 0
		totalDistilled := 0

		// Step 2: Save snapshot of facts to be affected.
		snapshot := biz.DreamSnapshot{
			ExecutedAt:   time.Now().UTC().Format(time.RFC3339),
			DeletedFacts: nil,
			MergedFacts:  nil,
		}
		rows, _, _, _, listErr := deps.MemoryAdmin.ListFactRows(ctx, "agent", input.AgentID, "", "", "", 500, 0)
		if listErr == nil {
			for _, raw := range rows {
				m, _ := jsonutil.ParseMap(raw)
				if m == nil {
					continue
				}
				factSnap := biz.FactSnapshot{
					ID:        jsonutil.IfaceStr(m, "id"),
					Statement: jsonutil.IfaceStr(m, "statement"),
					ScopeType: jsonutil.IfaceStr(m, "scope_type"),
					ScopeID:   jsonutil.IfaceStr(m, "scope_id"),
					Kind:      jsonutil.IfaceStr(m, "kind"),
				}
				snapshot.DeletedFacts = append(snapshot.DeletedFacts, factSnap)
			}
		}

		// Step 3: Execute forget_low_quality logic.
		lowQualIDs := findLowQualityFactIDs(ctx, deps, input.AgentID)
		if len(lowQualIDs) > 0 {
			deleted, delErr := deps.MemoryAdmin.DeleteFactRowsByIDs(ctx, lowQualIDs)
			if delErr == nil {
				totalDeleted += deleted
				actions = append(actions, "forget_low_quality")
			}
		}

		// Step 4: Execute forget_inactive logic.
		inactiveIDs := findInactiveFactIDs(ctx, deps, input.AgentID, 30)
		if len(inactiveIDs) > 0 {
			deleted, delErr := deps.MemoryAdmin.DeleteFactRowsByIDs(ctx, inactiveIDs)
			if delErr == nil {
				totalDeleted += deleted
				actions = append(actions, "forget_inactive")
			}
		}

		// Step 5: Execute deduplicate logic.
		dedupIDs := findDuplicateFactIDs(ctx, deps, input.AgentID, 0.8)
		if len(dedupIDs) > 0 {
			deleted, delErr := deps.MemoryAdmin.DeleteFactRowsByIDs(ctx, dedupIDs)
			if delErr == nil {
				totalMerged += deleted
				actions = append(actions, "deduplicate_memories")
			}
		}

		// Step 6: Execute consolidate_episodes logic.
		distilled := consolidateEpisodesCount(ctx, deps, input.AgentID)
		if distilled > 0 {
			totalDistilled += distilled
			actions = append(actions, "consolidate_episodes")
		}

		// Step 7: Save dream snapshot to agent runtime settings.
		snapJSON, _ := json.Marshal(snapshot)
		settings, getErr := deps.Agents.GetAgentRuntimeSettings(ctx, input.AgentID)
		if getErr == nil {
			settings.DreamSnapshotJSON = string(snapJSON)
			_, _ = deps.Agents.UpsertAgentRuntimeSettings(ctx, settings)
		}

		// Step 8: Measure quality after.
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
	rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, "agent", agentID, "", "", "", 500, 0)
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
		if factID != "" && hitCount >= 3 && negCount > 0 && float64(negCount)/float64(hitCount) > 0.5 {
			ids = append(ids, factID)
		}
	}
	return ids
}

// findInactiveFactIDs returns fact IDs that have not been updated within the threshold days.
func findInactiveFactIDs(ctx context.Context, deps Deps, agentID string, thresholdDays int) []string {
	if thresholdDays <= 0 {
		thresholdDays = 30
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -thresholdDays)

	rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, "agent", agentID, "", "", "", 500, 0)
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
		updatedAt := jsonutil.IfaceStr(m, "updated_at")
		if factID == "" || updatedAt == "" {
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
		threshold = 0.8
	}
	rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, "agent", agentID, "", "", "", 500, 0)
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
func consolidateEpisodesCount(ctx context.Context, deps Deps, agentID string) int {
	rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, "agent", agentID, "episode", "", "", 500, 0)
	if err != nil || len(rows) == 0 {
		return 0
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
		return 0
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
		return 0
	}
	return len(unique)
}
