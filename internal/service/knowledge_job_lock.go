package service

import (
	"context"
	"database/sql"
	"sync"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// KnowledgeJobLocker is a cross-process exclusive lease for rebuild/reembed.
// Stability: evolving
type KnowledgeJobLocker interface {
	// TryAcquire attempts to hold jobKey. ok=false means another holder.
	// err is set when the lock backend cannot be reached (fail-closed).
	TryAcquire(ctx context.Context, jobKey string) (release func(), ok bool, err error)
}

// NewMemKnowledgeJobLocker returns an in-process locker for tests and when
// Postgres is not configured.
func NewMemKnowledgeJobLocker() KnowledgeJobLocker {
	return &memKnowledgeJobLocker{held: map[string]struct{}{}}
}

type memKnowledgeJobLocker struct {
	mu   sync.Mutex
	held map[string]struct{}
}

func (m *memKnowledgeJobLocker) TryAcquire(_ context.Context, jobKey string) (func(), bool, error) {
	if m == nil {
		return func() {}, true, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.held[jobKey]; ok {
		return nil, false, nil
	}
	m.held[jobKey] = struct{}{}
	return func() {
		m.mu.Lock()
		delete(m.held, jobKey)
		m.mu.Unlock()
	}, true, nil
}

// NewPGKnowledgeJobLocker uses session-level pg_try_advisory_lock(hashtext(key)).
// The connection is held until release so unlock runs on the same session.
func NewPGKnowledgeJobLocker(db *sql.DB, lg loggateway.Logger) KnowledgeJobLocker {
	if db == nil {
		return NewMemKnowledgeJobLocker()
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &pgKnowledgeJobLocker{db: db, lg: lg}
}

type pgKnowledgeJobLocker struct {
	db *sql.DB
	lg loggateway.Logger
}

func (p *pgKnowledgeJobLocker) TryAcquire(ctx context.Context, jobKey string) (func(), bool, error) {
	if p == nil || p.db == nil {
		return func() {}, true, nil
	}
	conn, err := p.db.Conn(ctx)
	if err != nil {
		p.lg.Warn("knowledge job lock: acquire conn failed",
			loggateway.StepID("knowledge.job_lock"),
			loggateway.Str("job_key", jobKey),
			loggateway.Err(err))
		return nil, false, err
	}
	var acquired bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, jobKey).Scan(&acquired); err != nil {
		_ = conn.Close()
		p.lg.Warn("knowledge job lock: pg_try_advisory_lock failed",
			loggateway.StepID("knowledge.job_lock"),
			loggateway.Str("job_key", jobKey),
			loggateway.Err(err))
		return nil, false, err
	}
	if !acquired {
		_ = conn.Close()
		return nil, false, nil
	}
	return func() {
		unlockCtx := context.Background()
		if _, uerr := conn.ExecContext(unlockCtx, `SELECT pg_advisory_unlock(hashtext($1))`, jobKey); uerr != nil {
			p.lg.Warn("knowledge job lock: pg_advisory_unlock failed",
				loggateway.StepID("knowledge.job_lock"),
				loggateway.Str("job_key", jobKey),
				loggateway.Err(uerr))
		}
		_ = conn.Close()
	}, true, nil
}

func knowledgeRebuildJobKey(collectionID string) string {
	return "knowledge-rebuild:" + collectionID
}

func knowledgeReembedJobKey(docID string) string {
	return "knowledge-reembed:" + docID
}

func (s *KnowledgeService) acquireJob(ctx context.Context, runs *sync.Map, localKey, jobKey, conflictFmt, conflictArg string) (func(), error) {
	var distRelease func()
	if s != nil && s.jobLock != nil {
		rel, ok, err := s.jobLock.TryAcquire(ctx, jobKey)
		if err != nil {
			return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainKnowledge)
		}
		if !ok {
			return nil, apierror.Conflict(apierror.DomainKnowledge, conflictFmt, conflictArg)
		}
		distRelease = rel
	}
	if s != nil {
		if _, loaded := runs.LoadOrStore(localKey, struct{}{}); loaded {
			if distRelease != nil {
				distRelease()
			}
			return nil, apierror.Conflict(apierror.DomainKnowledge, conflictFmt, conflictArg)
		}
	}
	return func() {
		if s != nil {
			runs.Delete(localKey)
		}
		if distRelease != nil {
			distRelease()
		}
	}, nil
}
