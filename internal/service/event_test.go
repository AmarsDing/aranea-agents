package service_test

import (
	"context"
	"testing"
	"time"

	eventv1 "aranea-agents/api/kratos/event/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type memEventStoreRepo struct {
	records []biz.EventStoreRecord
}

func (m *memEventStoreRepo) Insert(_ context.Context, rec biz.EventStoreRecord) error {
	m.records = append(m.records, rec)
	return nil
}

func (m *memEventStoreRepo) List(_ context.Context, q biz.EventStoreQuery) (biz.EventStoreListResult, error) {
	var items []biz.EventStoreRecord
	for _, rec := range m.records {
		if rec.SessionID != q.SessionID {
			continue
		}
		if q.Type != "" && rec.Type != q.Type {
			continue
		}
		items = append(items, rec)
	}
	total := len(items)
	if q.Offset >= len(items) {
		return biz.EventStoreListResult{Total: total}, nil
	}
	items = items[q.Offset:]
	if q.Limit > 0 && len(items) > q.Limit {
		items = items[:q.Limit]
	}
	return biz.EventStoreListResult{Items: items, Total: total}, nil
}

func (m *memEventStoreRepo) DeleteOlderThan(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func newEventServiceForTest(sessions map[string]biz.Session) (*service.EventService, *memEventStoreRepo) {
	repo := &memEventStoreRepo{}
	storeUC := biz.NewEventStoreUsecase(repo)
	sessionUC := biz.NewSessionUsecase(&batchSessionRepo{sessions: sessions}, nil, nil, nil, nil, nil, nil, nil)
	return service.NewEventService(storeUC, sessionUC), repo
}

func TestEventService_ListEvents_RequiresExistingSession(t *testing.T) {
	t.Parallel()
	svc, _ := newEventServiceForTest(map[string]biz.Session{
		"sess-1": {ID: "sess-1", Title: "ok"},
	})
	ctx := context.Background()

	_, err := svc.ListEvents(ctx, &eventv1.ListEventsRequest{SessionId: "missing"})
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	if !kerrors.IsNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}

	_, err = svc.ListEvents(ctx, &eventv1.ListEventsRequest{})
	if err == nil {
		t.Fatal("expected error for empty session_id")
	}
	if !kerrors.IsBadRequest(err) {
		t.Fatalf("expected bad request, got %v", err)
	}
}

func TestEventService_ListEvents_ReturnsStoredRecords(t *testing.T) {
	t.Parallel()
	svc, repo := newEventServiceForTest(map[string]biz.Session{
		"sess-1": {ID: "sess-1"},
	})
	ctx := context.Background()
	repo.records = append(repo.records, biz.EventStoreRecord{
		ID:           "ev-1",
		SessionID:    "sess-1",
		Type:         "tool_call",
		EnvelopeJSON: `{"id":"ev-1","type":"tool_call"}`,
		CreatedAt:    time.Now().UTC(),
	})

	resp, err := svc.ListEvents(ctx, &eventv1.ListEventsRequest{SessionId: "sess-1", Limit: 10})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].GetId() != "ev-1" {
		t.Fatalf("unexpected items: %+v", resp.Items)
	}
}
