package session

import (
	"context"
	"fmt"
	"strings"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

const (
	stateKeyRunnerSnapshot    = "aranea:runner_snapshot"
	stateKeyCompressedSummary = "aranea:compressed_summary"
	stateKVPrefix             = "aranea:state:"
)

// Runtime wraps trpc session.Service with Aranea session-key conventions.
type Runtime struct {
	svc trpcsession.Service
}

func NewRuntime(svc trpcsession.Service) *Runtime {
	return &Runtime{svc: svc}
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
		return fmt.Errorf("session sync key: %w", err)
	}
	if _, err := r.svc.GetSession(ctx, k); err != nil {
		return fmt.Errorf("session sync get: %w", err)
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
		return fmt.Errorf("session sync update: %w", err)
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
		return fmt.Errorf("session state sync key: %w", err)
	}
	if _, err := r.svc.GetSession(ctx, k); err != nil {
		return fmt.Errorf("session state sync get: %w", err)
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
		return fmt.Errorf("session state sync update: %w", err)
	}
	return nil
}
