package runtime

import (
	"context"
	"errors"
	"sync"
	"time"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

// RunStatusEntry holds the lifecycle state of a single runtime run.
type RunStatusEntry struct {
	RunID     string
	Status    string // idle | pending | running | awaiting_user | completed | failed | cancelled
	ErrMsg    string
	UpdatedAt time.Time
}

type activeRun struct {
	runID       string
	runner      trpcrunner.Runner
	cancel      context.CancelFunc
	placeholder bool
}

// RunRegistry owns per-session runtime state shared by chat, websocket, and
// future runner control entrypoints.
type RunRegistry struct {
	activeRuns     sync.Map
	pendingCancels sync.Map
	runStatuses    sync.Map
}

func NewRunRegistry() *RunRegistry {
	return &RunRegistry{}
}

func (r *RunRegistry) HasActive(sessionID string) bool {
	if r == nil {
		return false
	}
	_, ok := r.activeRuns.Load(sessionID)
	return ok
}

func (r *RunRegistry) StorePlaceholder(sessionID string) {
	if r == nil {
		return
	}
	r.activeRuns.Store(sessionID, activeRun{placeholder: true})
}

func (r *RunRegistry) StoreRunner(sessionID, runID string, runner trpcrunner.Runner) {
	if r == nil {
		return
	}
	ar := activeRun{runID: runID, runner: runner}
	if prev, ok := r.activeRuns.Load(sessionID); ok {
		if existing, ok := prev.(activeRun); ok {
			ar.cancel = existing.cancel
			if ar.runID == "" {
				ar.runID = existing.runID
			}
		}
	}
	r.activeRuns.Store(sessionID, ar)
}

func (r *RunRegistry) StoreCancelable(sessionID, runID string, cancel context.CancelFunc) {
	if r == nil {
		return
	}
	ar := activeRun{runID: runID, cancel: cancel}
	if prev, ok := r.activeRuns.Load(sessionID); ok {
		if existing, ok := prev.(activeRun); ok {
			ar.runner = existing.runner
		}
	}
	r.activeRuns.Store(sessionID, ar)
}

func (r *RunRegistry) Finish(sessionID string) {
	if r == nil {
		return
	}
	r.activeRuns.Delete(sessionID)
}

func (r *RunRegistry) SetPendingCancel(sessionID string, cancel context.CancelFunc) {
	if r == nil || cancel == nil {
		return
	}
	r.pendingCancels.Store(sessionID, cancel)
}

func (r *RunRegistry) ClearPendingCancel(sessionID string) {
	if r == nil {
		return
	}
	r.pendingCancels.Delete(sessionID)
}

func (r *RunRegistry) Cancel(sessionID string) bool {
	if r == nil {
		return false
	}
	if cancelFn, ok := r.pendingCancels.LoadAndDelete(sessionID); ok {
		if c, ok := cancelFn.(context.CancelFunc); ok {
			c()
		}
	}
	val, ok := r.activeRuns.Load(sessionID)
	if !ok {
		return false
	}
	run, ok := val.(activeRun)
	if !ok {
		return false
	}
	if run.cancel != nil {
		run.cancel()
		r.SetStatus(sessionID, run.runID, "cancelled", "")
		r.activeRuns.Delete(sessionID)
		return true
	}
	if run.placeholder || run.runner == nil {
		return false
	}
	if mr, ok := run.runner.(trpcrunner.ManagedRunner); ok && mr.Cancel(sessionID) {
		return true
	}
	_ = run.runner.Close()
	r.activeRuns.Delete(sessionID)
	return true
}

func (r *RunRegistry) EnqueueUserMessage(sessionID, content string) (bool, error) {
	if r == nil {
		return false, nil
	}
	val, ok := r.activeRuns.Load(sessionID)
	if !ok {
		return false, nil
	}
	run, ok := val.(activeRun)
	if !ok || run.placeholder || run.runner == nil {
		return false, nil
	}
	err := trpcrunner.EnqueueUserMessage(run.runner, sessionID, trpcmodel.NewUserMessage(content))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, trpcrunner.ErrQueuedUserMessageUnsupported) || errors.Is(err, trpcrunner.ErrRunNotFound) {
		return false, nil
	}
	return false, err
}

func (r *RunRegistry) SetStatus(sessionID, runID, status, errMsg string) {
	if r == nil {
		return
	}
	r.runStatuses.Store(sessionID, &RunStatusEntry{
		RunID:     runID,
		Status:    status,
		ErrMsg:    errMsg,
		UpdatedAt: time.Now(),
	})
}

// ActiveRunner returns the in-flight runner and service run id when present.
func (r *RunRegistry) ActiveRunner(sessionID string) (trpcrunner.Runner, string, bool) {
	if r == nil {
		return nil, "", false
	}
	val, ok := r.activeRuns.Load(sessionID)
	if !ok {
		return nil, "", false
	}
	run, ok := val.(activeRun)
	if !ok || run.placeholder || run.runner == nil {
		return nil, "", false
	}
	return run.runner, run.runID, true
}

func (r *RunRegistry) GetStatus(sessionID string) (RunStatusEntry, bool) {
	if r == nil {
		return RunStatusEntry{}, false
	}
	val, ok := r.runStatuses.Load(sessionID)
	if !ok {
		return RunStatusEntry{}, false
	}
	entry, ok := val.(*RunStatusEntry)
	if !ok || entry == nil {
		return RunStatusEntry{}, false
	}
	return *entry, true
}
