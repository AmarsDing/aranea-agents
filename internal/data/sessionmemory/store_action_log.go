package sessionmemory

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MemoryActionLogInsert records a policy-level memory mutation (Ledger audit trail).
type MemoryActionLogInsert struct {
	Action         string
	TargetKind     string
	TargetID       string
	Reason         string
	PolicyVersion  string
	TurnID         string
	SourceEventIDs []string
	MetadataJSON   string
}

// InsertMemoryActionLog appends one row to memory_action_log. Failures are returned to the caller;
// upstream writers may log and continue when audit is best-effort.
func (st *Store) InsertMemoryActionLog(ctx context.Context, in MemoryActionLogInsert) error {
	if st == nil || st.client == nil {
		return errors.New("session memory store not wired")
	}
	return st.insertMemoryActionLogOn(ctx, st.client, in)
}

func (st *Store) insertMemoryActionLogOn(ctx context.Context, db sqlRunner, in MemoryActionLogInsert) error {
	if db == nil {
		return errors.New("db runner is required")
	}
	action := strings.TrimSpace(in.Action)
	if action == "" {
		return errors.New("action is required")
	}
	targetKind := strings.TrimSpace(in.TargetKind)
	targetID := strings.TrimSpace(in.TargetID)
	if targetKind == "" || targetID == "" {
		return errors.New("target_kind and target_id are required")
	}
	srcJSON, err := json.Marshal(in.SourceEventIDs)
	if err != nil {
		return err
	}
	meta := strings.TrimSpace(in.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	if turnID := strings.TrimSpace(in.TurnID); turnID != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(meta), &m); err != nil || m == nil {
			m = map[string]any{}
		}
		m["turn_id"] = turnID
		b, err := json.Marshal(m)
		if err != nil {
			return err
		}
		meta = string(b)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.ExecContext(ctx, `
INSERT INTO memory_action_log (
 id, action, target_kind, target_id, reason, policy_version, source_event_ids_json, metadata_json, created_at
) VALUES (?,?,?,?,?,?,?,?,?)`,
		uuid.NewString(), action, targetKind, targetID,
		strings.TrimSpace(in.Reason), strings.TrimSpace(in.PolicyVersion),
		string(srcJSON), meta, now)
	return err
}

func (st *Store) insertMemoryActionLogBestEffort(ctx context.Context, in MemoryActionLogInsert) {
	if st == nil {
		return
	}
	_ = st.InsertMemoryActionLog(ctx, in)
}
