package turntrace

import (
	"context"
	"sync"
	"testing"
)

func TestBridgeRecordToolCallEndConcurrent(t *testing.T) {
	ctx := context.Background()
	_, bridge, _ := Start(ctx, Config{Domain: DomainChat, SpanName: "test.turn", SessionID: "s1", RunID: "r1"})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bridge.RecordToolCallEnd("call_1", "search", nil)
		}()
	}
	wg.Wait()
	bridge.Finish(nil)
	bridge.Finish(nil)
}

func TestBridgeFinishUsesExecutionStatus(t *testing.T) {
	ctx := context.Background()
	_, bridge, _ := Start(ctx, Config{Domain: DomainGraph, SpanName: "graph.execute", SessionID: "s", RunID: "e1"})
	bridge.Finish(context.Canceled)
}
