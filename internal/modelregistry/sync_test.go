package modelregistry

import (
	"context"
	"testing"
	"time"
)

func TestSyncerNotModified(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
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

	out, err := NewSyncer(st).Sync(context.Background(), SyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Message != "not modified (304)" || out.Meta.ETag != `"cached"` {
		t.Fatalf("unexpected output: %+v", out)
	}
}
