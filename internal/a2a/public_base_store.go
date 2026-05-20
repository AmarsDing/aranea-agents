package a2a

import "sync"

// PublicBaseURLStore holds the effective public A2A base URL for hot reload.
type PublicBaseURLStore struct {
	mu     sync.RWMutex
	result PublicBaseURLResult
}

// NewPublicBaseURLStore constructs a store with the initial resolved URL.
func NewPublicBaseURLStore(initial PublicBaseURLResult) *PublicBaseURLStore {
	return &PublicBaseURLStore{result: initial}
}

// Get returns the current effective public base URL.
func (s *PublicBaseURLStore) Get() PublicBaseURLResult {
	if s == nil {
		return PublicBaseURLResult{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.result
}

// Set replaces the effective URL (called after system settings save).
func (s *PublicBaseURLStore) Set(result PublicBaseURLResult) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.result = result
}
