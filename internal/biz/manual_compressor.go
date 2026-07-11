package biz

import "context"

type ManualCompressor interface {
	CompactSession(ctx context.Context, sessionID string, preserveInstruction string) (*CompactResult, error)
}

// ManualCompressorFromNative extracts ManualCompressor from a NativeTurnCompressor
// via type assertion. Returns nil when the implementation does not also implement
// ManualCompressor. This is used by service/team layers to wire the compact tool.
func ManualCompressorFromNative(c NativeTurnCompressor) ManualCompressor {
	if c == nil {
		return nil
	}
	if mc, ok := c.(ManualCompressor); ok {
		return mc
	}
	return nil
}

type CompactResult struct {
	Compacted             bool
	FromTurn              int
	ToTurn                int
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
