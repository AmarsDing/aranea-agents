package tools

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/tools/browser"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// fakeMCPToolSet is a minimal ToolSet tracking Close calls.
type fakeMCPToolSet struct {
	name   string
	closed atomic.Int32
}

func (f *fakeMCPToolSet) Name() string { return f.name }

func (f *fakeMCPToolSet) Tools(context.Context) []trpctool.Tool { return nil }

func (f *fakeMCPToolSet) Close() error {
	f.closed.Add(1)
	return nil
}

// Close 时被引用的 entry 不得立即关闭（shutdown use-after-close），
// 须延迟到最后一次 release。
func TestMCPToolSetPool_CloseDefersReferencedEntry(t *testing.T) {
	fake := &fakeMCPToolSet{name: "srv"}
	pool := newMCPToolSetPoolWithFactory(func(MCPServerConfig) (ToolSet, error) { return fake, nil }, time.Hour)
	defer pool.Close()

	ts, err := pool.Acquire(context.Background(), MCPServerConfig{Name: "srv", Transport: "stdio", Command: "false"})
	if err != nil || ts == nil {
		t.Fatalf("Acquire: ts=%v err=%v", ts, err)
	}

	if err := pool.Close(); err != nil {
		t.Fatalf("pool.Close: %v", err)
	}
	if got := fake.closed.Load(); got != 0 {
		t.Fatalf("Close() closed a still-referenced entry: closed=%d, want 0", got)
	}

	if err := ts.Close(); err != nil {
		t.Fatalf("wrapper Close: %v", err)
	}
	if got := fake.closed.Load(); got != 1 {
		t.Fatalf("release after pool Close did not close entry: closed=%d, want 1", got)
	}
}

// 空闲 entry（refs==0）在 Close 时保持立即关闭的现有行为。
func TestMCPToolSetPool_CloseClosesIdleEntryImmediately(t *testing.T) {
	fake := &fakeMCPToolSet{name: "srv"}
	pool := newMCPToolSetPoolWithFactory(func(MCPServerConfig) (ToolSet, error) { return fake, nil }, time.Hour)

	ts, err := pool.Acquire(context.Background(), MCPServerConfig{Name: "srv", Transport: "stdio", Command: "false"})
	if err != nil || ts == nil {
		t.Fatalf("Acquire: ts=%v err=%v", ts, err)
	}
	if err := ts.Close(); err != nil {
		t.Fatalf("wrapper Close: %v", err)
	}
	if err := pool.Close(); err != nil {
		t.Fatalf("pool.Close: %v", err)
	}
	if got := fake.closed.Load(); got != 1 {
		t.Fatalf("idle entry not closed on pool Close: closed=%d, want 1", got)
	}
}

// 建连期间池被并发 Close：返回给调用方的 ToolSet 不得是已关闭的
//（调用方持有所有权，与不可池化路径一致）。
func TestMCPToolSetPool_AcquireDuringCloseReturnsUsableToolSet(t *testing.T) {
	entered := make(chan struct{})
	gate := make(chan struct{})
	fake := &fakeMCPToolSet{name: "srv"}
	pool := newMCPToolSetPoolWithFactory(func(MCPServerConfig) (ToolSet, error) {
		close(entered)
		<-gate
		return fake, nil
	}, time.Hour)

	type res struct {
		ts  ToolSet
		err error
	}
	resCh := make(chan res, 1)
	go func() {
		ts, err := pool.Acquire(context.Background(), MCPServerConfig{Name: "srv", Transport: "stdio", Command: "false"})
		resCh <- res{ts, err}
	}()

	<-entered // factory 已进入，Acquire 在锁外建连
	if err := pool.Close(); err != nil {
		t.Fatalf("pool.Close: %v", err)
	}
	close(gate) // 放行 factory

	r := <-resCh
	if r.err != nil || r.ts == nil {
		t.Fatalf("Acquire: ts=%v err=%v", r.ts, r.err)
	}
	if got := fake.closed.Load(); got != 0 {
		t.Fatalf("Acquire returned an already-closed ToolSet: closed=%d, want 0", got)
	}
	if err := r.ts.Close(); err != nil {
		t.Fatalf("caller Close: %v", err)
	}
}

// Assemble 错误路径必须释放已装配的池化 MCP ToolSet（引用计数归零，
// reaper 可回收），否则连接永久泄漏。
func TestAssemble_ErrorPathReleasesPooledMCPToolSets(t *testing.T) {
	fake := &fakeMCPToolSet{name: "pooled-srv"}
	testPool := newMCPToolSetPoolWithFactory(func(cfg MCPServerConfig) (ToolSet, error) {
		if cfg.Name == "browser" {
			return nil, errors.New("boom")
		}
		return fake, nil
	}, time.Hour)
	defer testPool.Close()

	saved := globalMCPToolSetPool
	globalMCPToolSetPool = testPool
	defer func() { globalMCPToolSetPool = saved }()

	_, err := Assemble(context.Background(), AssemblyConfig{
		EnabledTools: []string{"browser"},
		MCP: MCPConfig{Servers: []MCPServerConfig{
			{Name: "pooled-srv", Transport: "stdio", Command: "false"},
		}},
		Browser: &browser.PlaywrightMCPConfig{Transport: "stdio", Command: "false"},
	})
	if err == nil {
		t.Fatal("expected Assemble error from failing browser phase")
	}

	// 释放后引用应归零，空闲回收可真正关闭底层连接。
	testPool.reapIdle(time.Now().Add(2 * time.Hour))
	if got := fake.closed.Load(); got != 1 {
		t.Fatalf("pooled MCP ToolSet reference leaked: closed=%d after reap, want 1", got)
	}
}
