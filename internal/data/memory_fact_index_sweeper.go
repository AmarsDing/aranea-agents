package data

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// Default MemoryFactIndexSweeper knobs (overridable via the constructor).
const (
	defaultFactIndexSweepInterval = 30 * time.Second
	defaultFactIndexSweepBatch    = 50
	defaultFactIndexMaxAttempts   = 8
	factIndexSweepPerRowTimeout   = 30 * time.Second
)

// MemoryFactIndexSweeper repairs facts whose async embedding-index write died
// after Add returned (scheduleEmbed is fire-and-forget: on failure
// memory_fact_index_sync.markFactIndexStale flips embedding_status to 'stale'
// and bumps index_attempts, but nothing ever retried, permanently dropping the
// fact from vector recall). Each interval it picks a batch of due rows
// (embedding_status IN ('stale','pending','failed'), attempts below the cap,
// exponential backoff of 2^index_attempts minutes capped at 60 elapsed since
// updated_at) and replays them through the same SyncFactIndexFromRow path the
// fire-and-forget writer uses — Sync itself re-marks stale + increments
// index_attempts on failure, so rows converge to 'fresh' or age out of the
// sweep after maxAttempts. Concurrency is 1 (serial batches) to protect the
// embedding provider quota. All queries force status='active' and
// deleted_at='' so it never resurrects invalidated facts.
type MemoryFactIndexSweeper struct {
	data        *Data
	syncer      biz.MemoryFactIndexSyncer
	lg          loggateway.Logger
	interval    time.Duration
	batch       int
	maxAttempts int
}

// NewMemoryFactIndexSweeper wires the sweeper; non-positive interval/batch/
// maxAttempts fall back to the defaults above.
func NewMemoryFactIndexSweeper(d *Data, syncer biz.MemoryFactIndexSyncer, lg loggateway.Logger, interval time.Duration, batch, maxAttempts int) *MemoryFactIndexSweeper {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	if interval <= 0 {
		interval = defaultFactIndexSweepInterval
	}
	if batch <= 0 {
		batch = defaultFactIndexSweepBatch
	}
	if maxAttempts <= 0 {
		maxAttempts = defaultFactIndexMaxAttempts
	}
	return &MemoryFactIndexSweeper{
		data:        d,
		syncer:      syncer,
		lg:          lg.With(loggateway.Domain("memory_index_sweeper")),
		interval:    interval,
		batch:       batch,
		maxAttempts: maxAttempts,
	}
}

// Run sweeps once immediately, then every interval, until ctx is cancelled.
// Cancellation is honored promptly (in-flight row sync via ctx propagation,
// between rows via an explicit check) and always returns nil.
func (s *MemoryFactIndexSweeper) Run(ctx context.Context) error {
	if s == nil || s.data == nil || s.syncer == nil {
		return nil
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	s.sweepOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if ctx.Err() != nil {
				return nil
			}
			s.sweepOnce(ctx)
		}
	}
}

// sweepOnce processes one due batch; returns how many rows synced cleanly.
// Exported-test seam: package-internal so tests drive one deterministic pass
// instead of polling on the ticker.
func (s *MemoryFactIndexSweeper) sweepOnce(ctx context.Context) int {
	rows, err := s.pickDueStaleFactRows(ctx)
	if err != nil {
		s.lg.Warn("fact index sweep query failed",
			loggateway.StepID("memory.index_sweep_query_fail"), loggateway.Err(err))
		return 0
	}
	synced := 0
	for _, raw := range rows {
		if ctx.Err() != nil {
			return synced
		}
		rowCtx, cancel := context.WithTimeout(ctx, factIndexSweepPerRowTimeout)
		serr := s.syncer.SyncFactIndexFromRow(rowCtx, raw)
		cancel()
		if serr != nil {
			// Sync already marked the row stale and incremented index_attempts;
			// the backoff filter paces the next pickup. Warn-only by contract.
			s.lg.Warn("fact index sweep sync failed",
				loggateway.StepID("memory.index_sweep_sync_fail"),
				loggateway.Str("fact_id", factJSONString(raw, "id")), loggateway.Err(serr))
			continue
		}
		synced++
	}
	if len(rows) > 0 {
		s.lg.Info("fact index sweep batch done", loggateway.StepID("memory.index_sweep"),
			loggateway.Int("picked", len(rows)), loggateway.Int("synced", synced))
	}
	return synced
}

// pickDueStaleFactRows selects due rows, projecting the full fact columns so
// scanFactRowJSON yields raw JSON isomorphic to l3FactRepo.UpsertFactRow's
// return shape (id/agent_id/scope_id/user_id/statement are what
// SyncFactIndexFromRow consumes). The backoff due check is
// COALESCE(NULLIF(updated_at,'')) + 2^index_attempts minutes (capped at 60)
// <= now(); rows with a corrupt/empty updated_at are due immediately.
func (s *MemoryFactIndexSweeper) pickDueStaleFactRows(ctx context.Context) ([][]byte, error) {
	db := s.data.RWDB()
	if db == nil {
		return nil, nil
	}
	// LEAST(POWER(2.0, index_attempts), 60.0): exponential backoff in minutes,
	// capped at 60. PG-only cast syntax is safe: Postgres is the sole dialect.
	rows, err := db.ReadDB(ctx).QueryContext(ctx,
		s.data.Dialect().RenumberPlaceholders(sqlFactSelect+` WHERE embedding_status IN ('stale','pending','failed')
 AND status = 'active' AND deleted_at = '' AND index_attempts < ?
 AND COALESCE(NULLIF(updated_at, '')::timestamptz, to_timestamp(0)) + interval '1 minute' * LEAST(POWER(2.0, index_attempts), 60.0) <= now()
 ORDER BY updated_at ASC LIMIT ?`),
		s.maxAttempts, s.batch)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		raw, serr := scanFactRowJSON(rows)
		if serr != nil {
			return nil, entErrToBizErr(serr, "MEMORY_L3")
		}
		out = append(out, raw)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L3")
}
