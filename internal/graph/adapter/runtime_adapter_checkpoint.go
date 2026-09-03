package adapter

import (
	"context"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// HasCheckpoint implements biz.GraphRunnerFactory：崩溃续跑安全闸（83-长时运行
// 韧性）。PG saver 未命中返回 (nil, nil)（checkpoint/postgres/saver.go），
// 故 tuple != nil 即存在；saver 为 nil（未启用持久化）视为无 checkpoint。
func (f *trpcGraphBuilderFactory) HasCheckpoint(ctx context.Context, lineageID string) (bool, error) {
	if f == nil || f.saver == nil || lineageID == "" {
		return false, nil
	}
	tuple, err := f.saver.GetTuple(ctx, trpcgraph.CreateCheckpointConfig(lineageID, "", ""))
	if err != nil {
		return false, err
	}
	return tuple != nil, nil
}
