package biz

import "context"

// DurableTurnCompressor runs synchronous context compaction before durable resume (CH-BOR-14).
type DurableTurnCompressor interface {
	BeforeDurableTurn(ctx context.Context, sessionID string, agent Agent) error
}
