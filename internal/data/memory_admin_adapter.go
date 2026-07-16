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
