package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/eventstore"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type eventStoreRepo struct {
	data *Data
}

var _ biz.EventStoreRepo = (*eventStoreRepo)(nil)

func NewEventStoreRepo(d *Data) biz.EventStoreRepo {
	return &eventStoreRepo{data: d}
}

func (r *eventStoreRepo) Insert(ctx context.Context, rec biz.EventStoreRecord) error {
	if r == nil || r.data == nil || r.data.Ent() == nil {
		return kerrors.InternalServer("EVENT_STORE", "database not configured")
	}
	_, err := r.data.Ent().EventStore.Create().
		SetID(rec.ID).
		SetSessionID(rec.SessionID).
		SetType(rec.Type).
		SetAuthor(rec.Author).
		SetChannel(rec.Channel).
		SetEnvelopeJSON(rec.EnvelopeJSON).
		SetCreatedAt(rec.CreatedAt).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil
		}
		return kerrors.InternalServer("EVENT_STORE", "insert event_store failed").WithCause(err)
	}
	return nil
}

func (r *eventStoreRepo) List(ctx context.Context, q biz.EventStoreQuery) (biz.EventStoreListResult, error) {
	if r == nil || r.data == nil || r.data.Ent() == nil {
		return biz.EventStoreListResult{}, kerrors.InternalServer("EVENT_STORE", "database not configured")
	}
	client := r.data.Ent()
	query := client.EventStore.Query().
		Where(eventstore.SessionIDEQ(strings.TrimSpace(q.SessionID)))
	if !q.Since.IsZero() {
		query = query.Where(eventstore.CreatedAtGTE(q.Since))
	}
	if !q.Until.IsZero() {
		query = query.Where(eventstore.CreatedAtLTE(q.Until))
	}
	if t := strings.TrimSpace(q.Type); t != "" {
		query = query.Where(eventstore.TypeEQ(t))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return biz.EventStoreListResult{}, kerrors.InternalServer("EVENT_STORE", "count event_store failed").WithCause(err)
	}
	rows, err := query.
		Order(ent.Asc(eventstore.FieldCreatedAt)).
		Limit(q.Limit).
		Offset(q.Offset).
		All(ctx)
	if err != nil {
		return biz.EventStoreListResult{}, kerrors.InternalServer("EVENT_STORE", "list event_store failed").WithCause(err)
	}
	items := make([]biz.EventStoreRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, biz.EventStoreRecord{
			ID:           row.ID,
			SessionID:    row.SessionID,
			Type:         row.Type,
			Author:       row.Author,
			Channel:      row.Channel,
			EnvelopeJSON: row.EnvelopeJSON,
			CreatedAt:    row.CreatedAt,
		})
	}
	return biz.EventStoreListResult{Items: items, Total: total}, nil
}

func (r *eventStoreRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	if r == nil || r.data == nil || r.data.Ent() == nil {
		return 0, kerrors.InternalServer("EVENT_STORE", "database not configured")
	}
	n, err := r.data.Ent().EventStore.Delete().
		Where(eventstore.CreatedAtLT(cutoff)).
		Exec(ctx)
	if err != nil {
		return 0, kerrors.InternalServer("EVENT_STORE", "delete event_store failed").WithCause(err)
	}
	return int64(n), nil
}
