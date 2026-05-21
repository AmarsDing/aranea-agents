package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"

	"github.com/google/uuid"
)

type hookDeliveryRepo struct {
	data *Data
}

// NewHookDeliveryRepo implements biz.HookDeliveryRepo.
func NewHookDeliveryRepo(data *Data) biz.HookDeliveryRepo {
	return &hookDeliveryRepo{data: data}
}

func (r *hookDeliveryRepo) Insert(ctx context.Context, d biz.HookDelivery) error {
	if r == nil || r.data == nil || r.data.RawDB() == nil {
		return nil
	}
	id := strings.TrimSpace(d.ID)
	if id == "" {
		id = uuid.NewString()
	}
	now := strings.TrimSpace(d.CreatedAt)
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	updated := strings.TrimSpace(d.UpdatedAt)
	if updated == "" {
		updated = now
	}
	maxAttempts := d.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	_, err := r.data.RawDB().ExecContext(ctx, `
INSERT INTO hook_deliveries (id, hook_key, hook_id, webhook_url, payload_json, status, attempt_count, max_attempts, last_error, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, d.HookKey, d.HookID, d.WebhookURL, d.PayloadJSON, string(biz.NormalizeHookDeliveryStatus(string(d.Status))),
		d.AttemptCount, maxAttempts, d.LastError, now, updated,
	)
	return err
}

func (r *hookDeliveryRepo) UpdateResult(ctx context.Context, id string, status biz.HookDeliveryStatus, attemptCount int, lastError string) error {
	if r == nil || r.data == nil || r.data.RawDB() == nil {
		return nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.data.RawDB().ExecContext(ctx, `
UPDATE hook_deliveries SET status = ?, attempt_count = ?, last_error = ?, updated_at = ? WHERE id = ?`,
		string(status), attemptCount, lastError, now, id,
	)
	return err
}

func (r *hookDeliveryRepo) List(ctx context.Context, q biz.HookDeliveryQuery) (biz.HookDeliveryListResult, error) {
	if r == nil || r.data == nil || r.data.RawDB() == nil {
		return biz.HookDeliveryListResult{}, nil
	}
	limit := int(q.Limit)
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := int(q.Offset)
	if offset < 0 {
		offset = 0
	}
	where := " WHERE 1=1"
	args := []any{}
	if k := strings.TrimSpace(q.HookKey); k != "" {
		where += " AND hook_key = ?"
		args = append(args, k)
	}
	if k := strings.TrimSpace(q.Status); k != "" {
		where += " AND status = ?"
		args = append(args, strings.ToLower(k))
	}
	if k := strings.TrimSpace(q.From); k != "" {
		where += " AND created_at >= ?"
		args = append(args, k)
	}
	if k := strings.TrimSpace(q.To); k != "" {
		where += " AND created_at <= ?"
		args = append(args, k)
	}
	var total int32
	if err := r.data.RawDB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hook_deliveries"+where, args...).Scan(&total); err != nil {
		return biz.HookDeliveryListResult{}, err
	}
	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := r.data.RawDB().QueryContext(ctx, `
SELECT id, hook_key, hook_id, webhook_url, payload_json, status, attempt_count, max_attempts, last_error, created_at, updated_at
FROM hook_deliveries`+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return biz.HookDeliveryListResult{}, err
	}
	defer rows.Close()
	items := make([]biz.HookDelivery, 0, limit)
	for rows.Next() {
		var d biz.HookDelivery
		var status string
		if err := rows.Scan(&d.ID, &d.HookKey, &d.HookID, &d.WebhookURL, &d.PayloadJSON, &status,
			&d.AttemptCount, &d.MaxAttempts, &d.LastError, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return biz.HookDeliveryListResult{}, err
		}
		d.Status = biz.NormalizeHookDeliveryStatus(status)
		items = append(items, d)
	}
	return biz.HookDeliveryListResult{Items: items, Total: total, Limit: int32(limit), Offset: int32(offset)}, rows.Err()
}
