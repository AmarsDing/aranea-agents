package wal

import (
	"context"
	"sync"
	"time"
)

// MemoryStorage is an in-memory Storage implementation for testing.
type MemoryStorage struct {
	mu      sync.RWMutex
	entries map[string]Entry
}

// NewMemoryStorage creates a new in-memory WAL storage.
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		entries: make(map[string]Entry),
	}
}

func (s *MemoryStorage) Insert(ctx context.Context, entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.entries[entry.ID]; exists {
		return nil // idempotent
	}
	s.entries[entry.ID] = entry
	return nil
}

func (s *MemoryStorage) MarkPublished(ctx context.Context, id string, publishedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.entries[id]; ok {
		entry.Published = true
		entry.PublishedAt = publishedAt
		s.entries[id] = entry
	}
	return nil
}

func (s *MemoryStorage) ListUnpublished(ctx context.Context) ([]Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []Entry
	for _, e := range s.entries {
		if !e.Published {
			result = append(result, e)
		}
	}
	return result, nil
}

func (s *MemoryStorage) PurgePublished(ctx context.Context, cutoff time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var purged int64
	for id, e := range s.entries {
		if e.Published && e.CreatedAt.Before(cutoff) {
			delete(s.entries, id)
			purged++
		}
	}
	return purged, nil
}

func (s *MemoryStorage) Close() error {
	return nil
}
