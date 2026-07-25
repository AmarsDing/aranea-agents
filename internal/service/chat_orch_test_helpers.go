package service

import (
	"context"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
)

// --- noop sub-manager stubs for ChatOrchestrator tests ---
// These stubs satisfy the composite interfaces required by ChatOrchestrator
// (chatRunManager, chatTurnLifecycle) with no-op implementations.

// noopRunStatusTracker satisfies runStatusTracker.
type noopRunStatusTracker struct{}

func (noopRunStatusTracker) SetRunStatus(context.Context, string, string, string, string) error {
	return nil
}
func (noopRunStatusTracker) SetRunStatusWithAwait(context.Context, string, string, string, string, *AwaitStatusMeta) error {
	return nil
}
func (noopRunStatusTracker) PublishRunStatus(string, string, string, string) {}
func (noopRunStatusTracker) PersistRunStatus(context.Context, string, string, string, string) error {
	return nil
}
func (noopRunStatusTracker) GetRunStatus(context.Context, string) (string, string, string, string, bool) {
	return "", "", "", "", false
}
func (noopRunStatusTracker) HydrateRunStatusFromSession(context.Context, string) (persistedRunStatus, bool) {
	return persistedRunStatus{}, false
}
func (noopRunStatusTracker) StoreBinding(string, sessionRunTurnBinding) {}
func (noopRunStatusTracker) LoadBinding(string) (sessionRunTurnBinding, bool) {
	return sessionRunTurnBinding{}, false
}
func (noopRunStatusTracker) DeleteBinding(string)                        {}
func (noopRunStatusTracker) SetAwaitMetaCache(string, biz.ChatAwaitMeta) {}
func (noopRunStatusTracker) GetAwaitMetaCache(string) (biz.ChatAwaitMeta, bool) {
	return biz.ChatAwaitMeta{}, false
}
func (noopRunStatusTracker) ClearAwaitMetaCache(string) {}
func (noopRunStatusTracker) PersistAwaitMarkers(context.Context, string, string, AwaitStatusMeta, bool) {
}
func (noopRunStatusTracker) ResolveAwaitMeta(context.Context, string, string) biz.ChatAwaitMeta {
	return biz.ChatAwaitMeta{}
}
func (noopRunStatusTracker) ClearAwaitingRunStateSync(context.Context, string) error { return nil }
func (noopRunStatusTracker) ClearAwaitingRunState(context.Context, string)           {}
func (noopRunStatusTracker) Sweep()                                                  {}

// noopPendingQueueManager satisfies pendingQueueManager.
type noopPendingQueueManager struct{}

func (noopPendingQueueManager) EnqueueUserMessage(string, string) (bool, bool, string, string, error) {
	return false, false, "", "", nil
}
func (noopPendingQueueManager) CancelPendingMessage(string, string) bool { return false }
func (noopPendingQueueManager) UpdatePendingMessage(string, string, string) bool {
	return false
}
func (noopPendingQueueManager) DequeuePendingMessage(string) (biz.PendingQueueEntry, bool) {
	return biz.PendingQueueEntry{}, false
}
func (noopPendingQueueManager) GetPendingMessages(string) []biz.PendingQueueEntry { return nil }
func (noopPendingQueueManager) LastPendingMessageID(string) string                { return "" }
func (noopPendingQueueManager) SetSessionPendingMergeFollowup(string, bool)       {}
func (noopPendingQueueManager) SessionPendingMergeFollowup(string) bool           { return false }
func (noopPendingQueueManager) Sweep()                                            {}

// noopAwaitCoordinator satisfies awaitCoordinator.
type noopAwaitCoordinator struct{}

func (noopAwaitCoordinator) TryBeginResume(string) bool                    { return false }
func (noopAwaitCoordinator) EndResume(string)                              {}
func (noopAwaitCoordinator) RegisterAwaitChannel(string, biz.AwaitChannel) {}
func (noopAwaitCoordinator) DeleteAwaitChannel(string)                     {}
func (noopAwaitCoordinator) LoadAwaitChannel(string) (biz.AwaitChannel, bool) {
	return nil, false
}
func (noopAwaitCoordinator) TrySendAwaitChannel(string, biz.AwaitReplyMsg) bool { return false }
func (noopAwaitCoordinator) MakeAwaitReplyFunc(context.Context, string, string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) { return "", nil }
}
func (noopAwaitCoordinator) PublishAwaitResumed(string, string) {}
func (noopAwaitCoordinator) SessionAwaitingUser(context.Context, string) (persistedRunStatus, bool) {
	return persistedRunStatus{}, false
}
func (noopAwaitCoordinator) CanResumeAwait(context.Context, string) (string, bool) {
	return "", false
}
func (noopAwaitCoordinator) HasPendingAwaitUserReplyRoute(context.Context, string) bool {
	return false
}
func (noopAwaitCoordinator) ResumeAwaitAfterRestart(context.Context, string, string, string) error {
	return nil
}
func (noopAwaitCoordinator) Sweep() {}

// noopSessionRunLifecycle satisfies sessionRunLifecycle.
type noopSessionRunLifecycle struct{}

func (noopSessionRunLifecycle) BeginSessionRunLifecycle(ctx context.Context, _ SessionRunStartParams) (context.Context, string) {
	return ctx, ""
}
func (noopSessionRunLifecycle) FinishSessionRunLifecycle(context.Context, string, string, error) {}
func (noopSessionRunLifecycle) EscalateToDurableByUser(context.Context, string, string)          {}
func (noopSessionRunLifecycle) EscalateToDurableOnShutdown(context.Context, string, string)      {}
func (noopSessionRunLifecycle) ResolveChannelLongTaskConfig(context.Context, biz.Session) biz.ChannelLongTaskConfig {
	return biz.ChannelLongTaskConfig{}
}

// noopSessionStateTransitor satisfies sessionStateTransitor.
type noopSessionStateTransitor struct{}

func (noopSessionStateTransitor) TransitionStatus(context.Context, string, sessstatus.SessionStatus, sessstatus.SessionStatusReason) {
}

// noopTurnRecorder satisfies turnRecorder.
type noopTurnRecorder struct{}

func (noopTurnRecorder) RecordTurnUsage(context.Context, TurnUsageParams)           {}
func (noopTurnRecorder) RecordSessionTurn(context.Context, SessionTurnRecordParams) {}

// noopTurnEventPublisher satisfies turnEventPublisher.
type noopTurnEventPublisher struct{}

func (noopTurnEventPublisher) PublishTurnFailure(string, string, string, error, string) {}
func (noopTurnEventPublisher) BumpSessionRevision(context.Context, string)              {}

// newNoopChatRunManager creates a chatRunManager with all no-op sub-managers.
func newNoopChatRunManager() chatRunManager {
	return &chatRunManagerImpl{
		runStatusTracker:    noopRunStatusTracker{},
		pendingQueueManager: noopPendingQueueManager{},
		awaitCoordinator:    noopAwaitCoordinator{},
		sessionRunLifecycle: noopSessionRunLifecycle{},
	}
}

// newNoopChatTurnLifecycle creates a chatTurnLifecycle with all no-op sub-managers.
func newNoopChatTurnLifecycle() chatTurnLifecycle {
	return &chatTurnLifecycleImpl{
		sessionStateTransitor: noopSessionStateTransitor{},
		turnRecorder:          noopTurnRecorder{},
		turnEventPublisher:    noopTurnEventPublisher{},
	}
}
