package biz

import "context"

// RunnerSnapshotSync mirrors Ent session state into trpc session.Service.
type RunnerSnapshotSync interface {
	SyncRunnerSnapshot(ctx context.Context, userID, sessionID, snapshotJSON, summaryMarkdown string) error
	SyncStateDelta(ctx context.Context, userID, sessionID, operation, path, valueJSON string) error
}
