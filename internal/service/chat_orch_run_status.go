package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// RunStatusWriter manages run status transitions and persistence.
// Stability:evolving
type RunStatusWriter interface {
	SetRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) error
	SetRunStatusWithAwait(ctx context.Context, sessionID, runID, status, errMsg string, await *AwaitStatusMeta) error
	PublishRunStatus(sessionID, runID, status, errMsg string)
	PersistRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) error
}

// RunStatusReader provides read access to run status.
// Stability:evolving
type RunStatusReader interface {
	GetRunStatus(ctx context.Context, sessionID string) (runID, status, errMsg, updatedAt string, ok bool)
	HydrateRunStatusFromSession(ctx context.Context, sessionID string) (persistedRunStatus, bool)
}

// BindingManager manages session-run bindings.
// Stability:evolving
type BindingManager interface {
	StoreBinding(sessionID string, binding sessionRunTurnBinding)
	LoadBinding(sessionID string) (sessionRunTurnBinding, bool)
	DeleteBinding(sessionID string)
}

// AwaitMetaManager manages await metadata cache and persistence.
// Stability:evolving
type AwaitMetaManager interface {
	SetAwaitMetaCache(sessionID string, meta biz.ChatAwaitMeta)
	GetAwaitMetaCache(sessionID string) (biz.ChatAwaitMeta, bool)
	ClearAwaitMetaCache(sessionID string)
	PersistAwaitMarkers(ctx context.Context, sessionID, runID string, await AwaitStatusMeta, syncWrite bool)
	ResolveAwaitMeta(ctx context.Context, sessionID, status string) biz.ChatAwaitMeta
}

// AwaitStateCleaner clears awaiting run state.
// Stability:evolving
type AwaitStateCleaner interface {
	ClearAwaitingRunStateSync(ctx context.Context, sessionID string) error
	ClearAwaitingRunState(ctx context.Context, sessionID string)
}

// runStatusTracker is the composite interface for backward compatibility.
// Stability:evolving
type runStatusTracker interface {
	RunStatusWriter
	RunStatusReader
	BindingManager
	AwaitMetaManager
	AwaitStateCleaner
	Sweep()
}

// chatRunStatusTracker implements runStatusTracker.
//
// Part of the TECH-DEBT(BL8) resolution: extracting run status management
// from ChatOrchestrator to reduce cognitive complexity (AS-COG-01).
//
// Phase 3b-D Task 9: migrated bus field from v1 ActivityEventBus to v2 EventBus;
// PublishRunStatusFull now emits biz.RunStatusEvent.
type chatRunStatusTracker struct {
	runs       *rt.RunRegistry
	sessions   biz.SessionStatePort
	bus        biz.EventBus
	bindings   *TypedSyncMap[string, sessionRunTurnBinding]
	awaitCache *TypedSyncMap[string, biz.ChatAwaitMeta]
	sm         *biz.RunStateMachine
	lg         loggateway.Logger
}

func newChatRunStatusTracker(runs *rt.RunRegistry, sessions biz.SessionStatePort, bus biz.EventBus, lg loggateway.Logger) *chatRunStatusTracker {
	return &chatRunStatusTracker{
		runs:       runs,
		sessions:   sessions,
		bus:        bus,
		bindings:   NewTypedSyncMap[string, sessionRunTurnBinding](orchMapMaxIdle),
		awaitCache: NewTypedSyncMap[string, biz.ChatAwaitMeta](orchMapMaxIdle),
		sm:         biz.NewRunStateMachine(),
		lg:         lg,
	}
}

// Compile-time interface check.
var _ runStatusTracker = (*chatRunStatusTracker)(nil)

// SetRunStatus atomically updates the run status, publishes a WS envelope, and persists.
func (t *chatRunStatusTracker) SetRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) error {
	return t.SetRunStatusWithAwait(ctx, sessionID, runID, status, errMsg, nil)
}

// SetRunStatusWithAwait same as SetRunStatus but includes await metadata.
func (t *chatRunStatusTracker) SetRunStatusWithAwait(ctx context.Context, sessionID, runID, status, errMsg string, await *AwaitStatusMeta) error {
	// FSM validation (AS-FSM-01): reject illegal transitions.
	// When no prior status record exists (bootstrap/crash recovery), validation
	// is skipped to allow the first status write.
	// TECH-DEBT(FSM): retry-from-terminal needs explicit state machine rule;
	// current behavior rejects terminal→running transitions.
	if t.sm != nil {
		from := biz.RunStateNone
		hasCurrent := false
		if entry, ok := t.runs.GetStatus(sessionID); ok {
			from = biz.ParseRunState(entry.Status)
			hasCurrent = true
		}
		to := biz.ParseRunState(status)
		if hasCurrent && from != to && !t.sm.CanTransition(from, to) {
			t.lg.Warn("run: illegal state transition rejected by FSM",
				loggateway.StepID("run.fsm_illegal"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("run_id", runID),
				loggateway.Str("from", string(from)),
				loggateway.Str("to", string(to)))
			return apierror.BadRequest(apierror.DomainChat, "illegal run state transition: %s → %s", from, to)
		}
	}
	t.runs.SetStatus(sessionID, runID, status, errMsg)
	bind, _ := t.LoadBinding(sessionID)
	if await != nil {
		PublishRunStatusFull(t.bus, sessionID, runID, status, errMsg, await, bind.sessionRunID, bind.turnID)
	} else {
		PublishRunStatusFull(t.bus, sessionID, runID, status, errMsg, nil, bind.sessionRunID, bind.turnID)
	}
	// C-11 fix: log on persist failure so operators can detect DB/memory
	// divergence (WS already published; on restart the old status is restored).
	if err := t.PersistRunStatus(ctx, sessionID, runID, status, errMsg); err != nil {
		t.lg.Warn("persist run status failed; DB/memory may diverge on restart",
			loggateway.StepID("run.persist_failed"),
			loggateway.SessionID(sessionID),
			loggateway.Str("run_id", runID),
			loggateway.Str("status", status),
			loggateway.Err(err))
	}
	return nil
}

// PublishRunStatus publishes a WS run_status envelope (no state change).
func (t *chatRunStatusTracker) PublishRunStatus(sessionID, runID, status, errMsg string) {
	PublishRunStatus(t.bus, sessionID, runID, status, errMsg)
}

// PersistRunStatus persists run status to session state.
func (t *chatRunStatusTracker) PersistRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) error {
	return persistRunStatusToSession(t.sessions, ctx, sessionID, runID, status, errMsg)
}

// HydrateRunStatusFromSession loads run status from session state.
func (t *chatRunStatusTracker) HydrateRunStatusFromSession(ctx context.Context, sessionID string) (persistedRunStatus, bool) {
	if t == nil || t.sessions == nil {
		return persistedRunStatus{}, false
	}
	state, err := t.sessions.GetSessionState(ctx, sessionID)
	if err != nil || len(state) == 0 {
		return persistedRunStatus{}, false
	}
	status := strings.TrimSpace(state[stateKeyRunStatus])
	if status == "" {
		return persistedRunStatus{}, false
	}
	return persistedRunStatus{
		RunID:           strings.TrimSpace(state[stateKeyRunID]),
		Status:          status,
		ErrorMessage:    strings.TrimSpace(state[stateKeyRunError]),
		UpdatedAt:       strings.TrimSpace(state[stateKeyRunUpdatedAt]),
		AwaitKind:       strings.TrimSpace(state[stateKeyAwaitKind]),
		AwaitToolKey:    strings.TrimSpace(state[stateKeyAwaitToolKey]),
		AwaitToolCallID: strings.TrimSpace(state[stateKeyAwaitToolCallID]),
	}, true
}

// GetRunStatus returns the current run lifecycle state for a session.
func (t *chatRunStatusTracker) GetRunStatus(ctx context.Context, sessionID string) (runID, status, errMsg, updatedAt string, ok bool) {
	if entry, ok2 := t.runs.GetStatus(sessionID); ok2 {
		ua := ""
		if !entry.UpdatedAt.IsZero() {
			ua = entry.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		return entry.RunID, entry.Status, entry.ErrMsg, ua, true
	}
	return "", "", "", "", false
}

// StoreBinding stores a session-run binding.
func (t *chatRunStatusTracker) StoreBinding(sessionID string, binding sessionRunTurnBinding) {
	t.bindings.Store(strings.TrimSpace(sessionID), binding)
}

// LoadBinding loads a session-run binding.
func (t *chatRunStatusTracker) LoadBinding(sessionID string) (sessionRunTurnBinding, bool) {
	return t.bindings.Load(strings.TrimSpace(sessionID))
}

// DeleteBinding deletes a session-run binding.
func (t *chatRunStatusTracker) DeleteBinding(sessionID string) {
	t.bindings.Delete(strings.TrimSpace(sessionID))
}

// SetAwaitMetaCache stores await metadata in the in-memory cache.
func (t *chatRunStatusTracker) SetAwaitMetaCache(sessionID string, meta biz.ChatAwaitMeta) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	t.awaitCache.Store(sessionID, meta)
}

// GetAwaitMetaCache retrieves await metadata from the in-memory cache.
func (t *chatRunStatusTracker) GetAwaitMetaCache(sessionID string) (biz.ChatAwaitMeta, bool) {
	return t.awaitCache.Load(strings.TrimSpace(sessionID))
}

// ClearAwaitMetaCache removes await metadata from the in-memory cache.
func (t *chatRunStatusTracker) ClearAwaitMetaCache(sessionID string) {
	t.awaitCache.Delete(strings.TrimSpace(sessionID))
}

// PersistAwaitMarkers stores await metadata in cache and persists to session state.
func (t *chatRunStatusTracker) PersistAwaitMarkers(ctx context.Context, sessionID, runID string, await AwaitStatusMeta, syncWrite bool) {
	t.SetAwaitMetaCache(sessionID, await)
	persistAwaitMarkersToSession(t.sessions, ctx, sessionID, runID, await, syncWrite, t.lg)
}

// ResolveAwaitMeta resolves await metadata from cache or session state.
func (t *chatRunStatusTracker) ResolveAwaitMeta(ctx context.Context, sessionID, status string) biz.ChatAwaitMeta {
	if strings.TrimSpace(status) != "awaiting_user" {
		return biz.ChatAwaitMeta{}
	}
	if meta, ok := t.GetAwaitMetaCache(sessionID); ok {
		return meta
	}
	if snap, ok := t.HydrateRunStatusFromSession(ctx, sessionID); ok {
		return biz.ChatAwaitMeta{
			Kind:       snap.AwaitKind,
			ToolKey:    snap.AwaitToolKey,
			ToolCallID: snap.AwaitToolCallID,
		}
	}
	return biz.ChatAwaitMeta{}
}

// ClearAwaitingRunStateSync clears awaiting run state synchronously.
func (t *chatRunStatusTracker) ClearAwaitingRunStateSync(ctx context.Context, sessionID string) error {
	if t == nil || t.sessions == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	state, err := t.sessions.GetSessionState(ctx, sessionID)
	if err != nil {
		return err
	}
	if len(state) == 0 {
		return nil
	}
	t.ClearAwaitMetaCache(sessionID)
	delete(state, stateKeyRunID)
	delete(state, stateKeyRunStatus)
	delete(state, stateKeyRunError)
	delete(state, stateKeyRunUpdatedAt)
	delete(state, stateKeyAwaitRunID)
	delete(state, stateKeyAwaitSince)
	delete(state, stateKeyAwaitKind)
	delete(state, stateKeyAwaitToolKey)
	delete(state, stateKeyAwaitToolCallID)
	return t.sessions.SaveSessionState(ctx, sessionID, state)
}

// ClearAwaitingRunState clears awaiting run state asynchronously.
func (t *chatRunStatusTracker) ClearAwaitingRunState(ctx context.Context, sessionID string) {
	t.ClearAwaitMetaCache(sessionID)
	clearAwaitingRunStateFromSession(t.sessions, ctx, sessionID, t.lg)
}

// Sweep removes expired entries from bindings and await cache.
func (t *chatRunStatusTracker) Sweep() {
	t.bindings.Sweep()
	t.awaitCache.Sweep()
}
