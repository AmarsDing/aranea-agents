package session

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

const (
	stateKeyRunnerSnapshot    = "aranea:runner_snapshot"
	stateKeyCompressedSummary = "aranea:compressed_summary"
	stateKVPrefix             = "aranea:state:"
	stateKeyForceL0Snapshot   = "aranea:force_l0_snapshot"
)

// Runtime wraps trpc session.Service with Aranea session-key conventions.
type Runtime struct {
	svc trpcsession.Service
	lg  loggateway.Logger

	// forceL0Snapshot tracks sessions that need an immediate L0 snapshot write
	// (bypassing throttle). Set after compression; cleared after the next snapshot write.
	forceL0Snapshot sync.Map
}

func NewRuntime(svc trpcsession.Service, lg loggateway.Logger) *Runtime {
	return &Runtime{svc: svc, lg: lg}
}

func (r *Runtime) Service() trpcsession.Service {
	if r == nil {
		return nil
	}
	return r.svc
}

// Get loads the framework session for a user/session pair.
func (r *Runtime) Get(ctx context.Context, userID, sessionID string) (*trpcsession.Session, error) {
	if r == nil || r.svc == nil {
		return nil, trpcsession.ErrNilSession
	}
	k := Key(userID, sessionID)
	if err := k.CheckSessionKey(); err != nil {
		return nil, err
	}
	return r.svc.GetSession(ctx, k)
}

// HasPendingAwaitUserReply reports whether the trpc session has a pending await-user route.
func (r *Runtime) HasPendingAwaitUserReply(ctx context.Context, userID, sessionID string) bool {
	sess, err := r.Get(ctx, userID, sessionID)
	if err != nil || sess == nil {
		return false
	}
	_, pending, err := trpcagent.PendingAwaitUserReplyRoute(sess)
	return err == nil && pending
}

// SyncRunnerSnapshot mirrors Ent runner_snapshot_json into trpc session state for Runner reload.
func (r *Runtime) SyncRunnerSnapshot(ctx context.Context, userID, sessionID, snapshotJSON, summaryMarkdown string) error {
	if r == nil || r.svc == nil {
		return nil
	}
	k := Key(userID, sessionID)
	if err := k.CheckSessionKey(); err != nil {
		r.lg.Warn("session sync key invalid", loggateway.StepID("session.sync_snapshot"), loggateway.SessionID(sessionID), loggateway.Err(err))
		return apierror.Internal(apierror.DomainSession, "session sync key").WithCause(err)
	}
	if _, err := r.svc.GetSession(ctx, k); err != nil {
		r.lg.Warn("session sync get failed", loggateway.StepID("session.sync_snapshot"), loggateway.SessionID(sessionID), loggateway.Err(err))
		return apierror.Internal(apierror.DomainSession, "session sync get").WithCause(err)
	}
	state := trpcsession.StateMap{}
	if raw := strings.TrimSpace(snapshotJSON); raw != "" {
		state[stateKeyRunnerSnapshot] = []byte(raw)
	}
	if md := strings.TrimSpace(summaryMarkdown); md != "" {
		state[stateKeyCompressedSummary] = []byte(md)
	}
	if len(state) == 0 {
		return nil
	}
	if err := r.svc.UpdateSessionState(ctx, k, state); err != nil {
		r.lg.Warn("session sync update failed", loggateway.StepID("session.sync_snapshot"), loggateway.SessionID(sessionID), loggateway.Err(err))
		return apierror.Internal(apierror.DomainSession, "session sync update").WithCause(err)
	}
	return nil
}

// AppendUserContextEvent injects a user-authored message into the trpc session
// event stream so subsequent turns include it in the LLM request history. It
// mirrors the runner's own appendIncomingMessage mechanism (a user-role
// response event) and is used for out-of-band user decisions (e.g. plan
// confirmation) that must become LLM context without running a turn.
// The session is created when missing (e.g. after a process restart) so the
// decision survives until the next turn. Empty content is rejected before any
// write.
func (r *Runtime) AppendUserContextEvent(ctx context.Context, userID, sessionID, invocationID, content string) error {
	if r == nil || r.svc == nil {
		return apierror.Internal(apierror.DomainSession, "session runtime unavailable")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return apierror.BadRequest(apierror.DomainSession, "content is required")
	}
	k := Key(userID, sessionID)
	if err := k.CheckSessionKey(); err != nil {
		return err
	}
	sess, err := r.svc.GetSession(ctx, k)
	if err != nil || sess == nil {
		if sess, err = r.svc.CreateSession(ctx, k, nil); err != nil {
			r.lg.Warn("session context inject create failed",
				loggateway.StepID("session.inject_context"),
				loggateway.SessionID(sessionID),
				loggateway.Err(err))
			return apierror.Internal(apierror.DomainSession, "session context inject create").WithCause(err)
		}
	}
	evt := trpcevent.NewResponseEvent(invocationID, "user", &trpcmodel.Response{
		Done:    false,
		Choices: []trpcmodel.Choice{{Index: 0, Message: trpcmodel.NewUserMessage(content)}},
	})
	if err := r.svc.AppendEvent(ctx, sess, evt); err != nil {
		r.lg.Warn("session context inject append failed",
			loggateway.StepID("session.inject_context"),
			loggateway.SessionID(sessionID),
			loggateway.Err(err))
		return apierror.Internal(apierror.DomainSession, "session context inject append").WithCause(err)
	}
	return nil
}

// EnqueueFrameworkSummary triggers trpc built-in async summarization when supported.
func (r *Runtime) EnqueueFrameworkSummary(ctx context.Context, userID, sessionID string, force bool) error {
	if r == nil || r.svc == nil {
		return nil
	}
	sess, err := r.Get(ctx, userID, sessionID)
	if err != nil || sess == nil {
		return err
	}
	return r.svc.EnqueueSummaryJob(ctx, sess, trpcsession.SummaryFilterKeyAllContents, force)
}

// MarkForceL0Snapshot flags a session for immediate L0 snapshot on the next model call,
// bypassing the throttle interval. Called after successful compression.
func (r *Runtime) MarkForceL0Snapshot(sessionID string) {
	if r == nil {
		return
	}
	r.forceL0Snapshot.Store(sessionID, true)
}

// ConsumeForceL0Snapshot checks and clears the force-write flag for a session.
// Returns true if the next L0 snapshot should bypass throttling.
func (r *Runtime) ConsumeForceL0Snapshot(sessionID string) bool {
	if r == nil {
		return false
	}
	_, loaded := r.forceL0Snapshot.LoadAndDelete(sessionID)
	return loaded
}

func stateKVKey(path string) string {
	return stateKVPrefix + strings.TrimSpace(path)
}

// SyncStateDelta mirrors one Ent session KV delta into trpc session state.
func (r *Runtime) SyncStateDelta(ctx context.Context, userID, sessionID, operation, path, valueJSON string) error {
	if r == nil || r.svc == nil {
		return nil
	}
	path = strings.TrimSpace(path)
	if path == "" || path == "__state__" {
		return nil
	}
	k := Key(userID, sessionID)
	if err := k.CheckSessionKey(); err != nil {
		r.lg.Warn("session state sync key invalid", loggateway.StepID("session.sync_state"), loggateway.SessionID(sessionID), loggateway.Err(err))
		return apierror.Internal(apierror.DomainSession, "session state sync key").WithCause(err)
	}
	if _, err := r.svc.GetSession(ctx, k); err != nil {
		r.lg.Warn("session state sync get failed", loggateway.StepID("session.sync_state"), loggateway.SessionID(sessionID), loggateway.Err(err))
		return apierror.Internal(apierror.DomainSession, "session state sync get").WithCause(err)
	}
	state := trpcsession.StateMap{}
	key := stateKVKey(path)
	switch strings.ToLower(strings.TrimSpace(operation)) {
	case "delete":
		state[key] = nil
	default:
		state[key] = []byte(valueJSON)
	}
	if err := r.svc.UpdateSessionState(ctx, k, state); err != nil {
		r.lg.Warn("session state sync update failed", loggateway.StepID("session.sync_state"), loggateway.SessionID(sessionID), loggateway.Err(err))
		return apierror.Internal(apierror.DomainSession, "session state sync update").WithCause(err)
	}
	return nil
}
