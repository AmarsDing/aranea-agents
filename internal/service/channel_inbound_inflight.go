package service

import (
	"strings"
	"sync"
	"time"
)

const inflightEntryTTL = 30 * time.Minute

type inflightEntry struct {
	acquiredAt time.Time
}

type inboundInflightSet struct {
	mu sync.Mutex
	m  map[string]inflightEntry
}

func newInboundInflightSet() *inboundInflightSet {
	s := &inboundInflightSet{m: make(map[string]inflightEntry)}
	go s.cleanupLoop()
	return s
}

func (s *inboundInflightSet) tryAcquire(key string) bool {
	if key == "" {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if entry, ok := s.m[key]; ok && now.Sub(entry.acquiredAt) < inflightEntryTTL {
		return false
	}
	s.m[key] = inflightEntry{acquiredAt: now}
	return true
}

func (s *inboundInflightSet) release(key string) {
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
}

func (s *inboundInflightSet) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for k, v := range s.m {
			if now.Sub(v.acquiredAt) >= inflightEntryTTL {
				delete(s.m, k)
			}
		}
		s.mu.Unlock()
	}
}

func truncateForLog(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
