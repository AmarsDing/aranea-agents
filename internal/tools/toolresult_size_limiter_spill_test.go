package tools

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/toolspill"

	"aranea-agents/pkg/loggateway"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ---------------------------------------------------------------------------
// P0-2 spill branch of the AfterTool output size limiter (design §3.4/§3.8).
// Default OFF (no deps) keeps the legacy hard-truncation envelope
// byte-identical — that is pinned by the pre-existing
// TestNewOutputSizeLimiterHook_* family, which passes zero spill deps.
// ---------------------------------------------------------------------------

func TestOutputSizeLimiter_SpillsOversizedString(t *testing.T) {
	store := newSpillStoreForTest(t)
	deps := &toolspill.Deps{Store: store}
	hook := NewOutputSizeLimiterHook(100, loggateway.NewNoop(), deps)

	payload := strings.Repeat("chunk-数据-", 40) // 400 runes > 100 limit
	args := &trpctool.AfterToolArgs{ToolCallID: "call_1", ToolName: "mcp_tool", Result: payload}
	ctx := invocationCtxWithSession("agent-a", "user-1", "sess-limit")
	res, err := hook(ctx, args)
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	env, ok := res.CustomResult.(map[string]any)
	if !ok {
		t.Fatalf("expected spill envelope, got %T", res.CustomResult)
	}
	if spilled, _ := env["spilled"].(bool); !spilled {
		t.Fatalf("expected spilled=true, got %v", env)
	}
	if mode, _ := env["mode"].(string); mode != "spill" {
		t.Fatalf("mode = %q, want spill", mode)
	}
	locatorStr, _ := env["locator"].(string)
	loc, err := toolspill.ParseLocator(locatorStr)
	if err != nil {
		t.Fatalf("locator must parse: %v", err)
	}
	if loc.Store != toolspill.StoreFS {
		t.Fatalf("store = %q, want fs", loc.Store)
	}
	// The envelope content is the deterministic locator pointer, not a lossy
	// prefix of the payload.
	wantPointer := toolspill.PointerText("mcp_tool", len(payload), loc)
	if env["content"] != wantPointer {
		t.Fatalf("content = %q, want pointer %q", env["content"], wantPointer)
	}
	// Full payload retrievable through the store (>=20KB acceptance applied
	// at the decorator integration; here the contract is byte-faithfulness).
	rec, err := store.Get(context.Background(), "sess-limit", locatorStr)
	if err != nil {
		t.Fatalf("Get spilled record: %v", err)
	}
	if string(rec.Content) != payload {
		t.Fatalf("spilled content mismatch: got %d bytes, want %d", len(rec.Content), len(payload))
	}
}

func TestOutputSizeLimiter_SpillFailurePreservesContent(t *testing.T) {
	var dedupeKey string
	deps := &toolspill.Deps{
		Store: failingSpillStore{},
		Hooks: toolspill.Hooks{OnSpillFailed: func(_ context.Context, key string, _ error) { dedupeKey = key }},
	}
	hook := NewOutputSizeLimiterHook(100, loggateway.NewNoop(), deps)

	payload := strings.Repeat("z", 250)
	args := &trpctool.AfterToolArgs{ToolCallID: "call_1", ToolName: "mcp_tool", Result: payload}
	ctx := invocationCtxWithSession("agent-a", "user-1", "sess-limit")
	res, err := hook(ctx, args)
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	env, ok := res.CustomResult.(map[string]any)
	if !ok {
		t.Fatalf("expected fail-soft envelope, got %T", res.CustomResult)
	}
	if failed, _ := env["spill_failed"].(bool); !failed {
		t.Fatalf("expected spill_failed=true, got %v", env)
	}
	if truncated, _ := env["truncated"].(bool); truncated {
		t.Fatal("fail-soft must not mark the payload truncated")
	}
	if env["content"] != payload+toolspill.PartialPreservedNote {
		t.Fatal("content must be the full payload plus the enforcement=partial note")
	}
	if dedupeKey == "" {
		t.Fatal("Important-level spill_failed hook must fire")
	}
}

func TestOutputSizeLimiter_SpillWithoutSessionKeepsLegacyTruncation(t *testing.T) {
	spy := &countingSpillStore{}
	hook := NewOutputSizeLimiterHook(100, loggateway.NewNoop(), &toolspill.Deps{Store: spy})

	payload := strings.Repeat("a", 200)
	args := &trpctool.AfterToolArgs{ToolCallID: "call_1", ToolName: "mcp_tool", Result: payload}
	res, err := hook(context.Background(), args) // no invocation -> no session
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	env, ok := res.CustomResult.(map[string]any)
	if !ok {
		t.Fatalf("expected legacy truncation envelope, got %T", res.CustomResult)
	}
	if truncated, _ := env["truncated"].(bool); !truncated {
		t.Fatalf("legacy path must produce truncated=true, got %v", env)
	}
	if mode, _ := env["mode"].(string); mode != "head" {
		t.Fatalf("legacy mode = %q, want head", mode)
	}
	if content, _ := env["content"].(string); content != strings.Repeat("a", 100) {
		t.Fatalf("legacy content mismatch: got %d runes", len([]rune(content)))
	}
	if spy.puts != 0 {
		t.Fatalf("sessionless results must not spill, got %d puts", spy.puts)
	}
}

func TestOutputSizeLimiter_SkipsSpillEnvelopes(t *testing.T) {
	hook := NewOutputSizeLimiterHook(10, loggateway.NewNoop())

	// A decorator-produced spill envelope must pass through untouched even
	// though its map shape is not a string (guard the isSizeGovernedResult
	// extension explicitly).
	args := &trpctool.AfterToolArgs{
		ToolName: "web_search",
		Result:   map[string]any{"spilled": true, "locator": "spill://fs/ab/cd.spill.json"},
	}
	res, err := hook(context.Background(), args)
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if res.CustomResult != nil {
		t.Fatalf("spilled envelope must be left alone, got %v", res.CustomResult)
	}

	args = &trpctool.AfterToolArgs{
		ToolName: "web_search",
		Result:   map[string]any{"spill_failed": true, "content": strings.Repeat("y", 5000)},
	}
	res, err = hook(context.Background(), args)
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if res.CustomResult != nil {
		t.Fatalf("spill_failed envelope (deliberately preserved payload) must be left alone, got %v", res.CustomResult)
	}
}
