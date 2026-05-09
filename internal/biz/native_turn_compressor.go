package biz

import "context"

// NativeTurnCompressor triggers asynchronous session context compression after a native chat turn.
// Implemented by service.SessionCompressor; optional on team.Runner.
type NativeTurnCompressor interface {
	AfterNativeTurn(ctx context.Context, sessionID string, agent Agent)
}
