package data

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"aranea-agents/internal/biz"
)

// MemoryCompositeRecallAdapter maps composite recall to biz ports.
type MemoryCompositeRecallAdapter struct {
	data *Data
}

var _ biz.SessionCompositeRecallStore = (*MemoryCompositeRecallAdapter)(nil)

func NewMemoryCompositeRecallAdapter(data *Data) biz.SessionCompositeRecallStore {
	if data == nil {
		return nil
	}
	return &MemoryCompositeRecallAdapter{data: data}
}

func (a *MemoryCompositeRecallAdapter) CompositeSearchMemories(ctx context.Context, agentID, sessionID, userID, query string, limit int32) ([]biz.CompositeRecallStoreRow, error) {
	if a == nil || a.data == nil {
		return nil, nil
	}
	var all []biz.CompositeRecallStoreRow

	// L2 episodes
	l2 := newL2EpisodeRepo(a.data, nil)
	episodes, err := l2.RecallL2Episodes(ctx, agentID, sessionID, query, nil, limit)
	if err == nil {
		for _, raw := range episodes {
			var row map[string]any
			if json.Unmarshal(raw, &row) != nil {
				continue
			}
			title, _ := row["title"].(string)
			summary, _ := row["outcome_summary"].(string)
			var score float64
			if v, ok := row["importance"].(float64); ok {
				score = v
			}
			all = append(all, biz.CompositeRecallStoreRow{
				Layer:   "L2",
				Title:   title,
				Summary: summary,
				Score:   score,
			})
		}
	}

	// L3 facts
	l3 := newL3FactRepo(a.data, nil)
	facts, err := l3.RecallL3Facts(ctx, "agent", agentID, userID, query, nil, limit, 0)
	if err == nil {
		for _, raw := range facts {
			var row map[string]any
			if json.Unmarshal(raw, &row) != nil {
				continue
			}
			stmt, _ := row["statement"].(string)
			var score float64
			if v, ok := row["importance"].(float64); ok {
				score = v
			}
			// P2-04: extract provenance metadata from the raw fact row.
			factID, _ := row["id"].(string)
			srcSess, _ := row["source_session_id"].(string)
			var confidence float64
			if v, ok := row["confidence"].(float64); ok {
				confidence = v
			}
			var version int
			if v, ok := row["version"].(float64); ok {
				version = int(v)
			}
			all = append(all, biz.CompositeRecallStoreRow{
				Layer:         "L3",
				Statement:     stmt,
				Score:         score,
				FactID:        factID,
				SourceSession: srcSess,
				Confidence:    confidence,
				Version:       version,
			})
		}
	}

	sort.Slice(all, func(i, j int) bool { return all[i].Score > all[j].Score })
	if len(all) > int(limit) {
		all = all[:limit]
	}
	return all, nil
}

// ensure strings is referenced
var _ = strings.TrimSpace
