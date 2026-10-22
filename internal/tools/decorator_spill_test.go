package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/toolspill"

	"aranea-agents/pkg/loggateway"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// ---------------------------------------------------------------------------
// P0-2 spill branch of ToolDecorator.truncateResult (design §3.4 / §3.8).
// OFF (Spill deps absent) must stay byte-identical with the classic
// truncation envelope; ON must spill the full payload and inject a locator
// envelope; spill failure must preserve the content with an explicit
// enforcement=partial note (never silently truncated).
// ---------------------------------------------------------------------------

func newSpillStoreForTest(t *testing.T) *toolspill.LocalFS {
	t.Helper()
	store, err := toolspill.NewLocalFS(toolspill.Options{
		WorkspaceRoot: t.TempDir(),
		Logger:        loggateway.NewNoop(),
	})
	if err != nil {
		t.Fatalf("NewLocalFS: %v", err)
	}
	return store
}

// invocationCtxWithSession builds an invocation context carrying a session
// but NO artifact service, so the decorator's artifact offload is unavailable
// and the spill branch is what gets exercised.
func invocationCtxWithSession(appName, userID, sessionID string) context.Context {
	inv := &trpcagent.Invocation{
		Session: &trpcsession.Session{AppName: appName, UserID: userID, ID: sessionID},
	}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

// failingSpillStore always errors on Put (still satisfies toolspill.Store).
type failingSpillStore struct{ err error }

func (f failingSpillStore) Put(context.Context, toolspill.Record) (toolspill.Locator, error) {
	if f.err != nil {
		return toolspill.Locator{}, f.err
	}
	return toolspill.Locator{}, errors.New("disk on fire")
}
func (f failingSpillStore) Get(context.Context, string, string) (toolspill.Record, error) {
	return toolspill.Record{}, toolspill.ErrNotFound
}
func (f failingSpillStore) Stat(context.Context, string, string) (toolspill.FileInfo, error) {
	return toolspill.FileInfo{}, toolspill.ErrNotFound
}
func (f failingSpillStore) GC(context.Context, time.Time) (toolspill.GCStats, error) {
	return toolspill.GCStats{}, nil
}

func TestToolDecorator_SpillDisabledKeepsLegacyTruncation(t *testing.T) {
	big := strings.Repeat("x", 30*1024)
	tool := &decoratorMockTool{
		name: "web_search",
		call: func(_ context.Context, _ []byte) (any, error) {
			return map[string]any{"data": big}, nil
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		Logger:       loggateway.NewNoop(),
		ResultBudget: &ResultBudget{MaxBytes: 1024, Mode: "tail"},
	})
	res, err := d.Call(context.Background(), []byte(`{"q":"aranea"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	env, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want envelope map", res)
	}
	if truncated, _ := env["truncated"].(bool); !truncated {
		t.Fatalf("legacy path must produce truncated=true, got %v", env)
	}
	if _, spilled := env["spilled"]; spilled {
		t.Fatalf("OFF gate must never mark spilled: %v", env)
	}
	if mode, _ := env["mode"].(string); mode != "tail" {
		t.Fatalf("legacy mode = %q, want tail", mode)
	}
	data, _ := json.Marshal(map[string]any{"data": big})
	wantContent := string(sliceForMode(data, 1024-truncationEnvelopeOverhead, "tail"))
	if env["content"] != wantContent {
		t.Fatalf("legacy content mismatch: got %d bytes, want %d bytes (byte-identical regression)", len(env["content"].(string)), len(wantContent))
	}
}

func TestToolDecorator_SpillsOversizedResultWhenOffloadUnavailable(t *testing.T) {
	// >20KB payload: exercises the batch-A acceptance "ON 时 20KB 输出走 spill
	// 且 locator 可回读".
	big := strings.Repeat("data-块-", 3*1024) // ~24KB of multi-byte content
	tool := &decoratorMockTool{
		name: "web_search",
		call: func(_ context.Context, _ []byte) (any, error) {
			return map[string]any{"data": big}, nil
		},
	}
	store := newSpillStoreForTest(t)
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		Logger:       loggateway.NewNoop(),
		ResultBudget: &ResultBudget{MaxBytes: 1024, Mode: "tail"},
		Spill:        &toolspill.Deps{Store: store},
	})
	ctx := invocationCtxWithSession("agent-a", "user-1", "sess-1")
	res, err := d.Call(ctx, []byte(`{"q":"aranea"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	env, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want envelope map", res)
	}
	if spilled, _ := env["spilled"].(bool); !spilled {
		t.Fatalf("expected spilled=true envelope, got %v", env)
	}
	locatorStr, _ := env["locator"].(string)
	loc, err := toolspill.ParseLocator(locatorStr)
	if err != nil {
		t.Fatalf("locator must parse: %v (%q)", err, locatorStr)
	}
	if loc.Store != toolspill.StoreFS {
		t.Fatalf("locator store = %q, want %q", loc.Store, toolspill.StoreFS)
	}
	if hint, _ := env["read_hint"].(string); !strings.Contains(hint, "read_tool_result") || !strings.Contains(hint, locatorStr) {
		t.Fatalf("read_hint must direct the model via read_tool_result with the locator, got %q", hint)
	}
	// Dual-end previews mirror the offload envelope contract.
	if head, _ := env["preview_head"].(string); len(head) == 0 {
		t.Fatal("preview_head must be populated")
	}
	// Byte-faithful round trip through the store.
	rec, err := store.Get(context.Background(), "sess-1", locatorStr)
	if err != nil {
		t.Fatalf("Get spilled record: %v", err)
	}
	wantData, _ := json.Marshal(map[string]any{"data": big})
	if string(rec.Content) != string(wantData) {
		t.Fatalf("spilled content mismatch: got %d bytes, want %d bytes", len(rec.Content), len(wantData))
	}
	if rec.Tool != "web_search" {
		t.Fatalf("spilled tool = %q, want web_search", rec.Tool)
	}
}

func TestToolDecorator_OffloadPreferredOverSpill(t *testing.T) {
	big := strings.Repeat("x", 30*1024)
	svc := newMockArtifactService()
	tool := &decoratorMockTool{
		name: "web_search",
		call: func(_ context.Context, _ []byte) (any, error) {
			return map[string]any{"data": big}, nil
		},
	}
	spy := &countingSpillStore{}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		Logger:       loggateway.NewNoop(),
		ResultBudget: &ResultBudget{MaxBytes: 1024, Mode: "tail"},
		Spill:        &toolspill.Deps{Store: spy},
	})
	ctx := invocationCtxWithArtifact("agent-a", "user-1", "sess-1", svc)
	res, err := d.Call(ctx, []byte(`{"q":"aranea"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	env, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want envelope map", res)
	}
	if offloaded, _ := env["offloaded"].(bool); !offloaded {
		t.Fatalf("artifact offload must remain the primary path, got %v", env)
	}
	if spy.puts != 0 {
		t.Fatalf("spill store must be untouched when offload succeeds, got %d puts", spy.puts)
	}
}

func TestToolDecorator_SpillFailurePreservesFullContent(t *testing.T) {
	big := strings.Repeat("keep-me-", 4*1024) // 32KB
	tool := &decoratorMockTool{
		name: "web_search",
		call: func(_ context.Context, _ []byte) (any, error) {
			return map[string]any{"data": big}, nil
		},
	}
	var dedupeKey string
	deps := &toolspill.Deps{
		Store: failingSpillStore{},
		Hooks: toolspill.Hooks{OnSpillFailed: func(_ context.Context, key string, _ error) { dedupeKey = key }},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		Logger:       loggateway.NewNoop(),
		ResultBudget: &ResultBudget{MaxBytes: 1024, Mode: "tail"},
		Spill:        deps,
	})
	ctx := invocationCtxWithSession("agent-a", "user-1", "sess-1")
	res, err := d.Call(ctx, []byte(`{"q":"aranea"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	env, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want envelope map", res)
	}
	if failed, _ := env["spill_failed"].(bool); !failed {
		t.Fatalf("expected spill_failed=true envelope, got %v", env)
	}
	if truncated, _ := env["truncated"].(bool); truncated {
		t.Fatal("fail-soft must NOT mark the content as truncated (honest partial enforcement)")
	}
	wantData, _ := json.Marshal(map[string]any{"data": big})
	content, _ := env["content"].(string)
	if content != string(wantData)+toolspill.PartialPreservedNote {
		t.Fatalf("content = first 64 bytes %q..., want full payload + partial note", content[:64])
	}
	if !strings.Contains(content, "enforcement=partial") {
		t.Fatal("partial note must carry the enforcement=partial marker")
	}
	if dedupeKey == "" || !strings.HasPrefix(dedupeKey, "spill_failed:sess-1:") {
		t.Fatalf("Important-level spill_failed hook must fire with session-scoped dedupe key, got %q", dedupeKey)
	}
}

func TestToolDecorator_SpillWithoutSessionFallsBackToLegacy(t *testing.T) {
	big := strings.Repeat("x", 30*1024)
	tool := &decoratorMockTool{
		name: "web_search",
		call: func(_ context.Context, _ []byte) (any, error) {
			return map[string]any{"data": big}, nil
		},
	}
	spy := &countingSpillStore{}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		Logger:       loggateway.NewNoop(),
		ResultBudget: &ResultBudget{MaxBytes: 1024, Mode: "tail"},
		Spill:        &toolspill.Deps{Store: spy},
	})
	res, err := d.Call(context.Background(), []byte(`{"q":"aranea"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	env, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want envelope map", res)
	}
	if truncated, _ := env["truncated"].(bool); !truncated {
		t.Fatalf("sessionless call must keep the legacy truncation envelope, got %v", env)
	}
	if spy.puts != 0 {
		t.Fatalf("spill records are session-scoped; sessionless calls must not spill, got %d puts", spy.puts)
	}
}

type countingSpillStore struct{ puts int }

func (c *countingSpillStore) Put(context.Context, toolspill.Record) (toolspill.Locator, error) {
	c.puts++
	return toolspill.Locator{Store: toolspill.StoreFS, ID: "x/y.spill.json"}, nil
}
func (c *countingSpillStore) Get(context.Context, string, string) (toolspill.Record, error) {
	return toolspill.Record{}, toolspill.ErrNotFound
}
func (c *countingSpillStore) Stat(context.Context, string, string) (toolspill.FileInfo, error) {
	return toolspill.FileInfo{}, toolspill.ErrNotFound
}
func (c *countingSpillStore) GC(context.Context, time.Time) (toolspill.GCStats, error) {
	return toolspill.GCStats{}, nil
}
