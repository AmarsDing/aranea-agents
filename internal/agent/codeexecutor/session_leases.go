package codeexecutor

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/sandbox"
)

// sessionLeaseStore pins one sandbox lease per execution session (P1-1, FR-11):
// the framework passes a session-scoped ExecutionID (app/user/session) with
// every ExecuteCode call, so all code runs in a session share one sandbox —
// filesystem and installed packages persist across turns (US-2/A3).
//
// Eviction is lazy + backstopped, no extra goroutine:
//   - manager-side GC destroys leased sandboxes past idle_timeout/TTL (the
//     authoritative cleanup; quota is released there);
//   - the next ExecuteCode after such a destroy gets ErrNotFound, the stale
//     entry is evicted, and the call retries once on a fresh lease.
type sessionLeaseStore struct {
	mu     sync.Mutex
	mgr    *sandbox.Manager
	leases map[string]*sandbox.Lease
}

func newSessionLeaseStore(mgr *sandbox.Manager) *sessionLeaseStore {
	return &sessionLeaseStore{mgr: mgr, leases: map[string]*sandbox.Lease{}}
}

// acquire returns the cached lease for key, acquiring a fresh session-scoped
// one on first use. TTL 0 → manager default; renewed on every successful use.
func (s *sessionLeaseStore) acquire(ctx context.Context, key string) (*sandbox.Lease, error) {
	s.mu.Lock()
	if l, ok := s.leases[key]; ok {
		s.mu.Unlock()
		return l, nil
	}
	s.mu.Unlock()

	lease, err := s.mgr.Acquire(ctx, sandbox.AcquireReq{
		Profile:   sandbox.DefaultProfileName,
		SessionID: key,
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

// renew slides the lease deadline forward after successful use (capped at
// the manager's max TTL).
func (s *sessionLeaseStore) renew(ctx context.Context, key string, extend time.Duration) {
	s.mu.Lock()
	l, ok := s.leases[key]
	s.mu.Unlock()
	if ok {
		_ = l.Renew(ctx, extend)
	}
}

// evict drops key's cached lease only if it is still the same instance (the
// manager already destroyed it — no Release call here).
func (s *sessionLeaseStore) evict(key string, l *sandbox.Lease) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.leases[key]; ok && cur == l {
		delete(s.leases, key)
	}
}

// releaseSession destroys the lease pinned to key, if any (explicit session
// teardown hook; the idle/TTL GC remains the backstop).
func (s *sessionLeaseStore) releaseSession(ctx context.Context, key string) {
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

// normalizeSessionKey trims the framework ExecutionID; empty means the caller
// has no session context (ephemeral one-shot execution).
func normalizeSessionKey(executionID string) string {
	return strings.TrimSpace(executionID)
}
