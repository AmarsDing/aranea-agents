package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent/message"
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

func (a *recentMessageListerAdapter) ListRecentMessages(ctx context.Context, sessionID string, limit int) ([]biz.ConsolidateMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	if limit <= 0 || limit > biz.TimelineMessageMaxFetch {
		limit = biz.TimelineMessageMaxFetch
	}
	rows, err := a.data.RW().Read(ctx).Message.Query().
		Where(message.SessionIDEQ(sessionID)).
		Order(message.ByTurnID(entsql.OrderDesc()), message.BySeqInTurn(entsql.OrderDesc()), message.ByCreatedAt(entsql.OrderDesc())).
		Limit(limit).All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_MESSAGE")
	}
	// Reverse to chronological order.
	out := make([]biz.ConsolidateMessage, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		m := rows[i]
		content := strings.TrimSpace(m.ContentMarkdown)
		if content == "" {
			continue
		}
		role := strings.TrimSpace(m.Role)
		if role == "" {
			continue
		}
		out = append(out, biz.ConsolidateMessage{
			Role:      role,
			Content:   content,
			MessageID: m.ID,
		})
	}
	return out, nil
}
