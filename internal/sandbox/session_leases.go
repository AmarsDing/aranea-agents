package sandbox

import (
	"context"
	"sync"
	"time"
)

// SessionLeases pins one sandbox lease per execution session (P1-1, FR-11).
// It is the process-wide shared store consumed by both the codeexecutor
// pooled backend (ExecuteCode) and the sandbox_fs tool family
// (sandbox_fs_write/read): keying both on the same session id means code and
// file operations land in ONE sandbox per session — filesystem and installed
// packages persist across turns and across tool families (US-2/A3).
//
// Eviction is lazy + backstopped, no extra goroutine:
//   - manager-side GC destroys leased sandboxes past idle_timeout/TTL (the
//     authoritative cleanup; quota is released there);
//   - the next use after such a destroy gets ErrNotFound, the stale entry is
//     evicted, and the caller retries once on a fresh lease.
type SessionLeases struct {
	mu     sync.Mutex
	mgr    *Manager
	leases map[string]*Lease
}

// NewSessionLeases builds the shared store. mgr must outlive the store
// (wire process singleton).
func NewSessionLeases(mgr *Manager) *SessionLeases {
	return &SessionLeases{mgr: mgr, leases: map[string]*Lease{}}
}

// Acquire returns the cached lease for key, acquiring a fresh session-scoped
// one on first use. TTL 0 → manager default; renewed on every successful use.
func (s *SessionLeases) Acquire(ctx context.Context, key string) (*Lease, error) {
	s.mu.Lock()
	if l, ok := s.leases[key]; ok {
		s.mu.Unlock()
		return l, nil
	}
	s.mu.Unlock()

	lease, err := s.mgr.Acquire(ctx, AcquireReq{
		Profile:   DefaultProfileName,
		SessionID: key,
		// P2-2: first-creation is attributed to the owning team run's
		// cumulative budget when the caller carries a run id (team turn ctx).
		RunID: RunIDFromContext(ctx),
	})
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	// Race: another goroutine may have acquired for the same key meanwhile.
	if prev, ok := s.leases[key]; ok {
		s.mu.Unlock()
		_ = lease.Release(context.Background())
		return prev, nil
	}
	s.leases[key] = lease
	s.mu.Unlock()
	return lease, nil
}

// Renew slides the lease deadline forward after successful use (capped at
// the manager's max TTL).
func (s *SessionLeases) Renew(ctx context.Context, key string, extend time.Duration) {
	s.mu.Lock()
	l, ok := s.leases[key]
	s.mu.Unlock()
	if ok {
		_ = l.Renew(ctx, extend)
	}
}

// Evict drops key's cached lease only if it is still the same instance (the
// manager already destroyed it — no Release call here).
func (s *SessionLeases) Evict(key string, l *Lease) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.leases[key]; ok && cur == l {
		delete(s.leases, key)
	}
}

// ReleaseSession destroys the lease pinned to key, if any (explicit session
// teardown hook; the idle/TTL GC remains the backstop).
func (s *SessionLeases) ReleaseSession(ctx context.Context, key string) {
	s.mu.Lock()
	l, ok := s.leases[key]
	if ok {
		delete(s.leases, key)
	}
	s.mu.Unlock()
	if ok {
		_ = l.Release(ctx)
	}
}
