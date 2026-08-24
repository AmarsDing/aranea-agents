package data

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func setupEvalStore(t *testing.T) biz.EvalMemoryStore {
	t.Helper()
	r := setupL3FTSTestRepo(t, nil, 0)
	return NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
}

func TestEvalStore_PreservesAddressAndClassifiesPreference(t *testing.T) {
	store := setupEvalStore(t)
	ctx := context.Background()
	n, err := store.AddMessages(ctx, "u-pii", "s-pii", []biz.EvalMessage{
		{Role: "user", Content: "I live at 123 Maple Street", MessageID: "m1"},
		{Role: "user", Content: "My favorite color is blue", MessageID: "m2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("stored=%d, want 2", n)
	}
	items, err := store.SearchMemories(ctx, "u-pii", "Where does the user live?", 10)
	if err != nil {
		t.Fatal(err)
	}
	joined := evalContents(items)
	if !strings.Contains(joined, "123 Maple Street") {
		t.Fatalf("search missed plaintext address: %s", joined)
	}
	if strings.Contains(joined, "[address]") {
		t.Fatalf("address was redacted: %s", joined)
	}
}

func TestEvalStore_SupersedesFavoriteColor(t *testing.T) {
	store := setupEvalStore(t)
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "u-upd", "s-upd", []biz.EvalMessage{
		{Role: "user", Content: "My favorite color is blue", MessageID: "m1"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddMessages(ctx, "u-upd", "s-upd", []biz.EvalMessage{
		{Role: "user", Content: "My favorite color is red", MessageID: "m2"},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := store.SearchMemories(ctx, "u-upd", "What is the favorite color?", 10)
	if err != nil {
		t.Fatal(err)
	}
	joined := evalContents(items)
	if !strings.Contains(joined, "red") {
		t.Fatalf("missing updated color: %s", joined)
	}
	if strings.Contains(joined, "blue") {
		t.Fatalf("stale color leaked: %s", joined)
	}
}

func TestEvalStore_SearchMergesL2Episode(t *testing.T) {
	store := setupEvalStore(t)
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "u-l2", "s-l2", []biz.EvalMessage{
		{Role: "user", Content: "we shipped the auth patch yesterday", MessageID: "m1"},
		{Role: "assistant", Content: "recorded the deploy", MessageID: "m2"},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := store.SearchMemories(ctx, "u-l2", "auth patch deploy", 20)
	if err != nil {
		t.Fatal(err)
	}
	var hasL2 bool
	for _, it := range items {
		if strings.HasPrefix(it.ID, "l2:") {
			hasL2 = true
			if !strings.Contains(it.Content, "auth patch") {
				t.Fatalf("L2 content=%q, want auth patch", it.Content)
			}
		}
	}
	if !hasL2 {
		t.Fatalf("search missing L2 episode among %d hits: %s", len(items), evalContents(items))
	}
}

func TestEvalStore_UserIsolation(t *testing.T) {
	store := setupEvalStore(t)
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "u-a", "s-a", []biz.EvalMessage{
		{Role: "user", Content: "secret token ALPHA99", MessageID: "m1"},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := store.SearchMemories(ctx, "u-b", "ALPHA99", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("cross-user leak: %+v", items)
	}
}

func TestEvalStore_EmbedIsAsync(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop()).(*evalMemoryStore)
	started := make(chan struct{})
	var running atomic.Bool
	store.syncer = &evalSlowSyncer{started: started, running: &running}
	ctx := context.Background()
	begin := time.Now()
	if _, err := store.AddMessages(ctx, "u-async", "s-async", []biz.EvalMessage{
		{Role: "user", Content: "I like coffee", MessageID: "m1"},
	}); err != nil {
		t.Fatal(err)
	}
	if time.Since(begin) > 200*time.Millisecond {
		t.Fatalf("AddMessages blocked on embed: %s", time.Since(begin))
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("embed goroutine never started")
	}
}

func evalContents(items []biz.EvalMemoryItem) string {
	var b strings.Builder
	for _, it := range items {
		b.WriteString(it.Content)
		b.WriteByte('\n')
	}
	return b.String()
}

type evalSlowSyncer struct {
	started chan struct{}
	running *atomic.Bool
	once    sync.Once
}

func (s *evalSlowSyncer) SyncFactIndex(context.Context, string, string, string, string) error {
	return nil
}

func (s *evalSlowSyncer) SyncFactIndexFromRow(context.Context, []byte) error {
	s.once.Do(func() { close(s.started) })
	s.running.Store(true)
	time.Sleep(300 * time.Millisecond)
	s.running.Store(false)
	return nil
}

func TestApplyFactPIIGate_SkipKeepsAddress(t *testing.T) {
	stmt, _, pii, _, typesJSON := applyFactPIIGate(biz.FactUpsert{
		Statement:     "I live at 123 Maple Street",
		SkipPIIRedact: true,
	})
	if pii != 1 {
		t.Fatalf("pii=%d, want flagged", pii)
	}
	if !strings.Contains(stmt, "123 Maple Street") {
		t.Fatalf("statement redacted: %q", stmt)
	}
	if !strings.Contains(typesJSON, "home_address") {
		t.Fatalf("types=%s, want home_address", typesJSON)
	}
}

func TestEvalItemFromFactJSON_UsesEventTime(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"id":         "f1",
		"statement":  "Alice likes blue",
		"created_at": "2026-01-01T00:00:00Z",
		"valid_from": "2026-02-01T00:00:00Z",
		"scores":     map[string]any{"total": 0.8},
	})
	item, ok := evalItemFromFactJSON(raw, 0)
	if !ok {
		t.Fatal("decode failed")
	}
	if item.Timestamp != "2026-02-01T00:00:00Z" {
		t.Fatalf("timestamp=%q, want valid_from", item.Timestamp)
	}
}
