package service

import (
	"sync"
	"time"

	"aranea-agents/pkg/safego"
)

const (
	sessionLockMaxIdle     = 30 * time.Minute
	sessionLockSweepPeriod = 5 * time.Minute
)

type sessionLockEntry struct {
	mu       *sync.Mutex
	lastUsed time.Time
}

type sessionLockManager struct {
	mu      sync.Mutex
	entries map[string]*sessionLockEntry
	stopCh  chan struct{}
}

func NewSessionLockManager() *sessionLockManager {
	m := &sessionLockManager{
		entries: make(map[string]*sessionLockEntry),
		stopCh:  make(chan struct{}),
	}
	safego.Go(nil, "session-lock-sweep", m.sweepLoop)
	return m
}

func (m *sessionLockManager) Lock(sessionID string) func() {
	entry := m.getOrCreate(sessionID)
	entry.mu.Lock()
	return entry.mu.Unlock
}

func (m *sessionLockManager) getOrCreate(sessionID string) *sessionLockEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[sessionID]; ok {
		e.lastUsed = time.Now()
		return e
	}
	e := &sessionLockEntry{
		mu:       &sync.Mutex{},
		lastUsed: time.Now(),
	}
	m.entries[sessionID] = e
	return e
}

func (m *sessionLockManager) sweepLoop() {
	ticker := time.NewTicker(sessionLockSweepPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.sweep()
		case <-m.stopCh:
			return
		}
	}
}

func (m *sessionLockManager) sweep() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for id, e := range m.entries {
		if now.Sub(e.lastUsed) > sessionLockMaxIdle {
			delete(m.entries, id)
		}
	}
}

func (m *sessionLockManager) close() {
	close(m.stopCh)
}

func (m *sessionLockManager) len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}
