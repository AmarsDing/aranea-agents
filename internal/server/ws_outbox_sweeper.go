package server

import (
	"context"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// P1-3: compensation sweep for durable critical-delivery rows whose live
// publish path never completed — Insert succeeded, then the publish worker
// crashed mid-flight, the BlockUpTo fan-out dropped the frame, or
// MarkPublished failed. Without this worker such rows stayed invisible until
// the client's next reconnect replay, and published_at stayed NULL forever.
const (
	defaultOutboxSweepInterval = 10 * time.Second
	// defaultOutboxSweepGrace is the age after which an unpublished row is
	// considered stuck. The live publish path (BlockUpTo 2s + MarkPublished)
	// completes in ~2s, so 10s sits far outside the normal envelope and will
	// not race an in-flight publish.
	defaultOutboxSweepGrace = 10 * time.Second
	defaultOutboxSweepLimit = 200
	// outboxSweepQueryTimeout budgets ONLY ListPending + the broadcast loop.
	outboxSweepQueryTimeout = 5 * time.Second
	// outboxSweepMarkTimeout gives the end-of-batch MarkPublishedBatch call
	// its own short budget, decoupled from the query/broadcast window:
	// marking must not starve when broadcasting burned the query budget
	// (finding #6/#24 — otherwise tail rows stay pending and re-broadcast
	// every tick, a self-sustaining duplicate-frame storm).
	outboxSweepMarkTimeout = 2 * time.Second
	// outboxSweepWarnLogInterval throttles per-tick failure Warns (same
	// window as the sequencer stall-throttle chain, stallWarnLogInterval).
	outboxSweepWarnLogInterval = 10 * time.Second
)

// outboxSweepRuntime bundles the durable critical-event outbox (B-06
// last_event_id replay) and the one-time start guard of the P1-3
// compensation sweeper. Keeping both in one sub-struct stops WSServer from
// growing a loose field per optional worker (AS-COG-01).
type outboxSweepRuntime struct {
	repo biz.EventDeliveryOutboxRepo
	// started guards one-time sweeper startup from Start, which kratos may
	// invoke more than once.
	started atomic.Bool
}

// sweepWarnFields appends the suppressed-failure count when a throttle
// window reset since the previous emission (mirrors the sequencer's
// stallFields), so throttled-away failures stay visible as a count.
func sweepWarnFields(suppressed int, fields ...loggateway.Field) []loggateway.Field {
	if suppressed > 0 {
		fields = append(fields, loggateway.Int("suppressed", suppressed))
	}
	return fields
}

// eventOutboxSweeper periodically re-broadcasts pending outbox rows on the
// high-priority lane and marks them published.
//
// Delivery contract stays at-least-once (B-06): a crash between the broadcast
// and MarkPublished produces one duplicate frame on the next sweep; clients
// dedupe via DeliverySeqValue / event_id (frontend activityV2Store Version
// guard), matching the live path's semantics exactly.
type eventOutboxSweeper struct {
	outbox    biz.EventDeliveryOutboxRepo
	broadcast func(sessionID string, msg []byte)
	lg        loggateway.Logger
	interval  time.Duration
	grace     time.Duration
	limit     int
	// queryTimeout budgets ListPending + broadcasts per tick; markTimeout
	// gives the batch mark its own budget. Both are fields (not the package
	// consts directly) so tests can shrink them deterministically.
	queryTimeout time.Duration
	markTimeout  time.Duration
	// markWarn / skipWarn throttle per-tick failure Warns; behaviour (batch
	// marking, skipping corrupt rows) is never throttled — only logs.
	markWarn *loggateway.Throttle
	skipWarn *loggateway.Throttle
}

func newEventOutboxSweeper(outbox biz.EventDeliveryOutboxRepo, broadcast func(string, []byte), lg loggateway.Logger) *eventOutboxSweeper {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &eventOutboxSweeper{
		outbox:       outbox,
		broadcast:    broadcast,
		lg:           lg.With(loggateway.Domain("ws_outbox_sweeper")),
		interval:     defaultOutboxSweepInterval,
		grace:        defaultOutboxSweepGrace,
		limit:        defaultOutboxSweepLimit,
		queryTimeout: outboxSweepQueryTimeout,
		markTimeout:  outboxSweepMarkTimeout,
		markWarn:     loggateway.NewThrottle(outboxSweepWarnLogInterval),
		skipWarn:     loggateway.NewThrottle(outboxSweepWarnLogInterval),
	}
}

// Start launches the sweep loop bound to ctx (kratos hands WSServer.Start the
// app-lifetime context, so the loop exits on shutdown). No-op when deps are
// missing — e.g. WS enabled without an outbox repo in tests.
func (s *eventOutboxSweeper) Start(ctx context.Context) {
	if s == nil || s.outbox == nil || s.broadcast == nil || ctx == nil {
		return
	}
	safego.Go(ctx, "worker.ws_outbox_sweeper", func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.sweepOnce(ctx)
			}
		}
	})
}

// sweepOnce re-broadcasts one pending batch. A session with no connected
// clients accepts no frames (broadcast is a no-op) — the row is still marked
// published because reconnect replay (ListAfter, published_at-agnostic) covers
// that delivery; published_at only records "live delivery attempted".
//
// Budget discipline (findings #6/#24/#30): the query timeout covers
// ListPending + the broadcast loop ONLY. The loop breaks as soon as that
// budget is exhausted — tail rows stay pending for the next tick instead of
// being broadcast-then-never-marked (the failure mode that made stuck rows
// re-broadcast every 10s forever). Every row that WAS broadcast (plus
// undeliverable rows) is then marked with a single batch UPDATE on its own
// timeout, so marking never competes with broadcasts for budget.
func (s *eventOutboxSweeper) sweepOnce(ctx context.Context) {
	started := time.Now()
	cctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	cutoff := started.UTC().Add(-s.grace)
	rows, err := s.outbox.ListPending(cctx, cutoff, s.limit)
	if err != nil {
		s.lg.Warn("WS outbox sweep query failed",
			loggateway.StepID("ws.outbox_sweeper"),
			loggateway.Err(err))
		return
	}
	if len(rows) == 0 {
		return
	}
	broadcasts := 0
	markIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if cctx.Err() != nil {
			// Budget exhausted mid-batch: stop here. The remaining rows stay
			// pending and are re-broadcast next tick (at-least-once; clients
			// dedupe by event_id / delivery seq). Previously the loop checked
			// the parent ctx and kept broadcasting rows it then could not
			// mark — a self-sustaining duplicate-frame storm (finding #6/#24).
			break
		}
		if len(row.Payload) > 0 && row.SessionID != "" {
			s.broadcast(row.SessionID, row.Payload)
			broadcasts++
			markIDs = append(markIDs, row.ID)
			continue
		}
		// Corrupt/undeliverable row — mark it anyway so it stops blocking
		// the sweep window; replay would skip it identically.
		if ok, suppressed := s.skipWarn.Allow(); ok {
			s.lg.Warn("WS outbox sweep skipping undeliverable row",
				sweepWarnFields(suppressed,
					loggateway.StepID("ws.outbox_sweeper"),
					loggateway.Str("event_id", row.EventID))...)
		}
		markIDs = append(markIDs, row.ID)
	}
	markFailed := 0
	if len(markIDs) > 0 {
		mctx, mcancel := context.WithTimeout(ctx, s.markTimeout)
		merr := s.outbox.MarkPublishedBatch(mctx, markIDs, time.Now().UTC())
		mcancel()
		if merr != nil {
			// Leave every row pending: the next tick re-broadcasts them
			// (duplicates tolerated — clients dedupe by event_id / seq).
			markFailed = len(markIDs)
			if ok, suppressed := s.markWarn.Allow(); ok {
				s.lg.Warn("WS outbox sweep mark-published batch failed",
					sweepWarnFields(suppressed,
						loggateway.StepID("ws.outbox_sweeper"),
						loggateway.Int("rows", markFailed),
						loggateway.Err(merr))...)
			}
		}
	}
	s.lg.Debug("WS outbox sweep completed",
		loggateway.StepID("ws.outbox_sweeper"),
		loggateway.Int("rows", len(rows)),
		loggateway.Int("broadcasts", broadcasts),
		loggateway.Int("marked", len(markIDs)-markFailed),
		loggateway.Int("mark_failed", markFailed),
		loggateway.Duration(time.Since(started).Milliseconds()))
}
