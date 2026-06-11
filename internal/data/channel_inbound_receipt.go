package data

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

type channelInboundReceiptRepo struct {
	data *Data
}

var _ biz.ChannelInboundReceiptRepo = (*channelInboundReceiptRepo)(nil)

// NewChannelInboundReceiptRepo implements biz.ChannelInboundReceiptRepo.
func NewChannelInboundReceiptRepo(d *Data) biz.ChannelInboundReceiptRepo {
	return &channelInboundReceiptRepo{data: d}
}

// TryClaim uses Raw SQL because it relies on ON CONFLICT DO NOTHING upsert
// which requires special handling not easily expressible via Ent's Create API.
func (r *channelInboundReceiptRepo) TryClaim(ctx context.Context, channelID, idempotencyKey, peerID, textPreview string) (bool, error) {
	channelID = strings.TrimSpace(channelID)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if channelID == "" || idempotencyKey == "" {
		return true, nil
	}
	db := r.data.RWDB().WriteDB(ctx)
	if db == nil {
		return false, apierror.Internal("CHANNEL_INBOUND_RECEIPT", "repository unavailable")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.ExecContext(ctx, `
INSERT INTO channel_inbound_receipt (id, channel_id, idempotency_key, peer_id, text_preview, created_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(channel_id, idempotency_key) DO NOTHING`,
		biz.NewInboundReceiptID(), channelID, idempotencyKey, strings.TrimSpace(peerID), strings.TrimSpace(textPreview), now,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// PurgeOldInboundReceipts removes receipts older than retention (best-effort housekeeping).
func PurgeOldInboundReceipts(ctx context.Context, db *sql.DB, retention time.Duration) error {
	if db == nil || retention <= 0 {
		return nil
	}
	cutoff := time.Now().UTC().Add(-retention).Format(time.RFC3339)
	_, err := db.ExecContext(ctx, `DELETE FROM channel_inbound_receipt WHERE created_at < ? AND created_at != ''`, cutoff)
	return err
}
