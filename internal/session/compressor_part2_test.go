package session

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
	"aranea-agents/pkg/loggateway"
	"context"
	"testing"
	"time"
)

func TestLLMCompress_CacheHitSetsOutcomeFlag(t *testing.T) {
	read := &stubCompressReadDeps{}
	fake := &scriptedLLMCompressor{results: []compress.Result{{Markdown: "LLM 合并摘要", Provider: "p", Model: "m"}}}
	cache := compress.NewCompressCache(16, 10*time.Minute, loggateway.NewNoop())
	c := &Compressor{
		deps:     compressDeps{summaryReader: read},
		Compress: compress.NewCachingCompressor(fake, cache, loggateway.NewNoop()),
		lg:       loggateway.NewNoop(),
	}
	ctx := compress.ContextWithSessionID(context.Background(), "sess-1")
	body := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: "q6", TurnNumber: 6},
		{Role: "assistant", ContentMarkdown: "a6", TurnNumber: 6},
	}
	first := c.llmCompress(ctx, biz.Session{}, biz.Agent{}, body, nil, "sess-1")
	if first.level != compressLevelLLM || first.cacheHit {
		t.Fatalf("首次应 miss: level=%v cacheHit=%v", first.level, first.cacheHit)
	}
	if fake.calls != 1 {
		t.Fatalf("calls = %d, want 1", fake.calls)
	}
	second := c.llmCompress(ctx, biz.Session{}, biz.Agent{}, body, nil, "sess-1")
	if second.level != compressLevelLLM || !second.cacheHit {
		t.Fatalf("第二次应命中: level=%v cacheHit=%v", second.level, second.cacheHit)
	}
	if fake.calls != 1 {
		t.Fatalf("命中不应再调 LLM: calls = %d, want 1", fake.calls)
	}
}

func TestExecuteCompression_CacheHitInNoticeMetadata(t *testing.T) {
	read := &stubCompressReadDeps{}
	write := &stubCompressWriteDeps{}
	bus := &stubMonitorBus{}
	c := &Compressor{
		deps: compressDeps{
			sessionReader: read, messageReader: read, summaryReader: read,
			summaryWriter: write, messageWriter: write, contextUpdater: write,
			compressRepo: &stubCompressTxDeps{version: 0},
		},
		monitorBus: bus,
		lg:         loggateway.NewNoop(),
	}
	sess := biz.Session{ID: "sess-1", CompressVersion: 0, RunnerSnapshotJSON: `{"state":{}}`}
	outcome := compressOutcome{level: compressLevelLLM, markdown: "合并摘要", cacheHit: true}
	body := []biz.ChatMessage{{Role: "user", ContentMarkdown: "q", TurnNumber: 6}}
	wrote, err := c.executeCompression(context.Background(), sess, biz.Agent{}, nil, body, nil, outcome, "sess-1", "u")
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("成功写入必须上报 wrote=true")
	}
	if len(bus.events) != 1 {
		t.Fatalf("应发布 1 条压缩通知事件, got %d", len(bus.events))
	}
	hit, ok := bus.events[0].Metadata["cache_hit"].(bool)
	if !ok || !hit {
		t.Fatalf("metadata cache_hit 应为 true: %v", bus.events[0].Metadata)
	}
}
