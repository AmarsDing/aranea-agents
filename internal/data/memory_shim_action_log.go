package data

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
)

// actionLogRepo implements biz.MemoryActionLogWriter using direct Raw SQL.
type actionLogRepo struct {
	data *Data
}

func newActionLogRepo(data *Data) *actionLogRepo {
	if data == nil {
		return nil
	}
	return &actionLogRepo{data: data}
}

// NewMemoryActionLogWriter creates a biz.MemoryActionLogWriter backed by data.
func NewMemoryActionLogWriter(data *Data) biz.MemoryActionLogWriter {
	if data == nil {
		return nil
	}
	return newActionLogRepo(data)
}

// Compile-time interface check.
var _ biz.MemoryActionLogWriter = (*actionLogRepo)(nil)

func (r *actionLogRepo) WriteMemoryActionLog(ctx context.Context, rec biz.MemoryPolicyRecord) error {
	id := newUUIDString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	sourceEventIDs := "[]"
	if len(rec.SourceEventIDs) > 0 {
		if b, err := json.Marshal(rec.SourceEventIDs); err == nil {
			sourceEventIDs = string(b)
		}
	}
	meta := strings.TrimSpace(rec.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `INSERT INTO memory_action_logs (
		id, action, target_kind, target_id, reason, policy_version, turn_id, source_event_ids_json, metadata_json, created_at
	) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		id,
		strings.TrimSpace(rec.Action),
		strings.TrimSpace(rec.TargetKind),
		strings.TrimSpace(rec.TargetID),
		strings.TrimSpace(rec.Reason),
		strings.TrimSpace(rec.PolicyVersion),
		strings.TrimSpace(rec.TurnID),
		sourceEventIDs, meta, now,
	)
	return err
}
