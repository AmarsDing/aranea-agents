package agent

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestShardHoldToolSet(t *testing.T) {
	var released atomic.Int32
	h := newShardHoldToolSet("mcp:srv", func() { released.Add(1) })
	if h.Name() != "shard_hold:mcp:srv" {
		t.Errorf("占位符命名不符：%s", h.Name())
	}
	if got := h.Tools(context.Background()); got != nil {
		t.Error("占位符 Tools 必须恒为 nil（不参与工具面）")
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h.Close()
	if released.Load() != 1 {
		t.Error("占位符 Close 应幂等释放一次分片引用")
	}
}

func BenchmarkShardCacheAcquireHit(b *testing.B) {
	c := newShardCache(8)
	spec := shardSpecFor("fpBench", true, &shardProduct{}, nil)
	ctx := context.Background()

	_, rel, _ := c.acquire(ctx, spec)
	rel()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, release, err := c.acquire(ctx, spec)
		if err != nil {
			b.Fatal(err)
		}
		release()
	}
}

func BenchmarkShardFingerprint(b *testing.B) {
	proj := mcpServerShardFP{
		Name:      "srv",
		Transport: "streamable_http",
		ServerURL: "http://localhost:8930/mcp",
		Args:      []string{"-y", "pkg"},
		Env:       map[string]string{"K1": "V1", "K2": "V2"},
		Headers:   map[string]string{"Authorization": "Bearer k"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = shardFingerprint(shardGroupMCP, proj)
	}
}
