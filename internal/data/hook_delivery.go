package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizhook "aranea-agents/internal/biz/hook"

	"github.com/google/uuid"
)

type hookDeliveryRepo struct {
	data *Data
}

var _ bizhook.DeliveryRepo = (*hookDeliveryRepo)(nil)

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
	idempotencyKey := strings.TrimSpace(d.IdempotencyKey)
	if idempotencyKey != "" {
		_, err := r.data.RawDB().ExecContext(ctx, `
INSERT OR IGNORE INTO hook_deliveries (id, hook_key, hook_id, webhook_url, webhook_secret, payload_json, status, attempt_count, max_attempts, last_error, idempotency_key, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, d.HookKey, d.HookID, d.WebhookURL, d.WebhookSecret, d.PayloadJSON, string(biz.NormalizeHookDeliveryStatus(string(d.Status))),
			d.AttemptCount, maxAttempts, d.LastError, idempotencyKey, now, updated,
		)
		return err
	}
	_, err := r.data.RawDB().ExecContext(ctx, `
INSERT INTO hook_deliveries (id, hook_key, hook_id, webhook_url, webhook_secret, payload_json, status, attempt_count, max_attempts, last_error, idempotency_key, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, d.HookKey, d.HookID, d.WebhookURL, d.WebhookSecret, d.PayloadJSON, string(biz.NormalizeHookDeliveryStatus(string(d.Status))),
		d.AttemptCount, maxAttempts, d.LastError, idempotencyKey, now, updated,
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
	if r == nil || r.data == nil {
		return biz.HookDeliveryListResult{}, nil
	}
	readDB := r.data.ReadDB()
	if readDB == nil {
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
	if err := readDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM hook_deliveries"+where, args...).Scan(&total); err != nil {
		return biz.HookDeliveryListResult{}, err
	}
	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := readDB.QueryContext(ctx, `
SELECT id, hook_key, hook_id, webhook_url, webhook_secret, payload_json, status, attempt_count, max_attempts, last_error, idempotency_key, created_at, updated_at
FROM hook_deliveries`+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return biz.HookDeliveryListResult{}, err
	}
	defer rows.Close()
	items := make([]biz.HookDelivery, 0, limit)
	for rows.Next() {
		var d biz.HookDelivery
		var status string
		if err := rows.Scan(&d.ID, &d.HookKey, &d.HookID, &d.WebhookURL, &d.WebhookSecret, &d.PayloadJSON, &status,
			&d.AttemptCount, &d.MaxAttempts, &d.LastError, &d.IdempotencyKey, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return biz.HookDeliveryListResult{}, err
		}
		d.Status = biz.NormalizeHookDeliveryStatus(status)
		d.WebhookSecret = ""
		items = append(items, d)
	}
	return biz.HookDeliveryListResult{Items: items, Total: total, Limit: int32(limit), Offset: int32(offset)}, rows.Err()
}

// ListStalePending returns pending deliveries with updated_at older than updatedBefore
// and remaining attempts, ordered by created_at ASC (oldest first). (OUT-02 / HK-01)
func (r *hookDeliveryRepo) ListStalePending(ctx context.Context, updatedBefore time.Time, limit int) ([]biz.HookDelivery, error) {
	if r == nil || r.data == nil {
		return nil, nil
	}
	readDB := r.data.ReadDB()
	if readDB == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	cutoff := updatedBefore.UTC().Format(time.RFC3339)
	rows, err := readDB.QueryContext(ctx, `
SELECT id, hook_key, hook_id, webhook_url, webhook_secret, payload_json, status, attempt_count, max_attempts, last_error, idempotency_key, created_at, updated_at
FROM hook_deliveries
WHERE status = 'pending'
  AND attempt_count < max_attempts
  AND updated_at < ?
ORDER BY created_at ASC
LIMIT ?`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]biz.HookDelivery, 0, limit)
	for rows.Next() {
		var d biz.HookDelivery
		var status string
		if err := rows.Scan(&d.ID, &d.HookKey, &d.HookID, &d.WebhookURL, &d.WebhookSecret, &d.PayloadJSON, &status,
			&d.AttemptCount, &d.MaxAttempts, &d.LastError, &d.IdempotencyKey, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		d.Status = biz.NormalizeHookDeliveryStatus(status)
		items = append(items, d)
	}
	return items, rows.Err()
}

// TryClaimForRetry atomically increments attempt_count for the delivery only when
// its current count matches expectedAttemptCount. Returns true when the claim wins
// (this instance should proceed), false when another pod already claimed it.
// (OUT-02 / HK-01 — multi-pod safe optimistic lock)
func (r *hookDeliveryRepo) TryClaimForRetry(ctx context.Context, id string, expectedAttemptCount int) (bool, error) {
	if r == nil || r.data == nil || r.data.RawDB() == nil {
		return false, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := r.data.RawDB().ExecContext(ctx, `
UPDATE hook_deliveries
   SET attempt_count = attempt_count + 1,
       updated_at    = ?
 WHERE id             = ?
   AND status         = 'pending'
   AND attempt_count  = ?`, now, id, expectedAttemptCount)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}
