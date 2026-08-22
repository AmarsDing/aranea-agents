package data

import (
	"context"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent/stepv2"
	"aranea-agents/internal/data/ent/taskv2"
	"aranea-agents/internal/data/vector"

	entsql "entgo.io/ent/dialect/sql"
)

// MemoryAdminAdapter composes all memory shim repos. It implements the
// fine-grained L0–L4 biz ports and the deprecated SessionAdminStore (tests /
// compile-time check only). Production code must consume the narrow interfaces
// (via NewMemoryLayerPorts / MemoryAdminDeps), not biz.SessionAdminStore.
type MemoryAdminAdapter struct {
	*l0SnapshotRepo
	*l1WorkingMemoryRepo
	*l2EpisodeRepo
	*l3FactRepo
	*l4EntityRepo
}

var (
	_ biz.SessionAdminStore         = (*MemoryAdminAdapter)(nil)
	_ biz.MemoryAdminDeps           = (*MemoryAdminAdapter)(nil)
	_ biz.L1SchemaReader            = (*MemoryAdminAdapter)(nil)
	_ biz.L3FactWriter              = (*MemoryAdminAdapter)(nil)
	_ biz.MemoryOverviewStatsReader = (*MemoryAdminAdapter)(nil)
)

// NewSessionAdminStoreAdapter creates the L0–L4 shim adapter.
// The return type is the concrete adapter (not biz.SessionAdminStore) so
// production Wire can assign it to narrow ports including L1SchemaReader.
func NewSessionAdminStoreAdapter(data *Data, vs vector.VectorStore) *MemoryAdminAdapter {
	if data == nil {
		return nil
	}
	return &MemoryAdminAdapter{
		l0SnapshotRepo:      newL0SnapshotRepo(data),
		l1WorkingMemoryRepo: newL1WorkingMemoryRepo(data),
		l2EpisodeRepo:       newL2EpisodeRepo(data, vs),
		l3FactRepo:          newL3FactRepo(data, vs),
		l4EntityRepo:        newL4EntityRepo(data),
	}
}

// NewMemoryLayerPorts maps one adapter instance onto the ISP DTO used by
// MemorySet / agent builder. Returns a zero value when data is nil.
func NewMemoryLayerPorts(data *Data, vs vector.VectorStore) biz.MemoryLayerPorts {
	a := NewSessionAdminStoreAdapter(data, vs)
	if a == nil {
		return biz.MemoryLayerPorts{}
	}
	return biz.MemoryLayerPorts{
		L0Admin:    a,
		L1Reader:   a,
		L1Tasks:    a,
		L1Fields:   a,
		L1Schema:   a,
		L3Reader:   a,
		L3Writer:   a,
		L4Entities: a,
	}
}

// recentMessageListerAdapter implements biz.RecentMessageLister using v2 tables
// (tasks_v2 + steps_v2). The activities table was dropped (DDL 20261012).
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

// stepKindToConsolidateRole maps a v2 Step kind to a ConsolidateMessage role.
// Returns ok=false for kinds that don't map to chat messages (thinking/confirm/…).
func stepKindToConsolidateRole(kind string) (string, bool) {
	switch strings.TrimSpace(kind) {
	case "reply":
		return "assistant", true
	case "action":
		return "tool", true
	case "notice", "error":
		return "system", true
	default:
		return "", false
	}
}

type timedConsolidateMsg struct {
	ts  time.Time
	msg biz.ConsolidateMessage
}

func (a *recentMessageListerAdapter) ListRecentMessages(ctx context.Context, sessionID string, limit int) ([]biz.ConsolidateMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	if limit <= 0 || limit > biz.TimelineMessageMaxFetch {
		limit = biz.TimelineMessageMaxFetch
	}
	client := a.data.RW().Read(ctx)
	fetchN := limit * 2

	var timed []timedConsolidateMsg

	tasks, err := client.TaskV2.Query().
		Where(taskv2.SessionIDEQ(sessionID)).
		Order(taskv2.ByCreatedAt(entsql.OrderDesc()), taskv2.BySeq(entsql.OrderDesc())).
		Limit(fetchN).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_TASK_V2")
	}
	for _, row := range tasks {
		content := strings.TrimSpace(row.UserMessage)
		if content == "" {
			continue
		}
		timed = append(timed, timedConsolidateMsg{
			ts:  row.CreatedAt,
			msg: biz.ConsolidateMessage{Role: "user", Content: content, MessageID: row.ID},
		})
	}

	steps, err := client.StepV2.Query().
		Where(stepv2.SessionIDEQ(sessionID)).
		Order(stepv2.ByStartedAt(entsql.OrderDesc()), stepv2.BySeq(entsql.OrderDesc())).
		Limit(fetchN).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_STEP_V2")
	}
	for _, row := range steps {
		role, ok := stepKindToConsolidateRole(row.Kind)
		if !ok {
			continue
		}
		content := strings.TrimSpace(row.Content)
		if role == "tool" {
			if tr := strings.TrimSpace(row.ToolResult); tr != "" {
				content = tr
			} else if content == "" {
				content = strings.TrimSpace(row.ToolName)
			}
		}
		if content == "" {
			continue
		}
		timed = append(timed, timedConsolidateMsg{
			ts:  row.StartedAt,
			msg: biz.ConsolidateMessage{Role: role, Content: content, MessageID: row.ID},
		})
	}

	sort.Slice(timed, func(i, j int) bool {
		if timed[i].ts.Equal(timed[j].ts) {
			return timed[i].msg.MessageID < timed[j].msg.MessageID
		}
		return timed[i].ts.Before(timed[j].ts)
	})
	if len(timed) > limit {
		timed = timed[len(timed)-limit:]
	}
	out := make([]biz.ConsolidateMessage, 0, len(timed))
	for _, t := range timed {
		out = append(out, t.msg)
	}
	return out, nil
}
