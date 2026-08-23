package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

// PendingQueueEntry is one durable follow-up queue row.
type PendingQueueEntry struct {
	SessionID string
	EntryID   string
	Content   string
	Status    string
	CreatedAt string
	Priority  int
	Kind      string
}

// LoadPendingQueueEntries returns follow-up queue rows newer than two hours.
func (d *Data) LoadPendingQueueEntries(ctx context.Context) ([]PendingQueueEntry, error) {
	if d == nil || d.RWDB() == nil {
		return nil, nil
	}
	q := d.Dialect().RenumberPlaceholders(
		`SELECT session_id, entry_id, content, status, created_at, priority, kind FROM pending_queue_entries ORDER BY created_at ASC`)
	rows, err := d.RWDB().ReadDB(ctx).QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingQueueEntry
	cutoff := time.Now().Add(-2 * time.Hour)
	for rows.Next() {
		var e PendingQueueEntry
		if err := rows.Scan(&e.SessionID, &e.EntryID, &e.Content, &e.Status, &e.CreatedAt, &e.Priority, &e.Kind); err != nil {
			return nil, err
		}
		if t, err := time.Parse(time.RFC3339, e.CreatedAt); err == nil && !t.After(cutoff) {
			continue
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ReplacePendingQueueEntries atomically replaces all follow-up queue rows.
func (d *Data) ReplacePendingQueueEntries(ctx context.Context, entries []PendingQueueEntry) error {
	if d == nil || d.RWDB() == nil {
		return nil
	}
	return d.ExecInTx(ctx, func(txCtx context.Context) error {
		exec := d.RWDB().WriteDB(txCtx)
		if _, err := exec.ExecContext(txCtx, `DELETE FROM pending_queue_entries`); err != nil {
			return err
		}
		ins := d.Dialect().RenumberPlaceholders(
			`INSERT INTO pending_queue_entries (session_id, entry_id, content, status, created_at, priority, kind) VALUES (?, ?, ?, ?, ?, ?, ?)`)
		for _, e := range entries {
			sid := strings.TrimSpace(e.SessionID)
			id := strings.TrimSpace(e.EntryID)
			if sid == "" || id == "" {
				continue
			}
			if _, err := exec.ExecContext(txCtx, ins, sid, id, e.Content, e.Status, e.CreatedAt, e.Priority, e.Kind); err != nil {
				if d.lg != nil {
					d.lg.Warn("pending queue insert failed",
						loggateway.StepID("data.pending_queue.replace"),
						loggateway.Str("session_id", sid),
						loggateway.Err(err))
				}
				return err
			}
		}
		return nil
	})
}
