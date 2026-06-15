package runtime

import (
	"context"
	"errors"
	"sync"
	"time"

	"aranea-agents/internal/biz"
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

// activeRunMap wraps sync.Map with typed accessor methods to eliminate
// unsafe type assertions in external callers.
type activeRunMap struct {
	m sync.Map
}

func (a *activeRunMap) load(key string) (activeRun, bool) {
	v, ok := a.m.Load(key)
	if !ok {
		return activeRun{}, false
	}
	ar, ok := v.(activeRun)
	return ar, ok
}

func (a *activeRunMap) store(key string, val activeRun) { a.m.Store(key, val) }
func (a *activeRunMap) delete(key string)               { a.m.Delete(key) }

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
type RunRegistry struct {
	activeRuns     activeRunMap
	pendingCancels cancelMap
	runStatuses    statusMap
	lg             loggateway.Logger
}

func NewRunRegistry() *RunRegistry {
	return &RunRegistry{}
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
	_, ok := r.activeRuns.load(sessionID)
	return ok
}

func (r *RunRegistry) StorePlaceholder(sessionID string) {
	if r == nil {
		return
	}
	r.activeRuns.store(sessionID, activeRun{placeholder: true})
}

func (r *RunRegistry) StoreRunner(sessionID, runID string, runner trpcrunner.Runner) {
	if r == nil {
		return
	}
	ar := activeRun{runID: runID, runner: runner}
	if existing, ok := r.activeRuns.load(sessionID); ok {
		ar.cancel = existing.cancel
		if ar.runID == "" {
			ar.runID = existing.runID
		}
	}
	r.activeRuns.store(sessionID, ar)
}

func (r *RunRegistry) StoreCancelable(sessionID, runID string, cancel context.CancelFunc) {
	if r == nil {
		return
	}
	ar := activeRun{runID: runID, cancel: cancel}
	if existing, ok := r.activeRuns.load(sessionID); ok {
		ar.runner = existing.runner
	}
	r.activeRuns.store(sessionID, ar)
}

func (r *RunRegistry) Finish(sessionID string) {
	if r == nil {
		return
	}
	r.activeRuns.delete(sessionID)
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
	if cancelFn, ok := r.pendingCancels.loadAndDelete(sessionID); ok {
		cancelFn()
	}
	run, ok := r.activeRuns.load(sessionID)
	if !ok {
		return false, ""
	}
	if run.cancel != nil {
		run.cancel()
		r.SetStatus(sessionID, run.runID, biz.SessionRunPhaseCancelled, "")
		if current, ok := r.activeRuns.load(sessionID); ok && current.runID == run.runID {
			r.activeRuns.delete(sessionID)
		}
		return true, run.runID
	}
	if run.placeholder || run.runner == nil {
		return false, ""
	}
	if mr, ok := run.runner.(trpcrunner.ManagedRunner); ok && mr.Cancel(sessionID) {
		return true, run.runID
	}
	if err := run.runner.Close(); err != nil && r.lg != nil {
		r.lg.Warn("runner close error during cancel", loggateway.SessionID(sessionID), loggateway.Err(err))
	}
	r.activeRuns.delete(sessionID)
	return true, run.runID
}

func (r *RunRegistry) EnqueueUserMessage(sessionID, content string) (bool, error) {
	if r == nil {
		return false, nil
	}
	run, ok := r.activeRuns.load(sessionID)
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
	run, ok := r.activeRuns.load(sessionID)
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
