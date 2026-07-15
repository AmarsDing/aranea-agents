package cronrunner

import (
	"context"
	"database/sql"
	"sync"

	"aranea-agents/pkg/loggateway"
)

// taskLease provides exclusive execution rights for a cron task across processes.
// Stability:evolving
type taskLease interface {
	// TryAcquire attempts to hold the lease for taskID. On success, release must
	// be called when the critical section ends. ok=false means another holder.
	TryAcquire(ctx context.Context, taskID string) (release func(), ok bool)
}

// memTaskLease is an in-process lease used for tests and as a stand-in when
// no Postgres DB is configured (process mutex still serializes within a Runner).
type memTaskLease struct {
	mu   sync.Mutex
	held map[string]struct{}
}

func newMemTaskLease() *memTaskLease {
	return &memTaskLease{held: make(map[string]struct{})}
}

func (m *memTaskLease) TryAcquire(_ context.Context, taskID string) (func(), bool) {
	if m == nil {
		return func() {}, true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.held[taskID]; ok {
		return nil, false
	}
	m.held[taskID] = struct{}{}
	return func() {
		m.mu.Lock()
		delete(m.held, taskID)
		m.mu.Unlock()
	}, true
}

// pgAdvisoryTaskLease uses Postgres session-level advisory locks keyed by
// hashtext(task_id). The connection is held for the lease duration so unlock
// runs on the same session.
type pgAdvisoryTaskLease struct {
	db *sql.DB
	lg loggateway.Logger
}

func newPgAdvisoryTaskLease(db *sql.DB, lg loggateway.Logger) *pgAdvisoryTaskLease {
	return &pgAdvisoryTaskLease{db: db, lg: lg}
}

func (p *pgAdvisoryTaskLease) TryAcquire(ctx context.Context, taskID string) (func(), bool) {
	if p == nil || p.db == nil {
		return func() {}, true
	}
	conn, err := p.db.Conn(ctx)
	if err != nil {
		if p.lg != nil {
			p.lg.Warn("cron lease: acquire conn failed",
				loggateway.StepID("cron.lease"),
				loggateway.Str("task_id", taskID),
				loggateway.Err(err))
		}
		// Fail open to in-process mutex only — caller still holds lockTask.
		return func() {}, true
	}
	var acquired bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, taskID).Scan(&acquired); err != nil {
		_ = conn.Close()
		if p.lg != nil {
			p.lg.Warn("cron lease: pg_try_advisory_lock failed",
				loggateway.StepID("cron.lease"),
				loggateway.Str("task_id", taskID),
				loggateway.Err(err))
		}
		return func() {}, true
	}
	if !acquired {
		_ = conn.Close()
		return nil, false
	}
	return func() {
		unlockCtx := context.Background()
		if _, uerr := conn.ExecContext(unlockCtx, `SELECT pg_advisory_unlock(hashtext($1))`, taskID); uerr != nil && p.lg != nil {
			p.lg.Warn("cron lease: pg_advisory_unlock failed",
				loggateway.StepID("cron.lease"),
				loggateway.Str("task_id", taskID),
				loggateway.Err(uerr))
		}
		_ = conn.Close()
	}, true
}

// alwaysHeldLease always succeeds (backward compat when no cross-instance lease).
type alwaysHeldLease struct{}

func (alwaysHeldLease) TryAcquire(context.Context, string) (func(), bool) {
	return func() {}, true
}

func resolveTaskLease(db *sql.DB, lg loggateway.Logger) taskLease {
	if db == nil {
		return alwaysHeldLease{}
	}
	return newPgAdvisoryTaskLease(db, lg)
}
