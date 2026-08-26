package decision

import (
	"context"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// Outbox worker tuning (design §3.1): flush at >=batchSize buffered records or
// every flushInterval, retry a failed flush up to maxFlushAttempts, then fall
// back to the persisted retry queue (decision_record_outbox) which the worker
// replays every replayInterval and once at startup.
const (
	defaultChannelCapacity = 256
	defaultBatchSize       = 50
	defaultFlushInterval   = 500 * time.Millisecond
	defaultReplayInterval  = 30 * time.Second
	maxFlushAttempts       = 3
	outboxReplayBatch      = 100
)

// outboxCollector is the production Collector: non-blocking channel intake +
// single background flush worker. One instance per process (wire singleton).
type outboxCollector struct {
	repo Repo
	lg   loggateway.Logger

	ch chan Record

	batchSize      int
	flushInterval  time.Duration
	replayInterval time.Duration

	startOnce sync.Once
	stop      chan struct{}
	stopped   sync.WaitGroup
}

// Option overrides worker tunables (tests).
type Option func(*outboxCollector)

// WithBatchSize overrides the flush batch threshold.
func WithBatchSize(n int) Option {
	return func(c *outboxCollector) {
		if n > 0 {
			c.batchSize = n
		}
	}
}

// WithFlushInterval overrides the periodic flush interval.
func WithFlushInterval(d time.Duration) Option {
	return func(c *outboxCollector) {
		if d > 0 {
			c.flushInterval = d
		}
	}
}

// WithReplayInterval overrides the persisted-queue replay interval.
func WithReplayInterval(d time.Duration) Option {
	return func(c *outboxCollector) {
		if d > 0 {
			c.replayInterval = d
		}
	}
}

// WithChannelCapacity overrides the intake channel capacity.
func WithChannelCapacity(n int) Option {
	return func(c *outboxCollector) {
		if n > 0 {
			c.ch = make(chan Record, n)
		}
	}
}

// NewOutboxCollector builds the production collector. repo may be nil (CLI /
// tests): Emit still validates and accepts, worker flushes become no-ops.
// Call Start to launch the worker; Stop flushes and joins it.
func NewOutboxCollector(repo Repo, lg loggateway.Logger, opts ...Option) Lifecycle {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	c := &outboxCollector{
		repo:           repo,
		lg:             lg.With(loggateway.Domain("decision")),
		ch:             make(chan Record, defaultChannelCapacity),
		batchSize:      defaultBatchSize,
		flushInterval:  defaultFlushInterval,
		replayInterval: defaultReplayInterval,
		stop:           make(chan struct{}),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Emit validates and normalizes the record, then offers it to the intake
// channel without blocking. Invalid records are dropped with a warn (adapter
// bug); a full channel drops with a warn (NFR-80-01 backpressure policy).
func (c *outboxCollector) Emit(_ context.Context, rec Record) {
	if c == nil {
		return
	}
	rec.Normalize()
	// Timestamps are set at intake (not by adapters) so out-of-order flush
	// never rewrites when the decision actually happened.
	if rec.CreatedAt == "" {
		rec.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if rec.UpdatedAt == "" {
		rec.UpdatedAt = rec.CreatedAt
	}
	if err := rec.Validate(); err != nil {
		c.lg.Warn("decision record dropped: invalid",
			loggateway.StepID("decision.collector.validate"),
			loggateway.Err(err))
		return
	}
	select {
	case c.ch <- rec:
	default:
		c.lg.Warn("decision record dropped: intake channel full",
			loggateway.StepID("decision.collector.overflow"),
			loggateway.Str("category", string(rec.Category)),
			loggateway.Str("decision_key", rec.DecisionKey))
	}
}

// Start launches the flush worker exactly once.
func (c *outboxCollector) Start(ctx context.Context) {
	if c == nil {
		return
	}
	c.startOnce.Do(func() {
		c.stopped.Add(1)
		safego.Go(ctx, "decision-outbox-worker", func() {
			defer c.stopped.Done()
			c.run(ctx)
		})
		c.lg.Info("decision collector started",
			loggateway.StepID("decision.collector.start"))
	})
}

// Stop signals the worker, then waits for the final flush to finish.
func (c *outboxCollector) Stop() {
	if c == nil {
		return
	}
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
	c.stopped.Wait()
}

func (c *outboxCollector) run(ctx context.Context) {
	// Recover persisted retry-queue leftovers first (crash window between
	// flush-failure enqueue and process exit).
	c.replayPending(ctx)

	batch := make([]Record, 0, c.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		c.flushWithRetry(ctx, batch)
		batch = batch[:0]
	}
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()
	replayTicker := time.NewTicker(c.replayInterval)
	defer replayTicker.Stop()

	for {
		select {
		case rec := <-c.ch:
			batch = append(batch, rec)
			if len(batch) >= c.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-replayTicker.C:
			c.replayPending(ctx)
		case <-c.stop:
			// Drain what is still buffered, then exit.
			for {
				select {
				case rec := <-c.ch:
					batch = append(batch, rec)
				default:
					flush()
					return
				}
			}
		case <-ctx.Done():
			flush()
			return
		}
	}
}

// flushWithRetry writes the batch to decision_records; on repeated failure it
// persists the batch into the retry queue so no decision is silently lost.
func (c *outboxCollector) flushWithRetry(ctx context.Context, batch []Record) {
	if c.repo == nil {
		return
	}
	recs := make([]Record, len(batch))
	copy(recs, batch)
	var err error
	for attempt := 1; attempt <= maxFlushAttempts; attempt++ {
		if err = c.repo.InsertRecords(ctx, recs); err == nil {
			return
		}
		// Context cancellation is terminal, not retryable.
		if ctx.Err() != nil {
			break
		}
		time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
	}
	c.lg.Error("decision flush failed; persisting to retry queue",
		loggateway.StepID("decision.collector.flush_fail"),
		loggateway.Int("batch", len(recs)),
		loggateway.Err(err))
	if qErr := c.repo.EnqueueOutbox(ctx, recs); qErr != nil {
		// Dead-letter: both paths failed. Records are lost to logs only.
		c.lg.Error("decision retry-queue enqueue failed (dead letter)",
			loggateway.StepID("decision.collector.dead_letter"),
			loggateway.Int("batch", len(recs)),
			loggateway.Err(qErr))
	}
}

// replayPending drains the persisted retry queue oldest-first.
func (c *outboxCollector) replayPending(ctx context.Context) {
	if c.repo == nil {
		return
	}
	rows, err := c.repo.ListPendingOutbox(ctx, outboxReplayBatch)
	if err != nil {
		c.lg.Warn("decision retry-queue scan failed",
			loggateway.StepID("decision.collector.replay_scan"),
			loggateway.Err(err))
		return
	}
	if len(rows) == 0 {
		return
	}
	published := make([]int64, 0, len(rows))
	for _, row := range rows {
		rec, decErr := decodeRecord(row.Payload)
		if decErr != nil {
			// Poison row: payload undecodable — mark published to stop the
			// poison from blocking the queue forever; payload stays in the
			// row for forensics.
			c.lg.Error("decision retry-queue poison row",
				loggateway.StepID("decision.collector.poison"),
				loggateway.Int64("outbox_id", row.ID),
				loggateway.Err(decErr))
			published = append(published, row.ID)
			continue
		}
		if err := c.repo.InsertRecords(ctx, []Record{rec}); err != nil {
			if mErr := c.repo.MarkOutboxAttempt(ctx, row.ID, err.Error()); mErr != nil {
				c.lg.Warn("decision retry-queue attempt mark failed",
					loggateway.StepID("decision.collector.replay_mark"),
					loggateway.Err(mErr))
			}
			continue
		}
		published = append(published, row.ID)
	}
	if len(published) > 0 {
		if err := c.repo.MarkOutboxPublished(ctx, published, time.Now().UTC()); err != nil {
			c.lg.Warn("decision retry-queue publish mark failed",
				loggateway.StepID("decision.collector.replay_publish"),
				loggateway.Err(err))
		}
	}
	c.lg.Info("decision retry-queue replay done",
		loggateway.StepID("decision.collector.replay"),
		loggateway.Int("scanned", len(rows)),
		loggateway.Int("published", len(published)))
}
