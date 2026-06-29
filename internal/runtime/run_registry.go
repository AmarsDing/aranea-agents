package runtime

import (
	"context"
	"errors"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/runtime/lifecycle"
	"aranea-agents/pkg/loggateway"

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

// cancelMap wraps sync.Map for context.CancelFunc storage.
type cancelMap struct {
	m sync.Map
}

func (c *cancelMap) load(key string) (context.CancelFunc, bool) {
	v, ok := c.m.Load(key)
	if !ok {
		return nil, false
	}
	cf, ok := v.(context.CancelFunc)
	return cf, ok
}

func (c *cancelMap) store(key string, val context.CancelFunc) { c.m.Store(key, val) }
func (c *cancelMap) loadAndDelete(key string) (context.CancelFunc, bool) {
	v, ok := c.m.LoadAndDelete(key)
	if !ok {
		return nil, false
	}
	cf, ok := v.(context.CancelFunc)
	return cf, ok
}
func (c *cancelMap) delete(key string) { c.m.Delete(key) }

// statusMap wraps sync.Map for *RunStatusEntry storage.
type statusMap struct {
	m sync.Map
}

func (s *statusMap) load(key string) (*RunStatusEntry, bool) {
	v, ok := s.m.Load(key)
	if !ok {
		return nil, false
	}
	e, ok := v.(*RunStatusEntry)
	return e, ok
}

func (s *statusMap) store(key string, val *RunStatusEntry) { s.m.Store(key, val) }
func (s *statusMap) delete(key string)                     { s.m.Delete(key) }

// RunRegistry owns per-session runtime state shared by chat, websocket, team,
// cron, channel, and gateway ingress.
//
// trpc runner request_id is the session_id (see trpcagent.WithRequestID in chat/team turns).
//
// A1: activeRuns uses lifecycle.ManagedMap (ttl=0, terminal-state cleanup via
// Finish) instead of a hand-rolled sync.Map+mutex wrapper. The ManagedMap's
// built-in UpdateOrStore provides the same TOCTOU protection as the previous
// updateOrStore, without the bespoke mutex.
type RunRegistry struct {
	activeRuns     *lifecycle.ManagedMap[string, activeRun]
	pendingCancels cancelMap
	runStatuses    statusMap
	lg             loggateway.Logger
	cancelMu       sync.Mutex // serializes Cancel/Finish to prevent double-close races
}

func NewRunRegistry() *RunRegistry {
	return &RunRegistry{
		activeRuns: lifecycle.NewManagedMap[string, activeRun](0),
	}
}

// WithLogger sets the logger for the registry. Returns the same registry for chaining.
func (r *RunRegistry) WithLogger(lg loggateway.Logger) *RunRegistry {
	if r != nil {
		r.lg = lg.With(loggateway.Domain("run_registry"))
	}
	return r
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
	// A1: use ManagedMap.UpdateOrStore for atomic load-modify-store. Previously
	// load+store was non-atomic: a concurrent StoreCancelable could
	// overwrite the runner reference, or a concurrent StoreRunner could
	// lose the cancel func.
	r.activeRuns.UpdateOrStore(sessionID, func(existing activeRun, ok bool) activeRun {
		ar := activeRun{runID: runID, runner: runner}
		if ok {
			ar.cancel = existing.cancel
			if ar.runID == "" {
				ar.runID = existing.runID
			}
		}
		return ar
	})
}

func (r *RunRegistry) StoreCancelable(sessionID, runID string, cancel context.CancelFunc) {
	if r == nil {
		return
	}
	// A1: use ManagedMap.UpdateOrStore for atomic load-modify-store. Previously
	// load+store was non-atomic: a concurrent StoreRunner could
	// overwrite the cancel func, or a concurrent StoreCancelable could
	// lose the runner reference.
	r.activeRuns.UpdateOrStore(sessionID, func(existing activeRun, ok bool) activeRun {
		ar := activeRun{runID: runID, cancel: cancel}
		if ok {
			ar.runner = existing.runner
			if ar.runID == "" {
				ar.runID = existing.runID
			}
		}
		return ar
	})
}

func (r *RunRegistry) Finish(sessionID string) {
	if r == nil {
		return
	}
	r.cancelMu.Lock()
	defer r.cancelMu.Unlock()
	r.activeRuns.Delete(sessionID)
	r.runStatuses.delete(sessionID)
	r.pendingCancels.delete(sessionID)
}

func (r *RunRegistry) SetPendingCancel(sessionID string, cancel context.CancelFunc) {
	if r == nil || cancel == nil {
		return
	}
	r.pendingCancels.store(sessionID, cancel)
}

func (r *RunRegistry) ClearPendingCancel(sessionID string) {
	if r == nil {
		return
	}
	r.pendingCancels.delete(sessionID)
}

func (r *RunRegistry) Cancel(sessionID, reason string) (bool, string) {
	if r == nil {
		return false, ""
	}
	r.cancelMu.Lock()
	defer r.cancelMu.Unlock()

	// Drain pending cancel func (if any) before touching the active run.
	if cancelFn, ok := r.pendingCancels.loadAndDelete(sessionID); ok {
		cancelFn()
	}

	// Load the run; if already gone (finished or cancelled by a concurrent
	// Cancel), there is nothing to do.
	run, ok := r.activeRuns.Load(sessionID)
	if !ok {
		return false, ""
	}

	// Path 1: context.CancelFunc present (chat/team turns). Invoke once and
	// delete the entry so subsequent Cancel calls are no-ops.
	if run.cancel != nil {
		run.cancel()
		r.SetStatus(sessionID, run.runID, biz.SessionRunPhaseCancelled, "")
		// Double-check the active run is still the one we cancelled; a
		// concurrent Finish or a new StoreRunner may have replaced it.
		if current, ok := r.activeRuns.Load(sessionID); ok && current.runID == run.runID {
			r.activeRuns.Delete(sessionID)
		}
		return true, run.runID
	}

	// Path 2: placeholder or no runner — nothing to cancel.
	if run.placeholder || run.runner == nil {
		return false, ""
	}

	// Path 3: ManagedRunner.Cancel — idempotent within the registry because we
	// delete the entry immediately after a successful cancel.
	if mr, ok := run.runner.(trpcrunner.ManagedRunner); ok && mr.Cancel(sessionID) {
		r.SetStatus(sessionID, run.runID, biz.SessionRunPhaseCancelled, "")
		// Double-check the run is still the same before deleting.
		if current, ok := r.activeRuns.Load(sessionID); ok && current.runID == run.runID {
			r.activeRuns.Delete(sessionID)
		}
		return true, run.runID
	}

	// Path 4: fallback to Close. Delete first so concurrent callers see the
	// run is gone and skip Close.
	if current, ok := r.activeRuns.Load(sessionID); ok && current.runID == run.runID {
		r.activeRuns.Delete(sessionID)
	}
	if err := run.runner.Close(); err != nil && r.lg != nil {
		r.lg.Warn("runner close error during cancel", loggateway.SessionID(sessionID), loggateway.Err(err))
	}
	return true, run.runID
}

func (r *RunRegistry) EnqueueUserMessage(sessionID, content string) (bool, error) {
	if r == nil {
		return false, nil
	}
	run, ok := r.activeRuns.Load(sessionID)
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
	r.runStatuses.store(sessionID, &RunStatusEntry{
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
	run, ok := r.activeRuns.Load(sessionID)
	if !ok || run.placeholder || run.runner == nil {
		return nil, "", false
	}
	return run.runner, run.runID, true
}

func (r *RunRegistry) GetStatus(sessionID string) (RunStatusEntry, bool) {
	if r == nil {
		return RunStatusEntry{}, false
	}
	entry, ok := r.runStatuses.load(sessionID)
	if !ok || entry == nil {
		return RunStatusEntry{}, false
	}
	return *entry, true
}
