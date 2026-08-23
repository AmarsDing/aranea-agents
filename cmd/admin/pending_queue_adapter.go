package main

import (
	"context"

	"aranea-agents/internal/data"
	"aranea-agents/internal/runtime"
)

type dataPendingQueueStore struct {
	d *data.Data
}

func newDataPendingQueueStore(d *data.Data) runtime.PendingQueueStore {
	if d == nil {
		return nil
	}
	return &dataPendingQueueStore{d: d}
}

func (s *dataPendingQueueStore) LoadAll(ctx context.Context) (map[string][]runtime.PendingMessage, error) {
	rows, err := s.d.LoadPendingQueueEntries(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]runtime.PendingMessage, len(rows))
	for _, e := range rows {
		out[e.SessionID] = append(out[e.SessionID], runtime.PendingMessage{
			ID:        e.EntryID,
			Content:   e.Content,
			Status:    e.Status,
			CreatedAt: e.CreatedAt,
			Priority:  e.Priority,
			Kind:      e.Kind,
		})
	}
	return out, nil
}

func (s *dataPendingQueueStore) ReplaceAll(ctx context.Context, queues map[string][]runtime.PendingMessage) error {
	var entries []data.PendingQueueEntry
	for sid, list := range queues {
		for _, e := range list {
			entries = append(entries, data.PendingQueueEntry{
				SessionID: sid,
				EntryID:   e.ID,
				Content:   e.Content,
				Status:    e.Status,
				CreatedAt: e.CreatedAt,
				Priority:  e.Priority,
				Kind:      e.Kind,
			})
		}
	}
	return s.d.ReplacePendingQueueEntries(ctx, entries)
}
