package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

// sessionV2Repo implements biz.SessionV2Repo.
// Stability:evolving
type sessionV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.SessionV2Repo = (*sessionV2Repo)(nil)

// NewSessionV2Repo creates a new SessionV2Repo.
// Logger is preset with domain "SESSION_V2" per loggateway convention.
func NewSessionV2Repo(d *Data, lg loggateway.Logger) biz.SessionV2Repo {
	return &sessionV2Repo{data: d, lg: lg.With(loggateway.Domain("SESSION_V2"))}
}

// GetSession returns the SpiritSession by ID.
func (r *sessionV2Repo) GetSession(ctx context.Context, id string) (biz.SpiritSession, error) {
	if r == nil || r.data == nil {
		return biz.SpiritSession{}, fmt.Errorf("session v2 repo: database not configured")
	}
	row, err := r.data.RW().Read(ctx).SessionV2.Get(ctx, id)
	if err != nil {
		return biz.SpiritSession{}, entErrToBizErr(err, "SESSION_V2")
	}
	return entSessionV2ToBiz(row), nil
}

// CreateSession inserts a new SpiritSession.
func (r *sessionV2Repo) CreateSession(ctx context.Context, s biz.SpiritSession) (biz.SpiritSession, error) {
	if r == nil || r.data == nil {
		return biz.SpiritSession{}, fmt.Errorf("session v2 repo: database not configured")
	}
	row, err := r.data.RW().Write(ctx).SessionV2.Create().
		SetID(s.ID).
		SetUserID(s.UserID).
		SetSpiritAgentID(s.SpiritAgentID).
		SetStatus(string(s.Status)).
		SetCreatedAt(s.CreatedAt).
		SetUpdatedAt(s.UpdatedAt).
		Save(ctx)
	if err != nil {
		return biz.SpiritSession{}, entErrToBizErr(err, "SESSION_V2")
	}
	return entSessionV2ToBiz(row), nil
}

// UpdateSessionStatus patches the status field and bumps updated_at.
func (r *sessionV2Repo) UpdateSessionStatus(ctx context.Context, id string, status biz.SpiritSessionStatus) error {
	if r == nil || r.data == nil {
		return fmt.Errorf("session v2 repo: database not configured")
	}
	err := r.data.RW().Write(ctx).SessionV2.UpdateOneID(id).
		SetStatus(string(status)).
		SetUpdatedAt(time.Now().UTC()).
		Exec(ctx)
	return entErrToBizErr(err, "SESSION_V2")
}

// entSessionV2ToBiz converts an Ent SessionV2 row to biz.SpiritSession.
func entSessionV2ToBiz(row *ent.SessionV2) biz.SpiritSession {
	return biz.SpiritSession{
		ID:            row.ID,
		UserID:        row.UserID,
		SpiritAgentID: row.SpiritAgentID,
		Status:        biz.SpiritSessionStatus(row.Status),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}
