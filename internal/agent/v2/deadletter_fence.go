package v2

// deadletter_fence.go — FR-10: replay fencing for complete_task_terminal
// durable dead-letters.
//
// A terminal dead-letter is a stale snapshot of intent: between the persist
// failure and the replay sweep the task may have been resurrected (resume),
// terminalized by the LIVE path, or terminalized differently (failed vs
// cancelled). Re-applying blindly would clobber live state. Before each
// complete_task_terminal replay the fence reads live task state and decides:
//
//	current == requested terminal   → mark replayed (idempotent, nothing to do)
//	current == other terminal       → abandon (terminal_diverged: replay would开倒车)
//	current == running + active run → defer (live terminal will land itself;
//	                                  ≤deadLetterMaxDefer deferrals ≈1h, then abandoned)
//	current running w/o active run / pending / interrupted / row missing
//	                                → proceed with applyPersist (宕后合法收账路径)

import (
	"context"
	"strconv"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// deadLetterDeferMarkerPrefix marks a deferred replay in last_error as
// "deferred_active_run:<n>" so the defer budget survives restarts (attempts
// stays untouched for deferrals by design).
const deadLetterDeferMarkerPrefix = "deferred_active_run:"

// deadLetterMaxDefer caps active-run deferrals: 12 × deadLetterReplayInterval
// (5min) ≈ 1h. A longer wait means the "active run" is leaked, not live —
// abandon so a真死信 is not stranded forever (FR-10 风险行).
const deadLetterMaxDefer = 12

// parseDeadLetterDeferCount extracts the defer counter from last_error
// (0 when the marker is absent or malformed).
func parseDeadLetterDeferCount(lastError string) int {
	if !strings.HasPrefix(lastError, deadLetterDeferMarkerPrefix) {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimPrefix(lastError, deadLetterDeferMarkerPrefix))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// DeadLetterTerminalFence supplies the live-state reads that fence
// complete_task_terminal replays (FR-10). Optional: when unset, replay keeps
// the legacy blind-apply semantics. Stability: Internal.
type DeadLetterTerminalFence interface {
	// TaskStatusByID returns the persisted task status; found=false when the
	// task row is gone (replay falls back to applyPersist, which then fails
	// permanently via the normal attempt path).
	TaskStatusByID(ctx context.Context, taskID string) (status biz.TaskStatus, found bool, err error)
	// HasActiveRun reports whether the session currently holds an active run
	// (the live terminal event will land on its own; replay must defer).
	HasActiveRun(sessionID string) bool
}

// WithDeadLetterTerminalFence injects the FR-10 replay fence.
func WithDeadLetterTerminalFence(f DeadLetterTerminalFence) Option {
	return func(c *config) { c.terminalFence = f }
}

// TaskRunTerminalFence adapts the existing ports (task reader + run
// presence) to DeadLetterTerminalFence for production wiring.
type TaskRunTerminalFence struct {
	Tasks     biz.TaskV2Reader
	HasActive func(sessionID string) bool
}

func (f *TaskRunTerminalFence) TaskStatusByID(ctx context.Context, taskID string) (biz.TaskStatus, bool, error) {
	if f == nil || f.Tasks == nil {
		return "", false, nil
	}
	t, err := f.Tasks.GetTask(ctx, taskID)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	return t.Status, true, nil
}

func (f *TaskRunTerminalFence) HasActiveRun(sessionID string) bool {
	return f != nil && f.HasActive != nil && f.HasActive(sessionID)
}

// deadLetterFenceOutcome is the fence verdict for one record.
type deadLetterFenceOutcome int

const (
	fenceProceed   deadLetterFenceOutcome = iota // fall through to applyPersist
	fenceReplayed                                // already terminal as requested — marked replayed
	fenceAbandoned                               // terminal divergence / defer cap — abandoned
	fenceDeferred                                // active run holds the task — deferred (no attempt consumed)
)

// fenceTerminalTaskReplay applies the FR-10 fencing rules to a
// complete_task_terminal dead-letter. The fence itself performs the
// replayed/abandoned/deferred bookkeeping; fenceProceed means the caller
// should run the normal applyPersist path.
func (p *persistWorker) fenceTerminalTaskReplay(ctx context.Context, rec biz.EventDeadLetter, requested biz.TaskStatus) deadLetterFenceOutcome {
	if p.terminalFence == nil || rec.EntityKind != EntityKindTask || rec.EntityOp != PersistOpCompleteTaskTerminal {
		return fenceProceed
	}
	status, found, err := p.terminalFence.TaskStatusByID(ctx, rec.EntityID)
	if err != nil {
		// 读失败不推进任何计数（瞬态优先），下个 sweep 再读。
		p.lg.Warn("dead-letter fence: task status read failed, skipping this sweep",
			loggateway.Str("entity_id", rec.EntityID), loggateway.Err(err))
		return fenceDeferred
	}
	if found && biz.IsTerminalTaskV2Status(status) {
		if status == requested {
			// 终态已按请求落账（live 抢跑或幂等重放窗口）→ 直接结清。
			if markErr := p.store.MarkEventDeadLetterReplayed(ctx, rec.ID); markErr != nil {
				p.lg.Warn("dead-letter fence: mark replayed failed",
					loggateway.Str("entity_id", rec.EntityID), loggateway.Err(markErr))
			}
			metrics.SequencerDeadLetterReplayTotal.WithLabelValues("replayed").Inc()
			return fenceReplayed
		}
		// 任务已被另一终态收口——重放会覆盖 live 真实结局，永久弃置。
		p.abandonDeadLetter(ctx, rec, "terminal_diverged: current="+string(status)+", requested="+string(requested))
		return fenceAbandoned
	}
	if found && status == biz.TaskStatusRunning && p.terminalFence.HasActiveRun(rec.SessionID) {
		// 复活任务正在跑——live 终态会自己落账；挂起不消耗 attempts。
		deferN := parseDeadLetterDeferCount(rec.LastError)
		if deferN+1 > deadLetterMaxDefer {
			p.abandonDeadLetter(ctx, rec, "deferred_active_run_exceeded")
			return fenceAbandoned
		}
		if dm, ok := p.store.(biz.EventDeadLetterDeferMarker); ok {
			marker := deadLetterDeferMarkerPrefix + strconv.Itoa(deferN+1)
			if derr := dm.DeferEventDeadLetter(ctx, rec.ID, marker); derr != nil {
				p.lg.Warn("dead-letter fence: defer mark failed",
					loggateway.Str("entity_id", rec.EntityID), loggateway.Err(derr))
			}
		}
		metrics.SequencerDeadLetterReplayTotal.WithLabelValues("deferred").Inc()
		return fenceDeferred
	}
	// running 无活跃 run（僵尸 running）/pending/interrupted/行缺失 → 允许终态化
	// （宕后合法收账路径，与 CompleteTaskTerminal 注释语义一致）。
	return fenceProceed
}
