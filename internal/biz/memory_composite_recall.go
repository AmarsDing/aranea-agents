package biz

import (
	"context"
	"strings"
)

// CompositeRecallQuery merges L2 episodes and L3 facts by fused score.
type CompositeRecallQuery struct {
	AgentID   string
	SessionID string
	UserID    string
	Query     string
	Limit     int32
}

// CompositeRecallHit is one ranked line for composite prompt injection.
type CompositeRecallHit struct {
	Layer string
	Line  string
	Score float64
}

// SessionCompositeRecallStore loads fused L2+L3 candidates (implemented by sessionmemory.Store).
type SessionCompositeRecallStore interface {
	CompositeSearchMemories(ctx context.Context, agentID, sessionID, userID, query string, limit int32) ([]CompositeRecallStoreRow, error)
}

// CompositeRecallStoreRow is the store-neutral composite recall row.
type CompositeRecallStoreRow struct {
	Layer     string
	Title     string
	Summary   string
	Statement string
	Score     float64
}

// MemoryCompositeRecaller performs cross-layer L2+L3 recall for prompt injection.
type MemoryCompositeRecaller interface {
	RecallComposite(ctx context.Context, q CompositeRecallQuery) ([]CompositeRecallHit, error)
}

// MemoryCompositeRecallUsecase wraps SessionCompositeRecallStore.
type MemoryCompositeRecallUsecase struct {
	store SessionCompositeRecallStore
}

func NewMemoryCompositeRecallUsecase(store SessionCompositeRecallStore) *MemoryCompositeRecallUsecase {
	if store == nil {
		return nil
	}
	return &MemoryCompositeRecallUsecase{store: store}
}

func (uc *MemoryCompositeRecallUsecase) RecallComposite(ctx context.Context, q CompositeRecallQuery) ([]CompositeRecallHit, error) {
	if uc == nil || uc.store == nil {
		return nil, nil
	}
	agentID := strings.TrimSpace(q.AgentID)
	if agentID == "" {
		return nil, nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 8
	}
	if limit > 20 {
		limit = 20
	}
	rows, err := uc.store.CompositeSearchMemories(ctx, agentID, strings.TrimSpace(q.SessionID), strings.TrimSpace(q.UserID), strings.TrimSpace(q.Query), limit)
	if err != nil {
		return nil, err
	}
	out := make([]CompositeRecallHit, 0, len(rows))
	for _, row := range rows {
		line := formatCompositeRecallLine(row)
		if line == "" {
			continue
		}
		out = append(out, CompositeRecallHit{Layer: row.Layer, Line: line, Score: row.Score})
	}
	return out, nil
}

func formatCompositeRecallLine(row CompositeRecallStoreRow) string {
	layer := strings.ToUpper(strings.TrimSpace(row.Layer))
	switch layer {
	case "L2", "L2_EPISODE", "EPISODE":
		title := strings.TrimSpace(row.Title)
		summary := strings.TrimSpace(row.Summary)
		if title == "" {
			title = summary
		}
		if title == "" {
			return ""
		}
		if summary != "" && summary != title {
			return title + ": " + summary
		}
		return title
	default:
		stmt := strings.TrimSpace(row.Statement)
		if stmt == "" {
			stmt = strings.TrimSpace(row.Summary)
		}
		return stmt
	}
}
