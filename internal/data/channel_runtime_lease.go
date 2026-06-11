package data

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"aranea-agents/internal/biz"
)

type channelRuntimeLeaseRepo struct {
	data *Data
}

var _ biz.ChannelRuntimeLeaseRepo = (*channelRuntimeLeaseRepo)(nil)

func NewChannelRuntimeLeaseRepo(d *Data) biz.ChannelRuntimeLeaseRepo {
	if d == nil {
		return &channelRuntimeLeaseRepo{}
	}
	return &channelRuntimeLeaseRepo{data: d}
}

func EnsureChannelRuntimeLeaseSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS channel_runtime_lease (
  key TEXT PRIMARY KEY,
  channel_id TEXT NOT NULL DEFAULT '',
  platform TEXT NOT NULL DEFAULT '',
  owner_id TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_channel_runtime_lease_expires
  ON channel_runtime_lease(expires_at);
`)
	return err
}

func (r *channelRuntimeLeaseRepo) TryAcquireRuntimeLease(ctx context.Context, lease biz.RuntimeLease) (bool, error) {
	if r == nil || r.data == nil {
		return false, errors.New("channel runtime lease: repository unavailable")
	}
	if !lease.Valid() {
		return false, errors.New("channel runtime lease: invalid lease")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	exp := lease.ExpiresAt.UTC().Format(time.RFC3339Nano)
	db := r.data.RWDB().WriteDB(ctx)
	if db == nil {
		return false, errors.New("channel runtime lease: repository unavailable")
	}
	res, err := db.ExecContext(ctx, `
INSERT INTO channel_runtime_lease (key, channel_id, platform, owner_id, expires_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(key) DO UPDATE SET
  channel_id = excluded.channel_id,
  platform = excluded.platform,
  owner_id = excluded.owner_id,
  expires_at = excluded.expires_at,
  updated_at = excluded.updated_at
WHERE (channel_runtime_lease.owner_id = excluded.owner_id AND channel_runtime_lease.expires_at > ?)
   OR channel_runtime_lease.expires_at <= ?`,
		lease.Key, lease.ChannelID, lease.Platform, lease.OwnerID, exp, now, now, now, now,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *channelRuntimeLeaseRepo) RenewRuntimeLease(ctx context.Context, key, ownerID string, expiresAt time.Time) (bool, error) {
	if r == nil || r.data == nil {
		return false, errors.New("channel runtime lease: repository unavailable")
	}
	key = strings.TrimSpace(key)
	ownerID = strings.TrimSpace(ownerID)
	if key == "" || ownerID == "" || expiresAt.IsZero() {
		return false, errors.New("channel runtime lease: key, owner_id and expires_at are required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	db := r.data.RWDB().WriteDB(ctx)
	if db == nil {
		return false, errors.New("channel runtime lease: repository unavailable")
	}
	res, err := db.ExecContext(ctx, `
UPDATE channel_runtime_lease
SET expires_at = ?, updated_at = ?
WHERE key = ? AND owner_id = ? AND expires_at > ?`,
		expiresAt.UTC().Format(time.RFC3339Nano), now,
		key, ownerID, now,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *channelRuntimeLeaseRepo) ReleaseRuntimeLease(ctx context.Context, key, ownerID string) error {
	if r == nil || r.data == nil {
		return errors.New("channel runtime lease: repository unavailable")
	}
	key = strings.TrimSpace(key)
	ownerID = strings.TrimSpace(ownerID)
	if key == "" || ownerID == "" {
		return errors.New("channel runtime lease: key and owner_id are required")
	}
	db := r.data.RWDB().WriteDB(ctx)
	if db == nil {
		return errors.New("channel runtime lease: repository unavailable")
	}
	_, err := db.ExecContext(ctx, `DELETE FROM channel_runtime_lease WHERE key = ? AND owner_id = ?`, key, ownerID)
	return err
}
