package biz

import (
	"context"
	"strings"
	"sync"
)

// TurnPreviewRegistry tracks one active preview coordinator per session (CH-BOR-07).
// Replacing an entry stops the previous subscription so concurrent turns do not cross-wire.
// It implements TurnPreviewManager.
type TurnPreviewRegistry struct {
	mu      sync.Mutex
	entries map[string]*turnPreviewRegistryEntry
}

type turnPreviewRegistryEntry struct {
	cancel context.CancelFunc
	runID  string
}

// NewTurnPreviewRegistry creates a new preview registry.
func NewTurnPreviewRegistry() *TurnPreviewRegistry {
	return &TurnPreviewRegistry{entries: make(map[string]*turnPreviewRegistryEntry)}
}

// Register implements TurnPreviewManager.
// It stores a preview cancel function for the session, cancelling any previous one.
func (r *TurnPreviewRegistry) Register(sessionID string, cancel context.CancelFunc) context.CancelFunc {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return cancel
	}
	if r == nil {
		return cancel
	}
	r.mu.Lock()
	if prev, ok := r.entries[sessionID]; ok && prev != nil && prev.cancel != nil {
		prev.cancel()
	}
	r.entries[sessionID] = &turnPreviewRegistryEntry{cancel: cancel}
	r.mu.Unlock()
	return func() {
		if cancel != nil {
			cancel()
		}
		r.Unregister(sessionID)
	}
}

// Unregister implements TurnPreviewManager.
func (r *TurnPreviewRegistry) Unregister(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, sessionID)
}

// SetRunID implements TurnPreviewManager.
func (r *TurnPreviewRegistry) SetRunID(sessionID, runID string) {
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	if sessionID == "" || runID == "" || r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry, ok := r.entries[sessionID]; ok && entry != nil {
		entry.runID = runID
	}
}

// ActiveRunID implements TurnPreviewManager.
func (r *TurnPreviewRegistry) ActiveRunID(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry, ok := r.entries[sessionID]; ok && entry != nil {
		return entry.runID
	}
	return ""
}
