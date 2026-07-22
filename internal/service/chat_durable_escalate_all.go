package service

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// shutdownEscalateBatchLimit caps how many interactive runs are escalated to
// durable during a single shutdown. In practice a single-process server hosts
// a handful of active runs; the cap is a defensive bound.
const shutdownEscalateBatchLimit = 64

// EscalateAllActiveToDurable escalates every interactive SessionRun to durable
// mode during server shutdown (L2 crash protection, 2026-07-22).
//
// For each active run it writes a durable checkpoint (SQLite/PG-backed
// SessionRunCheckpoint), transitions the phase to durable, and cancels the
// in-process runner. After restart, SessionRunDurableWorker picks up the
// durable runs and resumes them automatically — the user reopens the app to
// find the task already continuing.
//
// Best-effort: per-run failures are logged and skipped so one bad run does not
// block the shutdown path. Returns the number of runs successfully escalated.
func (s *ChatService) EscalateAllActiveToDurable(ctx context.Context) int {
	if s == nil || s.orch == nil || s.orch.chJobs().SessionRuns == nil {
		return 0
	}
	runs, err := s.orch.chJobs().SessionRuns.ListByPhase(ctx, biz.SessionRunPhaseInteractive, shutdownEscalateBatchLimit)
	if err != nil {
		s.lg.Warn("shutdown durable escalate: list interactive runs failed",
			loggateway.StepID("chat.durable_escalate_all"),
			loggateway.Err(err))
		return 0
	}
	escalated := 0
	for _, run := range runs {
		if run.ID == "" || run.SessionID == "" {
			continue
		}
		s.orch.sessionRunLC().EscalateToDurableOnShutdown(ctx, run.SessionID, run.ID)
		escalated++
	}
	if escalated > 0 {
		s.lg.Info("shutdown durable escalate: interactive runs escalated",
			loggateway.StepID("chat.durable_escalate_all"),
			loggateway.Int("count", escalated))
	}
	return escalated
}
