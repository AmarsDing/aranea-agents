package tools

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// fakePoolToolSet is a test double for the pooled MCP ToolSet. It records
// Init/Close calls so tests can assert connection reuse and reclamation
// without a real MCP server.
type fakePoolToolSet struct {
	name      string
	initCalls int
	initErr   error
	closed    int
}

func (f *fakePoolToolSet) Init(context.Context) error { f.initCalls++; return f.initErr }
func (f *fakePoolToolSet) Close() error               { f.closed++; return nil }
func (f *fakePoolToolSet) Name() string               { return f.name }
func (f *fakePoolToolSet) Tools(context.Context) []trpctool.Tool {
	return nil
}

// poolFactoryRecorder counts factory invocations and hands out fake toolsets.
type poolFactoryRecorder struct {
	mu      sync.Mutex
	calls   int
	initErr error
	sets    []*fakePoolToolSet
}

func (r *poolFactoryRecorder) factory(cfg MCPServerConfig) (ToolSet, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	ts := &fakePoolToolSet{name: cfg.Name, initErr: r.initErr}
	r.sets = append(r.sets, ts)
	return ts, nil
}

func (r *poolFactoryRecorder) Calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestMCPPoolKey_OrderInsensitiveForMaps(t *testing.T) {
	a := MCPServerConfig{
		Name:      "fs",
		Transport: "stdio",
		Command:   "npx",
		Args:      []string{"-y", "server"},
		Env:       map[string]string{"A": "1", "B": "2"},
		Headers:   map[string]string{"X": "1", "Y": "2"},
	}
	b := MCPServerConfig{
		Name:      "fs",
		Transport: "stdio",
		Command:   "npx",
		Args:      []string{"-y", "server"},
		Env:       map[string]string{"B": "2", "A": "1"},
		Headers:   map[string]string{"Y": "2", "X": "1"},
	}
	ka, okA := mcpPoolKey(a)
	kb, okB := mcpPoolKey(b)
	if !okA || !okB {
		t.Fatal("expected both configs to be poolable")
	}
	if ka != kb {
		t.Fatalf("map ordering must not change the pool key: %s vs %s", ka, kb)
	}
}

func TestMCPPoolKey_DistinguishesConfigs(t *testing.T) {
	base := MCPServerConfig{Name: "fs", Transport: "stdio", Command: "npx", Args: []string{"s"}}
	baseKey, _ := mcpPoolKey(base)

	changed := base
	changed.Args = []string{"other"}
	if k, _ := mcpPoolKey(changed); k == baseKey {
		t.Fatal("args change must change the pool key")
	}

	changed = base
	changed.ToolPrefix = "pfx_"
	if k, _ := mcpPoolKey(changed); k == baseKey {
		t.Fatal("tool prefix change must change the pool key")
	}

	changed = base
	changed.Name = "other"
	if k, _ := mcpPoolKey(changed); k == baseKey {
		t.Fatal("name change must change the pool key")
	}

	changed = base
	changed.SessionReconnectMax = 3
	if k, _ := mcpPoolKey(changed); k == baseKey {
		t.Fatal("reconnect policy change must change the pool key")
	}
}

func TestMCPPoolKey_HeaderInjectorNotPoolable(t *testing.T) {
	cfg := MCPServerConfig{
		Name:      "secure",
		Transport: "sse",
		ServerURL: "http://localhost:1/mcp",
		HeaderInjector: func(context.Context) (map[string]string, error) {
			return map[string]string{"Authorization": "Bearer x"}, nil
		},
	}
	if _, ok := mcpPoolKey(cfg); ok {
		t.Fatal("configs with a per-request HeaderInjector must not be pooled (credential isolation)")
	}
}

func TestMCPToolSetPool_AcquireReusesConnection(t *testing.T) {
	rec := &poolFactoryRecorder{}
	p := newMCPToolSetPoolWithFactory(rec.factory, time.Minute)
	cfg := MCPServerConfig{Name: "fs", Transport: "stdio", Command: "npx"}

	w1, err := p.Acquire(context.Background(), cfg)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	w2, err := p.Acquire(context.Background(), cfg)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if rec.Calls() != 1 {
		t.Fatalf("factory called %d times, want 1 (connection must be reused)", rec.Calls())
	}
	if w1.(*pooledMCPToolSet).inner != w2.(*pooledMCPToolSet).inner {
		t.Fatal("both acquisitions must wrap the same underlying toolset")
	}
	// Init happens once at pool-entry creation, not per acquisition.
	if got := rec.sets[0].initCalls; got != 1 {
		t.Fatalf("Init called %d times, want 1", got)
	}
}

func TestMCPToolSetPool_ReleaseThenReapClosesConnection(t *testing.T) {
	rec := &poolFactoryRecorder{}
	p := newMCPToolSetPoolWithFactory(rec.factory, time.Minute)
	cfg := MCPServerConfig{Name: "fs", Transport: "stdio", Command: "npx"}

	w, err := p.Acquire(context.Background(), cfg)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("wrapper close: %v", err)
	}
	// Wrapper Close releases the reference but must NOT close the connection.
	if rec.sets[0].closed != 0 {
		t.Fatal("release must not close the underlying connection")
	}

	p.reapIdle(time.Now().Add(2 * time.Minute))
	if rec.sets[0].closed != 1 {
		t.Fatalf("idle entry past TTL must be closed, got closed=%d", rec.sets[0].closed)
	}

	// After reaping, a new Acquire builds a fresh connection.
	if _, err := p.Acquire(context.Background(), cfg); err != nil {
		t.Fatalf("acquire after reap: %v", err)
	}
	if rec.Calls() != 2 {
		t.Fatalf("factory called %d times after reap, want 2", rec.Calls())
	}
}

func TestMCPToolSetPool_ReapKeepsReferencedAndFreshEntries(t *testing.T) {
	rec := &poolFactoryRecorder{}
	p := newMCPToolSetPoolWithFactory(rec.factory, time.Minute)
	cfg := MCPServerConfig{Name: "fs", Transport: "stdio", Command: "npx"}

	// Entry 1: still referenced — must survive the reaper even when old.
	if _, err := p.Acquire(context.Background(), cfg); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	p.reapIdle(time.Now().Add(time.Hour))
	if rec.sets[0].closed != 0 {
		t.Fatal("referenced entry must never be reaped")
	}

	// Entry 2 (different config): released but still within idle TTL — must survive.
	cfg2 := MCPServerConfig{Name: "fs2", Transport: "stdio", Command: "npx"}
	w2, err := p.Acquire(context.Background(), cfg2)
	if err != nil {
		t.Fatalf("acquire cfg2: %v", err)
	}
	if err := w2.Close(); err != nil {
		t.Fatalf("release cfg2: %v", err)
	}
	p.reapIdle(time.Now()) // not past TTL
	if rec.sets[1].closed != 0 {
		t.Fatal("idle entry within TTL must not be reaped")
	}
}

func TestMCPToolSetPool_WrapperCloseIsIdempotent(t *testing.T) {
	rec := &poolFactoryRecorder{}
	p := newMCPToolSetPoolWithFactory(rec.factory, time.Minute)
	cfg := MCPServerConfig{Name: "fs", Transport: "stdio", Command: "npx"}

	w, err := p.Acquire(context.Background(), cfg)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	_ = w.Close()
	_ = w.Close()
	p.reapIdle(time.Now().Add(2 * time.Minute))
	if rec.sets[0].closed != 1 {
		t.Fatalf("underlying close must happen exactly once, got %d", rec.sets[0].closed)
	}
}

func TestMCPToolSetPool_InitFailureStillPooled(t *testing.T) {
	rec := &poolFactoryRecorder{initErr: fmt.Errorf("connect refused")}
	p := newMCPToolSetPoolWithFactory(rec.factory, time.Minute)
	cfg := MCPServerConfig{Name: "fs", Transport: "stdio", Command: "npx"}

	// Always-Ready semantics: init failure degrades, never fails the build.
	w, err := p.Acquire(context.Background(), cfg)
	if err != nil {
		t.Fatalf("init failure must not fail acquire: %v", err)
	}
	if w == nil {
		t.Fatal("toolset must still be returned on init failure")
	}
	if _, err := p.Acquire(context.Background(), cfg); err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	if rec.Calls() != 1 {
		t.Fatalf("failed entry must still be pooled (factory calls=%d)", rec.Calls())
	}
}

func TestMCPToolSetPool_CloseClosesAllAndIsIdempotent(t *testing.T) {
	rec := &poolFactoryRecorder{}
	p := newMCPToolSetPoolWithFactory(rec.factory, time.Minute)
	cfg := MCPServerConfig{Name: "fs", Transport: "stdio", Command: "npx"}

	// Referenced entry — pool shutdown must close it anyway.
	if _, err := p.Acquire(context.Background(), cfg); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("pool close: %v", err)
	}
	if rec.sets[0].closed != 1 {
		t.Fatalf("pool close must close all entries, got %d", rec.sets[0].closed)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("double close: %v", err)
	}
	if rec.sets[0].closed != 1 {
		t.Fatal("double close must not re-close entries")
	}

	// After shutdown the pool degrades to unpooled fresh toolsets so late
	// builds (dirty-rebuild racing shutdown) still get a working toolset.
	w, err := p.Acquire(context.Background(), cfg)
	if err != nil {
		t.Fatalf("post-close acquire: %v", err)
	}
	if _, pooled := w.(*pooledMCPToolSet); pooled {
		t.Fatal("post-close acquire must not return a pooled wrapper")
	}
	if rec.Calls() != 2 {
		t.Fatalf("post-close acquire must build fresh (factory calls=%d)", rec.Calls())
	}
}

func TestAcquireMCPToolSet_HeaderInjectorBypassesPool(t *testing.T) {
	rec := &poolFactoryRecorder{}
	p := newMCPToolSetPoolWithFactory(rec.factory, time.Minute)
	cfg := MCPServerConfig{
		Name:      "secure",
		Transport: "sse",
		ServerURL: "http://localhost:1/mcp",
		HeaderInjector: func(context.Context) (map[string]string, error) {
			return nil, nil
		},
	}
	w1, err := p.Acquire(context.Background(), cfg)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	w2, err := p.Acquire(context.Background(), cfg)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if rec.Calls() != 2 {
		t.Fatalf("credential-injected configs must build per call (factory calls=%d)", rec.Calls())
	}
	if w1 == w2 {
		t.Fatal("unpooled acquisitions must be independent instances")
	}
}
