package sessionmemory

import (
	"context"
	"sort"
)

// RecallL2EpisodesDebug returns scored L2 candidates with breakdown for admin tooling.
func (st *Store) RecallL2EpisodesDebug(ctx context.Context, agentID, sessionID, query string, limit int32) ([]RecallDebugRow, error) {
	return st.RecallL2EpisodesScored(ctx, agentID, sessionID, query, nil, limit)
}

// RecallL3FactsDebug returns scored L3 candidates with breakdown for admin tooling.
func (st *Store) RecallL3FactsDebug(ctx context.Context, scopeType, scopeID, userID, query string, limit int32) ([]RecallDebugRow, error) {
	return st.RecallL3FactsScored(ctx, scopeType, scopeID, userID, query, nil, limit)
}

// CompositeSearchMemories merges L2 + L3 recall hits sorted by fused score.
func (st *Store) CompositeSearchMemories(ctx context.Context, agentID, sessionID, userID, query string, limit int32) ([]RecallDebugRow, error) {
	lim := int(limit)
	if lim <= 0 {
		lim = 10
	}
	l2, err := st.RecallL2EpisodesScored(ctx, agentID, sessionID, query, nil, int32(lim))
	if err != nil {
		return nil, err
	}
	l3, err := st.RecallL3FactsScored(ctx, "agent", agentID, userID, query, nil, int32(lim))
	if err != nil {
		return nil, err
	}
	merged := append(l2, l3...)
	sort.Slice(merged, func(i, j int) bool { return merged[i].Scores.Total > merged[j].Scores.Total })
	if len(merged) > lim {
		merged = merged[:lim]
	}
	return merged, nil
}
