package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// ClaimPendingDeliveries atomically claims due outbound rows for one worker.
// Postgres: SELECT ... FOR UPDATE SKIP LOCKED inside a transaction, then CAS
// status to sending. Lease-expired sending rows are reclaimable so a crashed
// replica cannot pin a delivery forever.
func (r *channelRepo) ClaimPendingDeliveries(ctx context.Context, limit int) ([]biz.ChannelDelivery, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	db := r.data.RWDB().WriteHandle()
	if db == nil {
		return nil, apierror.Internal("CHANNEL", "repository unavailable")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, entErrToBizErr(err, "CHANNEL")
	}
	defer func() { _ = tx.Rollback() }()

	now := nowRFC3339()
	leaseCutoff := time.Now().UTC().Add(-biz.OutboundDeliveryLease).Format(time.RFC3339)
	nextRetry := r.data.Dialect().JSONExtract("payload_json", "next_retry_at")
	query := r.data.Dialect().RenumberPlaceholders(fmt.Sprintf(`
WITH picked AS (
  SELECT id FROM channel_delivery
  WHERE status = ?
     OR (status = ? AND updated_at < ?)
     OR (status = ? AND (COALESCE(%s, '') = '' OR COALESCE(%s, '') <= ?))
  ORDER BY created_at ASC
  LIMIT ?
  FOR UPDATE SKIP LOCKED
)
UPDATE channel_delivery AS d
SET status = ?, updated_at = ?
FROM picked
WHERE d.id = picked.id
  AND (
    d.status IN (?, ?)
    OR (d.status = ? AND d.updated_at < ?)
  )
RETURNING d.id, d.channel_id, d.agent_id, d.idempotency_key, d.status, d.payload_json, d.error_message, d.created_at, d.updated_at
`, nextRetry, nextRetry))

	rows, err := tx.QueryContext(ctx, query,
		biz.ChannelDeliveryStatusPending,
		biz.ChannelDeliveryStatusSending, leaseCutoff,
		biz.ChannelDeliveryStatusRetry, now,
		limit,
		biz.ChannelDeliveryStatusSending, now,
		biz.ChannelDeliveryStatusPending, biz.ChannelDeliveryStatusRetry,
		biz.ChannelDeliveryStatusSending, leaseCutoff,
	)
	if err != nil {
		return nil, entErrToBizErr(err, "CHANNEL")
	}
	defer rows.Close()

	out := make([]biz.ChannelDelivery, 0, limit)
	for rows.Next() {
		var d biz.ChannelDelivery
		if scanErr := rows.Scan(
			&d.ID, &d.ChannelID, &d.AgentID, &d.IdempotencyKey, &d.Status,
			&d.PayloadJSON, &d.ErrorMessage, &d.CreatedAt, &d.UpdatedAt,
		); scanErr != nil {
			return nil, entErrToBizErr(scanErr, "CHANNEL")
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "CHANNEL")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := tx.Commit(); err != nil {
		return nil, entErrToBizErr(err, "CHANNEL")
	}
	return out, nil
}
