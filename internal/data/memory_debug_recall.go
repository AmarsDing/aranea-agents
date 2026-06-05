package data

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
)

type memoryDebugRecallAdapter struct {
	data *Data
}

var _ biz.MemoryDebugRecaller = (*memoryDebugRecallAdapter)(nil)

func NewMemoryDebugRecaller(data *Data) biz.MemoryDebugRecaller {
	if data == nil {
		return nil
	}
	return &memoryDebugRecallAdapter{data: data}
}

func (a *memoryDebugRecallAdapter) RecallL2EpisodesDebug(ctx context.Context, agentID, sessionID, query string, limit int32) ([]biz.RecallDebugRow, error) {
	l2 := newL2EpisodeRepo(a.data, nil)
	candidates, err := l2.ListEpisodeRowsForRecall(ctx, agentID, sessionID, l2RecallCandidatePool)
	if err != nil {
		return nil, err
	}
	tokens := tokenizeQuery(query)
	now := time.Now().UTC()
	var rows []biz.RecallDebugRow
	for _, raw := range candidates {
		var row map[string]any
		if json.Unmarshal(raw, &row) != nil {
			continue
		}
		title, _ := row["title"].(string)
		summary, _ := row["outcome_summary"].(string)
		id, _ := row["id"].(string)
		imp := anyFloat(row, "importance")
		endedAt, _ := row["ended_at"].(string)

		kwScore := keywordOverlapScore(tokens, title+" "+summary)
		decay := decayFactor(endedAt, now)
		total := l2ScoreWeightKeyword*kwScore + l2ScoreWeightImport*imp*decay

		rows = append(rows, biz.RecallDebugRow{
			Layer:   "L2",
			ID:      id,
			Title:   title,
			Summary: summary,
			Scores: biz.RecallScoreBreakdown{
				Keyword:    kwScore,
				Importance: imp * decay,
				Total:      total,
			},
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Scores.Total > rows[j].Scores.Total })
	if len(rows) > int(limit) {
		rows = rows[:limit]
	}
	return rows, nil
}

func (a *memoryDebugRecallAdapter) RecallL3FactsDebug(ctx context.Context, scopeType, scopeID, userID, query string, limit int32) ([]biz.RecallDebugRow, error) {
	l3 := newL3FactRepo(a.data, nil)
	candidates, err := l3.RecallL3Facts(ctx, scopeType, scopeID, userID, query, nil, limit, 0)
	if err != nil {
		return nil, err
	}
	var rows []biz.RecallDebugRow
	for _, raw := range candidates {
		var row map[string]any
		if json.Unmarshal(raw, &row) != nil {
			continue
		}
		id, _ := row["id"].(string)
		stmt, _ := row["statement"].(string)
		rows = append(rows, biz.RecallDebugRow{
			Layer:     "L3",
			ID:        id,
			Statement: stmt,
		})
	}
	return rows, nil
}

func (a *memoryDebugRecallAdapter) CompositeSearchMemories(ctx context.Context, agentID, sessionID, userID, query string, limit int32) ([]biz.RecallDebugRow, error) {
	var all []biz.RecallDebugRow
	l2Rows, err := a.RecallL2EpisodesDebug(ctx, agentID, sessionID, query, limit)
	if err == nil {
		all = append(all, l2Rows...)
	}
	l3Rows, err := a.RecallL3FactsDebug(ctx, "agent", agentID, userID, query, limit)
	if err == nil {
		all = append(all, l3Rows...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Scores.Total > all[j].Scores.Total })
	if len(all) > int(limit) {
		all = all[:limit]
	}
	return all, nil
}

type memoryFactIndexCounterAdapter struct {
	data *Data
}

var _ biz.MemoryFactIndexCounter = (*memoryFactIndexCounterAdapter)(nil)

func NewMemoryFactIndexCounter(data *Data) biz.MemoryFactIndexCounter {
	if data == nil {
		return nil
	}
	return &memoryFactIndexCounterAdapter{data: data}
}

func (a *memoryFactIndexCounterAdapter) CountFactsByIndexStatus(ctx context.Context) (fresh, stale, disabled int64, err error) {
	if err := queryRowScan(ctx, a.data.RWDB().ReadDB(ctx),
		`SELECT COALESCE(SUM(CASE WHEN embedding_status = 'fresh' THEN 1 ELSE 0 END), 0) FROM memory_facts WHERE status = 'active' AND deleted_at = ''`,
		nil, &fresh); err != nil {
		return 0, 0, 0, err
	}
	if err := queryRowScan(ctx, a.data.RWDB().ReadDB(ctx),
		`SELECT COALESCE(SUM(CASE WHEN embedding_status = 'stale' THEN 1 ELSE 0 END), 0) FROM memory_facts WHERE status = 'active' AND deleted_at = ''`,
		nil, &stale); err != nil {
		return 0, 0, 0, err
	}
	if err := queryRowScan(ctx, a.data.RWDB().ReadDB(ctx),
		`SELECT COALESCE(SUM(CASE WHEN embedding_status = 'disabled' THEN 1 ELSE 0 END), 0) FROM memory_facts WHERE status = 'active' AND deleted_at = ''`,
		nil, &disabled); err != nil {
		return 0, 0, 0, err
	}
	return fresh, stale, disabled, nil
}

// ensure strings is referenced
var _ = strings.TrimSpace
