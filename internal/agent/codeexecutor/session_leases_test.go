package codeexecutor

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/sandbox"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcagentcodeexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// fakeSandboxEngine is an in-memory sandbox.Engine for adapter tests.
type fakeSandboxEngine struct {
	mu        sync.Mutex
	created   []string
	destroyed []string
	execFunc  func(h sandbox.Handle, spec sandbox.ExecSpec) (sandbox.ExecResult, error)
}

func (f *fakeSandboxEngine) Create(ctx context.Context, id string, p sandbox.Profile, labels map[string]string) (sandbox.Handle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, id)
	return sandbox.Handle{ID: id, SandboxID: id}, nil
}

func (f *fakeSandboxEngine) Exec(ctx context.Context, h sandbox.Handle, spec sandbox.ExecSpec) (sandbox.ExecResult, error) {
	if f.execFunc != nil {
		return f.execFunc(h, spec)
	}
	return sandbox.ExecResult{Stdout: "ok"}, nil
}

func (f *fakeSandboxEngine) CopyTo(ctx context.Context, h sandbox.Handle, path string, r io.Reader) error {
	return nil
}

func (f *fakeSandboxEngine) CopyFrom(ctx context.Context, h sandbox.Handle, path string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (f *fakeSandboxEngine) Destroy(ctx context.Context, h sandbox.Handle) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.destroyed = append(f.destroyed, h.SandboxID)
	return nil
}

func (f *fakeSandboxEngine) ListByLabels(ctx context.Context, labels map[string]string) ([]sandbox.Handle, error) {
	return nil, nil
}

func (f *fakeSandboxEngine) createdCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.created)
}

func (f *fakeSandboxEngine) destroyedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.destroyed)
}

func newSessionTestAdapter(t *testing.T, eng *fakeSandboxEngine) (*pooledAdapter, *sandbox.Manager) {
	t.Helper()
	cfg := sandbox.DefaultConfig()
	cfg.Pool.MinReady = 0
	mgr := sandbox.NewManager(cfg, eng, nil)
	t.Cleanup(mgr.Close)
	return newPooledAdapter(mgr, 10*time.Second, sandbox.NewSessionLeases(mgr)), mgr
}

func pythonInput(executionID, code string) trpcagentcodeexec.CodeExecutionInput {
	return trpcagentcodeexec.CodeExecutionInput{
		ExecutionID: executionID,
		CodeBlocks:  []trpcagentcodeexec.CodeBlock{{Language: "python", Code: code}},
	}
}

// A3: same ExecutionID reuses one sandbox across calls; a different session
// gets its own sandbox; an ephemeral call (empty ID) creates+destroys each time.
func TestPooledAdapterSessionStickiness(t *testing.T) {
	eng := &fakeSandboxEngine{}
	adapter, _ := newSessionTestAdapter(t, eng)
	ctx := context.Background()

	if _, err := adapter.ExecuteCode(ctx, pythonInput("app/u/s1", "print(1)")); err != nil {
		t.Fatalf("call 1: %v", err)
	}
	if _, err := adapter.ExecuteCode(ctx, pythonInput("app/u/s1", "print(2)")); err != nil {
		t.Fatalf("call 2: %v", err)
	}
	if n := eng.createdCount(); n != 1 {
		t.Fatalf("session reuse: created=%d, want 1", n)
	}
	if n := eng.destroyedCount(); n != 0 {
		t.Fatalf("session lease must stay alive between calls: destroyed=%d", n)
	}

	if _, err := adapter.ExecuteCode(ctx, pythonInput("app/u/s2", "print(3)")); err != nil {
		t.Fatalf("call 3: %v", err)
	}
	if n := eng.createdCount(); n != 2 {
		t.Fatalf("second session: created=%d, want 2", n)
	}

	if _, err := adapter.ExecuteCode(ctx, pythonInput("", "print(4)")); err != nil {
		t.Fatalf("ephemeral: %v", err)
	}
	if n := eng.createdCount(); n != 3 {
		t.Fatalf("ephemeral: created=%d, want 3", n)
	}
	if n := eng.destroyedCount(); n != 1 {
		t.Fatalf("ephemeral must destroy on return: destroyed=%d, want 1", n)
	}
}

// Manager-side idle/TTL destroy makes the pinned lease stale; the next call
// evicts it and retries once on a fresh sandbox.
func TestPooledAdapterStaleLeaseRetry(t *testing.T) {
	eng := &fakeSandboxEngine{}
	adapter, _ := newSessionTestAdapter(t, eng)
	ctx := context.Background()

	if _, err := adapter.ExecuteCode(ctx, pythonInput("app/u/s1", "print(1)")); err != nil {
		t.Fatalf("call 1: %v", err)
	}
	// Simulate the manager-side idle/TTL GC having destroyed the pinned
	// sandbox: the next exec on the stale lease fails with ErrNotFound, the
	// store evicts it, and the retried call succeeds on a fresh sandbox.
	call := 0
	eng.execFunc = func(h sandbox.Handle, spec sandbox.ExecSpec) (sandbox.ExecResult, error) {
		call++
		if call == 1 {
			return sandbox.ExecResult{}, sandbox.ErrNotFound
		}
		return sandbox.ExecResult{Stdout: "ok"}, nil
	}
	if _, err := adapter.ExecuteCode(ctx, pythonInput("app/u/s1", "print(2)")); err != nil {
		t.Fatalf("stale retry: %v", err)
	}
	if n := eng.createdCount(); n != 2 {
		t.Fatalf("stale retry must acquire a fresh sandbox: created=%d, want 2", n)
	}
}

// P1-2: empty model-supplied ExecutionID falls back to the invocation session
// (app/user/session); an explicit ExecutionID still wins over the invocation.
func TestPooledAdapterInvocationSessionKeyFallback(t *testing.T) {
	eng := &fakeSandboxEngine{}
	adapter, _ := newSessionTestAdapter(t, eng)

	inv := trpcagent.NewInvocation()
	inv.Session = &trpcsession.Session{ID: "s1", AppName: "app", UserID: "u"}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	// Two calls without execution_id share the invocation-derived sandbox.
	if _, err := adapter.ExecuteCode(ctx, pythonInput("", "print(1)")); err != nil {
		t.Fatalf("call 1: %v", err)
	}
	if _, err := adapter.ExecuteCode(ctx, pythonInput("", "print(2)")); err != nil {
		t.Fatalf("call 2: %v", err)
	}
	if n := eng.createdCount(); n != 1 {
		t.Fatalf("invocation-keyed reuse: created=%d, want 1", n)
	}

	// Explicit execution_id wins over the invocation session.
	if _, err := adapter.ExecuteCode(ctx, pythonInput("other/u/x9", "print(3)")); err != nil {
		t.Fatalf("call 3: %v", err)
	}
	if n := eng.createdCount(); n != 2 {
		t.Fatalf("explicit execution_id must get its own sandbox: created=%d, want 2", n)
	}

	// No execution_id and no invocation → ephemeral (create+destroy per call).
	if _, err := adapter.ExecuteCode(context.Background(), pythonInput("", "print(4)")); err != nil {
		t.Fatalf("ephemeral: %v", err)
	}
	if n := eng.createdCount(); n != 3 {
		t.Fatalf("ephemeral: created=%d, want 3", n)
	}
	if n := eng.destroyedCount(); n != 1 {
		t.Fatalf("ephemeral must destroy on return: destroyed=%d, want 1", n)
	}
}

// ReleaseSession destroys the pinned lease (explicit session teardown hook).
func TestSessionLeaseStoreReleaseSession(t *testing.T) {
	eng := &fakeSandboxEngine{}
	adapter, _ := newSessionTestAdapter(t, eng)
	ctx := context.Background()

	if _, err := adapter.ExecuteCode(ctx, pythonInput("app/u/s1", "print(1)")); err != nil {
		t.Fatalf("call 1: %v", err)
	}
	adapter.store.ReleaseSession(ctx, "app/u/s1")
	if n := eng.destroyedCount(); n != 1 {
		t.Fatalf("releaseSession: destroyed=%d, want 1", n)
	}
}
