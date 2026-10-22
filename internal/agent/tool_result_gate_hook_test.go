package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/toolspill"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

func TestEstimateTurnNumber(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleUser},
		{Role: trpcmodel.RoleAssistant},
		{Role: trpcmodel.RoleUser},
		{Role: trpcmodel.RoleTool},
		{Role: trpcmodel.RoleAssistant},
		{Role: trpcmodel.RoleUser},
	}
	n := estimateTurnNumber(msgs)
	if n != 3 {
		t.Fatalf("estimateTurnNumber = %d, want 3", n)
	}
}

func TestEstimateTurnNumber_NoUserMessages(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleAssistant},
		{Role: trpcmodel.RoleTool},
	}
	n := estimateTurnNumber(msgs)
	if n != 0 {
		t.Fatalf("estimateTurnNumber = %d, want 0", n)
	}
}

func TestEstimateTurnNumber_Empty(t *testing.T) {
	n := estimateTurnNumber(nil)
	if n != 0 {
		t.Fatalf("estimateTurnNumber = %d, want 0", n)
	}
}

func TestExtractTextContent_PlainContent(t *testing.T) {
	msg := &trpcmodel.Message{Content: "hello world"}
	got := extractTextContent(msg)
	if got != "hello world" {
		t.Fatalf("got %q, want %q", got, "hello world")
	}
}

func TestExtractTextContent_ContentParts(t *testing.T) {
	text := "from parts"
	msg := &trpcmodel.Message{
		ContentParts: []trpcmodel.ContentPart{
			{Type: trpcmodel.ContentTypeText, Text: &text},
		},
	}
	got := extractTextContent(msg)
	if got != "from parts" {
		t.Fatalf("got %q, want %q", got, "from parts")
	}
}

func TestExtractTextContent_Empty(t *testing.T) {
	msg := &trpcmodel.Message{}
	got := extractTextContent(msg)
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestToolResultGateEnabled_Default(t *testing.T) {
	if !toolResultGateEnabled(biz.Agent{}) {
		t.Fatal("default should be enabled when Settings is nil")
	}
}

func TestToolResultGateEnabled_WithSettings(t *testing.T) {
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolResultGateEnabled: true}}
	if !toolResultGateEnabled(ag) {
		t.Fatal("explicitly enabled should return true")
	}
}

func TestLastNToolResultIndexes(t *testing.T) {
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleUser},
		{Role: trpcmodel.RoleTool, Content: "old"},
		{Role: trpcmodel.RoleAssistant},
		{Role: trpcmodel.RoleTool, Content: "a"},
		{Role: trpcmodel.RoleTool, Content: "b"},
		{Role: trpcmodel.RoleTool, Content: "c"},
	}
	got := lastNToolResultIndexes(msgs, 3)
	if got[1] {
		t.Fatal("older tool result must not be exempt")
	}
	if !got[3] || !got[4] || !got[5] {
		t.Fatalf("last 3 tool results must be exempt, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// P0-2 spill fallback of the ToolResultGate BeforeModel hook (design §3.4):
// when the PG blob persist fails, the content is spilled and replaced by a
// locator pointer; if the spill also fails the content is preserved whole
// with an explicit enforcement=partial note. With no spill deps the legacy
// truncateContent fallback runs byte-identically.
// ---------------------------------------------------------------------------

// failingGateWriter forces gate.Check down its error path.
type failingGateWriter struct{}

func (failingGateWriter) SaveBlob(context.Context, *biz.ToolResultBlob) error {
	return errors.New("pg unavailable")
}

type noopReplacementReader struct{}

func (noopReplacementReader) GetReplacementByMessage(context.Context, string, string) (*biz.ToolResultReplacement, error) {
	return nil, nil
}

type noopReplacementWriter struct{}

func (noopReplacementWriter) SaveReplacement(context.Context, *biz.ToolResultReplacement) error {
	return nil
}

func newFailingGate() *biz.ToolResultGate {
	return biz.NewToolResultGate(nil, failingGateWriter{}, noopReplacementReader{}, noopReplacementWriter{})
}

// gateHookInvocationCtx carries a session so the hook proceeds past its
// session guard.
func gateHookInvocationCtx(sessionID string) context.Context {
	inv := &trpcagent.Invocation{Session: &trpcsession.Session{ID: sessionID}}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

// oversizedToolMessages builds 4 tool messages: the first oversized (not in
// the last-3 exempt window), the rest small.
func oversizedToolMessages(oversizedContent string) []trpcmodel.Message {
	return []trpcmodel.Message{
		{Role: trpcmodel.RoleUser, Content: "run"},
		{Role: trpcmodel.RoleTool, ToolID: "call_big", ToolName: "web_search", Content: oversizedContent},
		{Role: trpcmodel.RoleTool, ToolID: "call_a", ToolName: "t", Content: "a"},
		{Role: trpcmodel.RoleTool, ToolID: "call_b", ToolName: "t", Content: "b"},
		{Role: trpcmodel.RoleTool, ToolID: "call_c", ToolName: "t", Content: "c"},
	}
}

func fireGateHook(t *testing.T, hook callbacks.Callback, ctx context.Context, msgs []trpcmodel.Message) []trpcmodel.Message {
	t.Helper()
	bm, ok := hook.(callbacks.BeforeModelHook)
	if !ok {
		t.Fatalf("hook does not implement BeforeModelHook: %T", hook)
	}
	res, err := bm.HandleBeforeModel(ctx, &trpcmodel.BeforeModelArgs{
		Request: &trpcmodel.Request{Messages: msgs},
	})
	if err != nil {
		t.Fatalf("HandleBeforeModel err: %v", err)
	}
	_ = res
	return msgs
}

func newGateSpillStore(t *testing.T) *toolspill.LocalFS {
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

func locatorFromPointerText(t *testing.T, pointer string) string {
	t.Helper()
	i := strings.Index(pointer, toolspill.Scheme+toolspill.StoreFS+"/")
	if i < 0 {
		t.Fatalf("pointer text carries no fs locator: %q", pointer)
	}
	rest := pointer[i:]
	j := strings.Index(rest, "｜")
	k := strings.Index(rest, "]")
	end := j
	if k >= 0 && (end < 0 || k < end) {
		end = k
	}
	if end <= 0 {
		t.Fatalf("cannot delimit locator in pointer text: %q", pointer)
	}
	return rest[:end]
}

func TestGateHook_PersistFailedLegacyWithoutSpill(t *testing.T) {
	oversized := strings.Repeat("x", biz.ToolResultSizeThreshold+1)
	hook := newToolResultGateBeforeHook(newFailingGate(), biz.Agent{}, loggateway.NewNoop())
	msgs := fireGateHook(t, hook, gateHookInvocationCtx("sess-1"), oversizedToolMessages(oversized))
	got := msgs[1].Content
	if !strings.Contains(got, "persist failed") {
		t.Fatalf("legacy fallback must keep the truncateContent marker, got %q...", got[:80])
	}
	if len([]rune(got)) >= len([]rune(oversized)) {
		t.Fatalf("legacy fallback must truncate, runes=%d original=%d", len([]rune(got)), len([]rune(oversized)))
	}
	if strings.Contains(got, "spill://") {
		t.Fatal("no spill deps were wired; a locator must not appear")
	}
}

func TestGateHook_PersistFailedSpillsWithLocator(t *testing.T) {
	oversized := strings.Repeat("data-数据-", biz.ToolResultSizeThreshold/5) // >50K runes
	store := newGateSpillStore(t)
	deps := &toolspill.Deps{Store: store}
	hook := newToolResultGateBeforeHook(newFailingGate(), biz.Agent{}, loggateway.NewNoop(), deps)
	msgs := fireGateHook(t, hook, gateHookInvocationCtx("sess-1"), oversizedToolMessages(oversized))
	got := msgs[1].Content
	locator := locatorFromPointerText(t, got)
	loc, err := toolspill.ParseLocator(locator)
	if err != nil {
		t.Fatalf("locator must parse: %v", err)
	}
	if loc.Store != toolspill.StoreFS {
		t.Fatalf("store = %q, want fs", loc.Store)
	}
	// Byte-faithful round trip: the model can read the full content back.
	rec, err := store.Get(context.Background(), "sess-1", locator)
	if err != nil {
		t.Fatalf("Get spilled record: %v", err)
	}
	if string(rec.Content) != oversized {
		t.Fatalf("spilled content mismatch: got %d bytes, want %d", len(rec.Content), len(oversized))
	}
	if rec.Tool != "web_search" {
		t.Fatalf("spilled tool = %q, want web_search", rec.Tool)
	}
}

type failingGateSpillStore struct{}

func (failingGateSpillStore) Put(context.Context, toolspill.Record) (toolspill.Locator, error) {
	return toolspill.Locator{}, errors.New("disk gone")
}
func (failingGateSpillStore) Get(context.Context, string, string) (toolspill.Record, error) {
	return toolspill.Record{}, toolspill.ErrNotFound
}
func (failingGateSpillStore) Stat(context.Context, string, string) (toolspill.FileInfo, error) {
	return toolspill.FileInfo{}, toolspill.ErrNotFound
}
func (failingGateSpillStore) GC(context.Context, time.Time) (toolspill.GCStats, error) {
	return toolspill.GCStats{}, nil
}

func TestGateHook_PersistAndSpillFailedPreservesContent(t *testing.T) {
	oversized := strings.Repeat("x", biz.ToolResultSizeThreshold+1)
	var dedupeKey string
	deps := &toolspill.Deps{
		Store: failingGateSpillStore{},
		Hooks: toolspill.Hooks{OnSpillFailed: func(_ context.Context, key string, _ error) { dedupeKey = key }},
	}
	hook := newToolResultGateBeforeHook(newFailingGate(), biz.Agent{}, loggateway.NewNoop(), deps)
	msgs := fireGateHook(t, hook, gateHookInvocationCtx("sess-1"), oversizedToolMessages(oversized))
	got := msgs[1].Content
	if got != oversized+toolspill.PartialPreservedNote {
		t.Fatalf("fail-soft must preserve the full content + partial note, runes=%d want=%d", len([]rune(got)), len([]rune(oversized+toolspill.PartialPreservedNote)))
	}
	if !strings.Contains(got, "enforcement=partial") {
		t.Fatal("partial note must carry the enforcement=partial marker")
	}
	if dedupeKey == "" || !strings.HasPrefix(dedupeKey, "spill_failed:sess-1:") {
		t.Fatalf("Important-level spill_failed hook must fire with session dedupe key, got %q", dedupeKey)
	}
}
