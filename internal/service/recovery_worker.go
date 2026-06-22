package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// recoveryWorkerPollInterval is the default interval between recovery
// scans when running via Start. Chosen to be conservative so we don't
// hammer the DB while still recovering stale runs promptly.
const recoveryWorkerPollInterval = 5 * time.Minute

// recoveryWorkerBatchSize is the max number of stale runs fetched per scan.
const recoveryWorkerBatchSize = 32

// staleRunLister is the narrow interface RecoveryWorker needs from
// biz.SessionRunUsecase. Defined here to keep RecoveryWorker testable
// without pulling in the full SessionRunUsecase surface (ISP / red-line
// on narrow interfaces).
//
// Stability:internal
type staleRunLister interface {
	ListDurablePending(ctx context.Context, limit int) ([]biz.SessionRun, error)
	Fail(ctx context.Context, id, errMsg string) error
}

// RecoveryWorker scans for stale durable SessionRuns and tries to resume
// them from their persisted checkpoints (P1-8 crash recovery).
//
// It is intentionally minimal:
//   - Run(ctx): list stale durable runs; for each:
//   - skip if no checkpoint_id (cannot recover; orphan cleanup handles these)
//   - load checkpoint via CheckpointSaver; on failure, mark run failed
//   - on success, call DurableResumeGateway.ResumeDurableSessionRun
//   - Start(ctx): run once immediately, then poll on pollInterval.
type RecoveryWorker struct {
	lister       staleRunLister
	saver        trpcgraph.CheckpointSaver
	resumer      biz.DurableResumeGateway
	lg           loggateway.Logger
	pollInterval time.Duration
}

// NewRecoveryWorker constructs a RecoveryWorker. Returns nil if any
// required dependency is nil (defensive construction per red-line #26).
func NewRecoveryWorker(lister staleRunLister, saver trpcgraph.CheckpointSaver, resumer biz.DurableResumeGateway, lg loggateway.Logger) *RecoveryWorker {
	if lister == nil || saver == nil || resumer == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &RecoveryWorker{
		lister:       lister,
		saver:        saver,
		resumer:      resumer,
		lg:           lg.With(loggateway.Domain("recovery")),
		pollInterval: recoveryWorkerPollInterval,
	}
}

// Run performs a single scan-and-recover cycle. It returns an error only
// when listing stale runs fails; per-run failures are logged and the
// cycle continues so one bad run doesn't block recovery of others.
func (w *RecoveryWorker) Run(ctx context.Context) error {
	if w == nil {
		return nil
	}
	runs, err := w.lister.ListDurablePending(ctx, recoveryWorkerBatchSize)
	if err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, apierror.DomainBackgroundJob)
	}
	for _, run := range runs {
		w.recoverOne(ctx, run)
	}
	return nil
}

// recoverOne handles a single stale run.
func (w *RecoveryWorker) recoverOne(ctx context.Context, run biz.SessionRun) {
	if strings.TrimSpace(run.CheckpointID) == "" {
		// No checkpoint to recover from — skip silently. These runs are
		// cleaned up by MarkOrphanedRunsCancelled at startup.
		return
	}
	config := map[string]any{
		trpcgraph.CfgKeyConfigurable: map[string]any{
			trpcgraph.CfgKeyLineageID:    run.SessionID,
			trpcgraph.CfgKeyCheckpointID: run.CheckpointID,
		},
	}
	if _, err := w.saver.Get(ctx, config); err != nil {
		w.lg.Warn("checkpoint load failed; marking run failed",
			loggateway.StepID("recovery.worker"),
			loggateway.Str("run_id", run.ID),
			loggateway.Str("checkpoint_id", run.CheckpointID),
			loggateway.Err(err),
		)
		if ferr := w.lister.Fail(ctx, run.ID, fmt.Sprintf("checkpoint load failed: %v", err)); ferr != nil {
			w.lg.Warn("mark run failed after checkpoint load failure",
				loggateway.StepID("recovery.worker"),
				loggateway.Str("run_id", run.ID),
				loggateway.Err(ferr),
			)
		}
		return
	}
	if err := w.resumer.ResumeDurableSessionRun(ctx, run.ID); err != nil {
		w.lg.Warn("resume failed",
			loggateway.StepID("recovery.worker"),
			loggateway.Str("run_id", run.ID),
			loggateway.Err(err),
		)
		return
	}
	w.lg.Info("recovered run",
		loggateway.StepID("recovery.worker"),
		loggateway.Str("run_id", run.ID),
	)
}

// Start launches the worker in a background goroutine via safego.Go
// (red-line #13). The goroutine runs one cycle immediately so stale
// runs are recovered at startup, then polls on pollInterval until ctx
// is cancelled (red-line #23 goroutine exit path).
func (w *RecoveryWorker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	safego.Go(ctx, "recovery-worker", func() {
		if err := w.Run(ctx); err != nil {
			w.lg.Warn("recovery cycle failed",
				loggateway.StepID("recovery.worker"),
				loggateway.Err(err),
			)
		}
		ticker := time.NewTicker(w.pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := w.Run(ctx); err != nil {
					w.lg.Warn("recovery cycle failed",
						loggateway.StepID("recovery.worker"),
						loggateway.Err(err),
					)
				}
			}
		}
	})
}
