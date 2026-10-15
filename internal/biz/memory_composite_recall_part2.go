package biz

import (
	"context"
	"strings"
)

// compositeRowScore extracts the calibrated scores.total annotated by the
// recall pipeline; falls back to the raw importance field when absent.
func compositeRowScore(row map[string]any) float64 {
	if sc, ok := row["scores"].(map[string]any); ok {
		if total, ok := sc["total"].(float64); ok && total > 0 {
			return total
		}
	}
	if v, ok := row["importance"].(float64); ok {
		return v
	}
	return 0
}

// ProactiveRecall retrieves memories based on the conversation context
// (mentioned entities, current topic, user statement) without requiring an
// explicit query. It is intended to be called before each conversation turn
// to surface relevant memories that the agent should consider.
//
// Returns empty list (not error) when no proactive recaller is wired or
// when the conversation context carries no usable signal.
func (uc *MemoryCompositeRecallUsecase) ProactiveRecall(ctx context.Context, agentID, userID string, convCtx ProactiveRecallContext) ([]CompositeRecallHit, error) {
	if uc == nil || uc.proactiveRecaller == nil {
		return nil, nil
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, nil
	}
	return uc.proactiveRecaller.ProactiveRecall(ctx, agentID, strings.TrimSpace(userID), convCtx)
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
