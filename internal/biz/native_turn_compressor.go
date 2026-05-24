package biz

import "context"

// NativeTurnCompressor triggers asynchronous session context compression after a native chat turn.
// Implementations may also expose BeforeDurableTurn for synchronous pre-resume compaction (CH-BOR-14).
type NativeTurnCompressor interface {
	AfterNativeTurn(ctx context.Context, sessionID string, agent Agent)
}
