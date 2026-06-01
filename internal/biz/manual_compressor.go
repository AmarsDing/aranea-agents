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
