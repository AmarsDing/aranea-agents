package biz

import "context"

type ManualCompressor interface {
	CompactSession(ctx context.Context, sessionID string, preserveInstruction string) (*CompactResult, error)
}

type CompactResult struct {
	Compacted            bool
	FromTurn             int
	ToTurn               int
	EstimatedTokensBefore int
	EstimatedTokensAfter  int
	CompressionLevel      string
}

// CompressStatusReader returns the current compression status for a session.
type CompressStatusReader interface {
	CompressStatus(ctx context.Context, sessionID string) (string, error)
}

// L0SnapshotForcer allows the compression pipeline to signal that the next L0 snapshot
// write should bypass throttling. Implemented by session.Runtime.
type L0SnapshotForcer interface {
	ConsumeForceL0Snapshot(sessionID string) bool
}
