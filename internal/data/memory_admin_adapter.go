package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent/activity"
	"aranea-agents/internal/data/vector"

	entsql "entgo.io/ent/dialect/sql"
)

// sessionAdminStoreAdapter composes all memory shim repos to implement biz.SessionAdminStore.
type sessionAdminStoreAdapter struct {
	*l0SnapshotRepo
	*l1WorkingMemoryRepo
	*l2EpisodeRepo
	*l3FactRepo
	*l4EntityRepo
}

// Compile-time interface check.
var _ biz.SessionAdminStore = (*sessionAdminStoreAdapter)(nil)

// NewSessionAdminStoreAdapter creates a SessionAdminStore by composing all shim repos.
func NewSessionAdminStoreAdapter(data *Data, vs vector.VectorStore) biz.SessionAdminStore {
	if data == nil {
		return nil
	}
	return &sessionAdminStoreAdapter{
		l0SnapshotRepo:      newL0SnapshotRepo(data),
		l1WorkingMemoryRepo: newL1WorkingMemoryRepo(data),
		l2EpisodeRepo:       newL2EpisodeRepo(data, vs),
		l3FactRepo:          newL3FactRepo(data, vs),
		l4EntityRepo:        newL4EntityRepo(data),
	}
}

// recentMessageListerAdapter implements biz.RecentMessageLister using the Ent client.
type recentMessageListerAdapter struct {
	data *Data
}

var _ biz.RecentMessageLister = (*recentMessageListerAdapter)(nil)

// NewRecentMessageLister creates a RecentMessageLister backed by the data layer.
func NewRecentMessageLister(d *Data) biz.RecentMessageLister {
	if d == nil {
		return nil
	}
	return &recentMessageListerAdapter{data: d}
}

// activityKindToConsolidateRole maps an Activity kind to a ConsolidateMessage role.
// Returns ok=false for kinds that don't map to chat messages (thinking/session/etc.).
func activityKindToConsolidateRole(kind string) (string, bool) {
	switch strings.TrimSpace(kind) {
	case "task":
		return "user", true
	case "reply":
		return "assistant", true
	case "action":
		return "tool", true
	case "notice":
		return "system", true
	default:
		return "", false
	}
}

func (a *recentMessageListerAdapter) ListRecentMessages(ctx context.Context, sessionID string, limit int) ([]biz.ConsolidateMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	if limit <= 0 || limit > biz.TimelineMessageMaxFetch {
		limit = biz.TimelineMessageMaxFetch
	}
	// Phase 1c-3: messages table deleted. Recent messages are now sourced from
	// the activities table. Only chat-shaped kinds (task/reply/action/notice)
	// are returned; thinking/session/team_stage/etc. are skipped.
	rows, err := a.data.RW().Read(ctx).Activity.Query().
		Where(activity.SessionIDEQ(sessionID)).
		Order(activity.ByTimestamp(entsql.OrderDesc()), activity.BySeq(entsql.OrderDesc())).
		Limit(limit * 2).All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_ACTIVITY")
	}
	// Reverse to chronological order and filter to chat-shaped kinds.
	out := make([]biz.ConsolidateMessage, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		row := rows[i]
		role, ok := activityKindToConsolidateRole(row.Kind)
		if !ok {
			continue
		}
		content := strings.TrimSpace(row.Content)
		if role == "tool" {
			// Tool messages use ToolResult as content (falls back to tool name).
			if tr := strings.TrimSpace(row.ToolResult); tr != "" {
				content = tr
			} else if content == "" {
				content = strings.TrimSpace(row.ToolName)
			}
		}
		if content == "" {
			continue
		}
		out = append(out, biz.ConsolidateMessage{
			Role:      role,
			Content:   content,
			MessageID: row.ID,
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
