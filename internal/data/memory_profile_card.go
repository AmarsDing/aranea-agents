package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"

	"github.com/google/uuid"
)

// memoryProfileCardRepo implements the resident profile card ports (FR-12.7)
// over the memory_profile_cards table via raw SQL. One card per
// (agent_id, user_id) — enforced by the unique index; upsert bumps version.
type memoryProfileCardRepo struct {
	data *Data
}

// Compile-time interface checks.
var (
	_ biz.MemoryProfileCardReader = (*memoryProfileCardRepo)(nil)
	_ biz.MemoryProfileCardWriter = (*memoryProfileCardRepo)(nil)
)

// NewMemoryProfileCardStore creates the profile card reader/writer backed by
// data. The same instance satisfies both biz.MemoryProfileCardReader and
// biz.MemoryProfileCardWriter. Returns nil when data is nil.
func NewMemoryProfileCardStore(data *Data) *memoryProfileCardRepo {
	if data == nil {
		return nil
	}
	return &memoryProfileCardRepo{data: data}
}

// GetProfileCard loads the resident card for (agent, user). Returns
// (nil, nil) when no card exists — the inject hook treats that as "skip".
func (r *memoryProfileCardRepo) GetProfileCard(ctx context.Context, agentID, userID string) (*biz.ProfileCard, error) {
	if r == nil || r.data == nil {
		return nil, nil
	}
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(`
		SELECT content, fact_count, version, updated_at
		FROM memory_profile_cards
		WHERE agent_id = ? AND user_id = ?`), strings.TrimSpace(agentID), strings.TrimSpace(userID))
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_PROFILE_CARD")
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, entErrToBizErr(rows.Err(), "MEMORY_PROFILE_CARD")
	}
	var c biz.ProfileCard
	if err := rows.Scan(&c.Content, &c.FactCount, &c.Version, &c.UpdatedAt); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_PROFILE_CARD")
	}
	c.AgentID = strings.TrimSpace(agentID)
	c.UserID = strings.TrimSpace(userID)
	return &c, nil
}

// UpsertProfileCard inserts or replaces the resident card for
// (agent, user), bumping version on conflict. Empty content is rejected by
// the caller (distiller); this method stores whatever it is given.
func (r *memoryProfileCardRepo) UpsertProfileCard(ctx context.Context, card biz.ProfileCard) error {
	if r == nil || r.data == nil {
		return nil
	}
	agentID := strings.TrimSpace(card.AgentID)
	if agentID == "" {
		return nil
	}
	userID := strings.TrimSpace(card.UserID)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
		INSERT INTO memory_profile_cards (id, agent_id, user_id, content, fact_count, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT(agent_id, user_id) DO UPDATE SET
			content = excluded.content,
			fact_count = excluded.fact_count,
			version = memory_profile_cards.version + 1,
			updated_at = excluded.updated_at`),
		uuid.NewString(), agentID, userID, card.Content, card.FactCount, now, now)
	return entErrToBizErr(err, "MEMORY_PROFILE_CARD")
}

// DeleteProfileCard removes the resident card for (agent, user) — called by
// the distiller when no active source facts remain, so a stale card never
// outlives its facts. Idempotent: deleting a missing card is a no-op.
func (r *memoryProfileCardRepo) DeleteProfileCard(ctx context.Context, agentID, userID string) error {
	if r == nil || r.data == nil {
		return nil
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
		DELETE FROM memory_profile_cards WHERE agent_id = ? AND user_id = ?`),
		strings.TrimSpace(agentID), strings.TrimSpace(userID))
	return entErrToBizErr(err, "MEMORY_PROFILE_CARD")
}
