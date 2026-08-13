package v2

// deadletter_replay.go — P1-R2b: durable dead-letter save + replay worker.
//
// 职责边界：
//   - pushDeadLetter：内存环形缓冲 + Prometheus 指标 + （可选）落库
//   - deadLetterReplayLoop：启动时 sweep 一次 + 每 deadLetterReplayInterval 周期 sweep
//   - replayDeadLettersOnce：单批次重放——成功标记 replayed，失败累加 attempts，
//     到达上限或 payload 无法解码（永久毒化）时 abandon
//
// 实体路由（kind/op/id/payload 解码）统一走 event_router.go 的 persist
// descriptor，live 与 replay 路径共用同一份映射真相源。

import (
	"context"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"
)

// P1-R2b dead-letter replay: startup + periodic sweep of the durable
// dead-letter store. Batch and attempt cap follow the memory_job
// dead-letter replayer conventions.
const (
	deadLetterReplayInterval   = 5 * time.Minute
	deadLetterReplayBatchSize  = 50
	deadLetterMaxReplayAttempt = 3
)

// WithDeadLetterStore injects the durable dead-letter store (P1-R2b). When
// set, dead-lettered events are persisted to the event_dead_letter table and
// a replay worker (startup + 5min sweep) re-attempts their entity upserts.
func WithDeadLetterStore(repo biz.EventDeadLetterRepo) Option {
	return func(c *config) { c.deadLetterStore = repo }
}

// WithDeadLetterReplayLoopDisabled prevents the background replay worker from
// starting. Test-only: production sequencers must keep the worker alive.
func WithDeadLetterReplayLoopDisabled() Option {
	return func(c *config) { c.disableReplayLoop = true }
}

// pushDeadLetter appends the event to the dead-letter ring and updates the
// exported Prometheus metrics (P0-R2a): total counter by event kind + current
// ring occupancy gauge. Centralised here so both drop paths (queue-full in
// processTask, retry-exhaustion in persistWithRetry) report consistently.
//
// P1-R2b: when a durable store is configured, the event's entity payload is
// also upserted into the event_dead_letter table (best-effort, bounded 2s)
// so the loss survives process restart and can be replayed.
//
// Receiver is the persist sub-manager (P1-1 split); the two drop paths call
// in from Sequencer.processTask / persistWorker.persistWithRetry.
func (p *persistWorker) pushDeadLetter(e biz.Event) {
	p.ring.Push(e)
	metrics.SequencerDeadLetterTotal.WithLabelValues(string(e.EventKind())).Inc()
	metrics.SequencerDeadLetterSize.Set(float64(p.ring.Len()))
	if p.store == nil {
		return
	}
	d := describePersist(e)
	if d == nil {
		return
	}
	payload, err := json.Marshal(d.entity)
	if err != nil {
		p.lg.Warn("dead-letter payload marshal failed",
			loggateway.Str("kind", string(e.EventKind())), loggateway.Err(err))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rec := biz.EventDeadLetter{
		EventKind:   string(e.EventKind()),
		EntityKind:  d.entityKind,
		EntityOp:    d.op,
		EntityID:    deadLetterID(e),
		SessionID:   e.SpiritSessionID(),
		PayloadJSON: string(payload),
		State:       biz.EventDeadLetterStatePending,
	}
	if err := p.store.SaveEventDeadLetter(ctx, rec); err != nil {
		// Best-effort: the in-memory ring + metrics still record the loss.
		// R3: throttled — fires per dead-lettered event while the store is
		// down, stacking on top of the drop-site log.
		if ok, suppressed := p.throttles.deadLetterSaveWarn.Allow(); ok {
			p.lg.Warn("durable dead-letter save failed",
				stallFields(suppressed,
					loggateway.Str("kind", string(e.EventKind())), loggateway.Err(err))...)
		}
	}
}

// deadLetterReplayLoop replays durable dead-letters once at startup, then
// every deadLetterReplayInterval, until close signals replayDone.
func (p *persistWorker) deadLetterReplayLoop() {
	defer p.replayWG.Done()
	p.replayDeadLettersOnce()
	ticker := time.NewTicker(deadLetterReplayInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.replayDone:
			return
		case <-ticker.C:
			p.replayDeadLettersOnce()
		}
	}
}

// replayDeadLettersOnce sweeps one batch of pending dead-letters: re-apply
// the entity upsert via the shared persist router. Success marks the row
// replayed; failure increments attempts; rows at the attempt cap (or with
// undecodable payloads — permanently poisoned) are abandoned.
func (p *persistWorker) replayDeadLettersOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	recs, err := p.store.ListPendingEventDeadLetters(ctx, deadLetterReplayBatchSize)
	if err != nil {
		p.lg.Warn("dead-letter replay: list pending failed", loggateway.Err(err))
		return
	}
	if len(recs) == 0 {
		return
	}
	var replayed, failed, abandoned int
	for _, rec := range recs {
		if rec.Attempts >= deadLetterMaxReplayAttempt {
			p.abandonDeadLetter(ctx, rec, "max_attempts_exceeded")
			abandoned++
			continue
		}
		entity, decErr := decodePersistEntity(rec.EntityKind, []byte(rec.PayloadJSON))
		if decErr != nil {
			// Payload can never succeed — no point retrying.
			p.abandonDeadLetter(ctx, rec, "decode_failed: "+decErr.Error())
			abandoned++
			continue
		}
		pctx, pcancel := context.WithTimeout(context.Background(), 2*time.Second)
		applyErr := applyPersist(pctx, p.repoSet, rec.EntityKind, rec.EntityOp, entity)
		pcancel()
		if applyErr == nil {
			if markErr := p.store.MarkEventDeadLetterReplayed(ctx, rec.ID); markErr != nil {
				p.lg.Warn("dead-letter replay: mark replayed failed",
					loggateway.Str("entity_kind", rec.EntityKind), loggateway.Err(markErr))
			}
			metrics.SequencerDeadLetterReplayTotal.WithLabelValues("replayed").Inc()
			replayed++
			continue
		}
		if incErr := p.store.IncrementEventDeadLetterAttempt(ctx, rec.ID, applyErr.Error()); incErr != nil {
			p.lg.Warn("dead-letter replay: increment attempt failed",
				loggateway.Str("entity_kind", rec.EntityKind), loggateway.Err(incErr))
		}
		metrics.SequencerDeadLetterReplayTotal.WithLabelValues("failed").Inc()
		failed++
	}
	p.lg.Info("dead-letter replay summary",
		loggateway.Int("replayed", replayed),
		loggateway.Int("failed", failed),
		loggateway.Int("abandoned", abandoned),
		loggateway.Int("total", len(recs)))
}

func (p *persistWorker) abandonDeadLetter(ctx context.Context, rec biz.EventDeadLetter, reason string) {
	if err := p.store.MarkEventDeadLetterAbandoned(ctx, rec.ID, reason); err != nil {
		p.lg.Warn("dead-letter replay: mark abandoned failed",
			loggateway.Str("entity_kind", rec.EntityKind), loggateway.Err(err))
		return
	}
	metrics.SequencerDeadLetterReplayTotal.WithLabelValues("abandoned").Inc()
	p.lg.Warn("dead-letter abandoned",
		loggateway.Str("kind", rec.EventKind),
		loggateway.Str("entity_kind", rec.EntityKind),
		loggateway.Str("reason", reason))
}
