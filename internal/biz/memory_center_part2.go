package biz

import (
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/jsonutil"
	"context"
	"encoding/json"
	"strings"
	"time"
)

// ListEpisodesAdmin returns paginated L2 episodes (created_at DESC) for one
// agent, optionally filtered by session. Reader-backed; no new SQL here.
func (uc *MemoryAdminUsecase) ListEpisodesAdmin(ctx context.Context, agentID, sessionID string, limit, offset int32) ([]MemoryEpisodeItem, int32, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, 0, err
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, 0, apierror.BadRequest(apierror.DomainMemory, "agent id is required")
	}
	if uc.l2AdminReader == nil {
		return nil, 0, apierror.Internal(apierror.DomainMemory, "memory center readers not wired")
	}
	if limit <= 0 {
		limit = episodesAdminDefaultLimit
	}
	if limit > episodesAdminMaxLimit {
		limit = episodesAdminMaxLimit
	}
	if offset < 0 {
		offset = 0
	}
	rows, total, _, err := uc.l2AdminReader.ListEpisodeRowsAdmin(ctx, agentID, strings.TrimSpace(sessionID), limit, offset)
	if err != nil {
		return nil, 0, err
	}
	items := make([]MemoryEpisodeItem, 0, len(rows))
	for _, raw := range rows {
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		items = append(items, MemoryEpisodeItem{
			ID:                  jsonutil.IfaceStr(m, "id"),
			SessionID:           jsonutil.IfaceStr(m, "session_id"),
			AgentID:             jsonutil.IfaceStr(m, "agent_id"),
			Kind:                jsonutil.IfaceStr(m, "episode_kind"),
			Title:               jsonutil.IfaceStr(m, "title"),
			OutcomeSummary:      jsonutil.IfaceStr(m, "outcome_summary"),
			Importance:          jsonutil.IfaceF64(m, "importance"),
			ConsolidationStatus: jsonutil.IfaceStr(m, "consolidation_status"),
			ConsolidatedL3Count: jsonutil.IfaceI32(m, "consolidated_l3_count"),
			EndedAt:             jsonutil.IfaceStr(m, "ended_at"),
			CreatedAt:           jsonutil.IfaceStr(m, "created_at"),
		})
	}
	return items, total, nil
}

// heaviestGraphNode returns the node ID with the max weight (ties broken by ID
// for determinism). Returns "" for an empty graph.
func heaviestGraphNode(nodes map[string]*UnifiedGraphNode) string {
	best := ""
	var bestW float64
	for id, n := range nodes {
		if best == "" || n.Weight > bestW || (n.Weight == bestW && id < best) {
			best, bestW = id, n.Weight
		}
	}
	return best
}

func parseJSONStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// memCreatedToday reports whether a created_at timestamp (RFC3339 variants)
// falls on/after today 00:00 UTC.
func memCreatedToday(createdAt string, todayStart time.Time) bool {
	t := parseMemTime(createdAt)
	return !t.IsZero() && !t.Before(todayStart)
}

func parseMemTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
