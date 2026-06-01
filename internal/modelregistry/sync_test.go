package modelregistry

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

func TestSyncerNotModified(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, loggateway.NewNoop())
	if err := st.SavePolicy(DefaultPolicy()); err != nil {
		t.Fatal(err)
	}
	cat := Directory{"openai": {ID: "openai", Name: "OpenAI", Models: map[string]Model{}}}
	meta := Meta{
		SyncedAt:      time.Now().UTC().Format(time.RFC3339),
		ETag:          `"cached"`,
		SourceURL:     DefaultPolicy().SourceURL,
		ProviderCount: 1,
	}
	if err := st.SaveDirectory(cat, meta); err != nil {
		t.Fatal(err)
	}

	oldHook := catalogFetchHook
	defer func() { catalogFetchHook = oldHook }()
	catalogFetchHook = func(ctx context.Context, sourceURL, ifNoneMatch string) (FetchResult, error) {
		if ifNoneMatch != `"cached"` {
			t.Fatalf("expected If-None-Match cached, got %q", ifNoneMatch)
		}
		return FetchResult{NotModified: true, ETag: `"cached"`}, nil
	}

	out, err := NewSyncer(st, loggateway.NewNoop()).Sync(context.Background(), SyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Message != "not modified (304)" || out.Meta.ETag != `"cached"` {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestSyncer_FetchError(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, loggateway.NewNoop())
	if err := st.SavePolicy(DefaultPolicy()); err != nil {
		t.Fatal(err)
	}

	oldHook := catalogFetchHook
	defer func() { catalogFetchHook = oldHook }()
	catalogFetchHook = func(ctx context.Context, sourceURL, ifNoneMatch string) (FetchResult, error) {
		return FetchResult{}, errors.New("network unreachable")
	}

	out, err := NewSyncer(st, loggateway.NewNoop()).Sync(context.Background(), SyncInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if out.Status != "failed" {
		t.Fatalf("expected status failed, got %q", out.Status)
	}
	if !strings.Contains(out.Message, "network unreachable") {
		t.Fatalf("expected message to contain error info, got %q", out.Message)
	}

	logs, lerr := ReadSyncLogs(st, 10)
	if lerr != nil {
		t.Fatal(lerr)
	}
	if len(logs) == 0 {
		t.Fatal("expected sync log entry to be appended")
	}
	if logs[0].Status != "failed" {
		t.Fatalf("expected log status failed, got %q", logs[0].Status)
	}
}

func TestSyncer_ParseError(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, loggateway.NewNoop())
	if err := st.SavePolicy(DefaultPolicy()); err != nil {
		t.Fatal(err)
	}

	oldHook := catalogFetchHook
	defer func() { catalogFetchHook = oldHook }()
	catalogFetchHook = func(ctx context.Context, sourceURL, ifNoneMatch string) (FetchResult, error) {
		return FetchResult{Body: []byte(`{invalid`)}, nil
	}

	out, err := NewSyncer(st, loggateway.NewNoop()).Sync(context.Background(), SyncInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if out.Status != "failed" {
		t.Fatalf("expected status failed, got %q", out.Status)
	}
}

func TestSyncer_DryRun(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, loggateway.NewNoop())
	if err := st.SavePolicy(DefaultPolicy()); err != nil {
		t.Fatal(err)
	}

	oldHook := catalogFetchHook
	defer func() { catalogFetchHook = oldHook }()
	catalogFetchHook = func(ctx context.Context, sourceURL, ifNoneMatch string) (FetchResult, error) {
		return FetchResult{
			Body: []byte(`{"openai":{"id":"openai","name":"OpenAI","models":{"gpt-4o":{"id":"gpt-4o","name":"GPT-4o"}}}}`),
			ETag: `"etag-dry"`,
		}, nil
	}

	out, err := NewSyncer(st, loggateway.NewNoop()).Sync(context.Background(), SyncInput{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "ok" {
		t.Fatalf("expected status ok, got %q", out.Status)
	}
	if !strings.Contains(out.Message, "dry run") {
		t.Fatalf("expected message to contain 'dry run', got %q", out.Message)
	}

	loaded, _, lerr := st.LoadDirectory()
	if lerr == nil && len(loaded) > 0 {
		t.Fatal("expected store to be empty after dry run")
	}
}

func TestSyncer_Success(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, loggateway.NewNoop())
	if err := st.SavePolicy(DefaultPolicy()); err != nil {
		t.Fatal(err)
	}

	oldHook := catalogFetchHook
	defer func() { catalogFetchHook = oldHook }()
	catalogFetchHook = func(ctx context.Context, sourceURL, ifNoneMatch string) (FetchResult, error) {
		return FetchResult{
			Body: []byte(`{"openai":{"id":"openai","name":"OpenAI","models":{"gpt-4o":{"id":"gpt-4o","name":"GPT-4o"}}}}`),
			ETag: `"etag-ok"`,
		}, nil
	}

	out, err := NewSyncer(st, loggateway.NewNoop()).Sync(context.Background(), SyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "ok" {
		t.Fatalf("expected status ok, got %q", out.Status)
	}
	if !strings.Contains(out.Message, "synced") {
		t.Fatalf("expected message to contain 'synced', got %q", out.Message)
	}

	loaded, meta, lerr := st.LoadDirectory()
	if lerr != nil {
		t.Fatal(lerr)
	}
	if len(loaded) == 0 {
		t.Fatal("expected directory to be saved in store")
	}
	if meta.SyncedAt == "" {
		t.Fatal("expected Meta.SyncedAt to be set")
	}
	if meta.ETag == "" {
		t.Fatal("expected Meta.ETag to be set")
	}
	if meta.SHA256 == "" {
		t.Fatal("expected Meta.SHA256 to be set")
	}
}

func TestSyncer_NeedsScheduledSync_ScheduledNotExpired(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, loggateway.NewNoop())

	policy := Policy{
		SourceURL:         "https://example.com",
		SyncPolicy:        "scheduled",
		SyncIntervalHours: 24,
	}
	if err := st.SavePolicy(policy); err != nil {
		t.Fatal(err)
	}

	meta := Meta{
		SyncedAt: time.Now().UTC().Format(time.RFC3339),
	}
	cat := Directory{"openai": {ID: "openai", Name: "OpenAI", Models: map[string]Model{}}}
	if err := st.SaveDirectory(cat, meta); err != nil {
		t.Fatal(err)
	}

	needs, _, err := NewSyncer(st, loggateway.NewNoop()).NeedsScheduledSync()
	if err != nil {
		t.Fatal(err)
	}
	if needs {
		t.Fatal("expected false, sync interval not expired yet")
	}
}

func TestSyncer_NeedsScheduledSync_ScheduledExpired(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, loggateway.NewNoop())

	policy := Policy{
		SourceURL:         "https://example.com",
		SyncPolicy:        "scheduled",
		SyncIntervalHours: 1,
	}
	if err := st.SavePolicy(policy); err != nil {
		t.Fatal(err)
	}

	meta := Meta{
		SyncedAt: time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339),
	}
	cat := Directory{"openai": {ID: "openai", Name: "OpenAI", Models: map[string]Model{}}}
	if err := st.SaveDirectory(cat, meta); err != nil {
		t.Fatal(err)
	}

	needs, _, err := NewSyncer(st, loggateway.NewNoop()).NeedsScheduledSync()
	if err != nil {
		t.Fatal(err)
	}
	if !needs {
		t.Fatal("expected true, sync interval expired")
	}
}

func TestSyncer_NeedsScheduledSync_ManualPolicy(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, loggateway.NewNoop())

	policy := Policy{
		SourceURL:  "https://example.com",
		SyncPolicy: "manual",
	}
	if err := st.SavePolicy(policy); err != nil {
		t.Fatal(err)
	}

	needs, _, err := NewSyncer(st, loggateway.NewNoop()).NeedsScheduledSync()
	if err != nil {
		t.Fatal(err)
	}
	if needs {
		t.Fatal("expected false for manual policy")
	}
}

func TestSyncer_NeedsScheduledSync_NoMeta(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, loggateway.NewNoop())

	policy := Policy{
		SourceURL:         "https://example.com",
		SyncPolicy:        "scheduled",
		SyncIntervalHours: 24,
	}
	if err := st.SavePolicy(policy); err != nil {
		t.Fatal(err)
	}

	needs, _, err := NewSyncer(st, loggateway.NewNoop()).NeedsScheduledSync()
	if err != nil {
		t.Fatal(err)
	}
	if !needs {
		t.Fatal("expected true when no meta exists")
	}
}

func TestSyncer_NeedsScheduledSync_DefaultInterval(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, loggateway.NewNoop())

	policy := Policy{
		SourceURL:         "https://example.com",
		SyncPolicy:        "scheduled",
		SyncIntervalHours: 0,
	}
	if err := st.SavePolicy(policy); err != nil {
		t.Fatal(err)
	}

	meta := Meta{
		SyncedAt: time.Now().UTC().Add(-23 * time.Hour).Format(time.RFC3339),
	}
	cat := Directory{"openai": {ID: "openai", Name: "OpenAI", Models: map[string]Model{}}}
	if err := st.SaveDirectory(cat, meta); err != nil {
		t.Fatal(err)
	}

	needs, _, err := NewSyncer(st, loggateway.NewNoop()).NeedsScheduledSync()
	if err != nil {
		t.Fatal(err)
	}
	if needs {
		t.Fatal("expected false, 23h < 24h default interval")
	}
}
