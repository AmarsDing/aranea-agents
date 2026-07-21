package agent

import (
	"strings"
	"sync"
	"time"
)

// toolSessionGrantTTL bounds how long a session-scoped grant lives in
// memory. Grants are lazily evicted on access, so a long-running process
// does not accumulate entries for dead sessions. 24h comfortably covers an
// active chat session while keeping the "session grant" semantics
// (process restart always clears all session grants, matching Grok).
const toolSessionGrantTTL = 24 * time.Hour

type toolSessionGrantKey struct {
	sessionID string
	agentID   string
	toolKey   string
}

type toolSessionGrantEntry struct {
	grantedAt time.Time
}

// toolGrantStore holds session-scoped tool grants in memory, keyed by
// (sessionID, agentID, toolKey) so a grant never leaks across sessions.
// Persisted (cross-session) grants live in the DB and are queried through
// the biz layer; this store only covers the in-memory session tier.
type toolGrantStore struct {
	mu            sync.Mutex
	sessionGrants map[toolSessionGrantKey]toolSessionGrantEntry
	now           func() time.Time
}

func newToolGrantStore(now func() time.Time) *toolGrantStore {
	if now == nil {
		now = time.Now
	}
	return &toolGrantStore{
		sessionGrants: make(map[toolSessionGrantKey]toolSessionGrantEntry),
		now:           now,
	}
}

// GrantSession records a session-scoped grant. Empty keys are ignored so a
// grant can never be created that would match an unidentified context.
func (s *toolGrantStore) GrantSession(sessionID, agentID, toolKey string) {
	key, ok := makeToolSessionGrantKey(sessionID, agentID, toolKey)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionGrants[key] = toolSessionGrantEntry{grantedAt: s.now()}
}

// HasSession reports whether a live (non-expired) session grant exists.
// Expired entries are lazily evicted.
func (s *toolGrantStore) HasSession(sessionID, agentID, toolKey string) bool {
	key, ok := makeToolSessionGrantKey(sessionID, agentID, toolKey)
	if !ok {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.sessionGrants[key]
	if !ok {
		return false
	}
	if s.now().Sub(entry.grantedAt) > toolSessionGrantTTL {
		delete(s.sessionGrants, key)
		return false
	}
	return true
}

func makeToolSessionGrantKey(sessionID, agentID, toolKey string) (toolSessionGrantKey, bool) {
	key := toolSessionGrantKey{
		sessionID: strings.TrimSpace(sessionID),
		agentID:   strings.TrimSpace(agentID),
		toolKey:   strings.TrimSpace(toolKey),
	}
	if key.sessionID == "" || key.agentID == "" || key.toolKey == "" {
		return toolSessionGrantKey{}, false
	}
	return key, true
}
