package service

import (
	"context"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/event/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type EventService struct {
	v1.UnimplementedEventServiceServer
	store    *biz.EventStoreUsecase
	sessions *biz.SessionUsecase
}

func NewEventService(store *biz.EventStoreUsecase, sessions *biz.SessionUsecase) *EventService {
	return &EventService{store: store, sessions: sessions}
}

func (s *EventService) ListEvents(ctx context.Context, req *v1.ListEventsRequest) (*v1.ListEventsResponse, error) {
	if s == nil || s.store == nil {
		return nil, kerrors.InternalServer("EVENT", "event store not configured")
	}
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, kerrors.BadRequest("EVENT", "session_id is required")
	}
	if s.sessions == nil {
		return nil, kerrors.InternalServer("EVENT", "session service not configured")
	}
	if _, err := s.sessions.Get(ctx, sessionID); err != nil {
		return nil, mapSessionErr(err)
	}
	q := biz.EventStoreQuery{
		SessionID: sessionID,
		Type:      strings.TrimSpace(req.GetType()),
		Limit:     int(req.GetLimit()),
		Offset:    int(req.GetOffset()),
	}
	if since := strings.TrimSpace(req.GetSince()); since != "" {
		t, err := parseEventTime(since)
		if err != nil {
			return nil, kerrors.BadRequest("EVENT", "invalid since: "+err.Error())
		}
		q.Since = t
	}
	if until := strings.TrimSpace(req.GetUntil()); until != "" {
		t, err := parseEventTime(until)
		if err != nil {
			return nil, kerrors.BadRequest("EVENT", "invalid until: "+err.Error())
		}
		q.Until = t
	}
	result, err := s.store.List(ctx, q)
	if err != nil {
		return nil, err
	}
	items := make([]*v1.EnvelopeRecord, 0, len(result.Items))
	for _, row := range result.Items {
		items = append(items, &v1.EnvelopeRecord{
			Id:           row.ID,
			SessionId:    row.SessionID,
			Type:         row.Type,
			Author:       row.Author,
			Channel:      row.Channel,
			EnvelopeJson: row.EnvelopeJSON,
			CreatedAt:    row.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return &v1.ListEventsResponse{
		Items: items,
		Total: int32(result.Total),
	}, nil
}

func parseEventTime(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}
