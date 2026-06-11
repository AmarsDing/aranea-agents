package service

import (
	"context"
	"strings"
	"sync"
)

// turnPreviewRegistry tracks one active preview coordinator per session (CH-BOR-07).
// Replacing an entry stops the previous subscription so concurrent turns do not cross-wire.
// It implements biz.TurnPreviewManager.
type turnPreviewRegistry struct {
	mu      sync.Mutex
	entries map[string]*turnPreviewRegistryEntry
}

type turnPreviewRegistryEntry struct {
	coord  *TurnPreviewCoordinator
	cancel context.CancelFunc
	runID  string
}

func newTurnPreviewRegistry() *turnPreviewRegistry {
	return &turnPreviewRegistry{entries: make(map[string]*turnPreviewRegistryEntry)}
}

// Register implements biz.TurnPreviewManager.
// It stores a preview cancel function for the session, cancelling any previous one.
func (r *turnPreviewRegistry) Register(sessionID string, cancel context.CancelFunc) context.CancelFunc {
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

// Unregister implements biz.TurnPreviewManager.
func (r *turnPreviewRegistry) Unregister(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, sessionID)
}

// SetRunID implements biz.TurnPreviewManager.
func (r *turnPreviewRegistry) SetRunID(sessionID, runID string) {
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	if sessionID == "" || runID == "" || r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry, ok := r.entries[sessionID]; ok && entry != nil {
		entry.runID = runID
		if entry.coord != nil {
			entry.coord.SetActiveRunID(runID)
		}
	}
}

// ActiveRunID implements biz.TurnPreviewManager.
func (r *turnPreviewRegistry) ActiveRunID(sessionID string) string {
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

// registerWithCoord stores a preview coordinator and cancel function for the session.
// This is the service-level method that also tracks the coordinator for SetActiveRunID.
func (r *turnPreviewRegistry) registerWithCoord(sessionID string, coord *TurnPreviewCoordinator, cancel context.CancelFunc) context.CancelFunc {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || coord == nil {
		return cancel
	}
	if r == nil {
		return cancel
	}
	r.mu.Lock()
	if prev, ok := r.entries[sessionID]; ok && prev != nil && prev.cancel != nil {
		prev.cancel()
	}
	r.entries[sessionID] = &turnPreviewRegistryEntry{coord: coord, cancel: cancel}
	r.mu.Unlock()
	return func() {
		if cancel != nil {
			cancel()
		}
		r.unregisterWithCoord(sessionID, coord)
	}
}

func (r *turnPreviewRegistry) unregisterWithCoord(sessionID string, coord *TurnPreviewCoordinator) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[sessionID]
	if !ok || entry == nil || entry.coord != coord {
		return
	}
	delete(r.entries, sessionID)
}
