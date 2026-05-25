package modelcatalog

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestCatalogFetchRetryLoop(t *testing.T) {
	calls := 0
	res, err := attemptCatalogFetch(context.Background(), func() (FetchResult, error) {
		calls++
		if calls < 3 {
			return FetchResult{}, fmt.Errorf("fetch catalog: HTTP 503")
		}
		return FetchResult{Body: []byte(`ok`), ETag: `"v1"`}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
	if string(res.Body) != "ok" {
		t.Fatalf("body = %q", res.Body)
	}
}

func TestCatalogFetchRetryStopsOn404(t *testing.T) {
	calls := 0
	_, err := attemptCatalogFetch(context.Background(), func() (FetchResult, error) {
		calls++
		return FetchResult{}, fmt.Errorf("fetch catalog: HTTP 404")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestIsRetryableFetchErr(t *testing.T) {
	if !isRetryableFetchErr(errors.New("fetch catalog: HTTP 503")) {
		t.Fatal("503 should retry")
	}
	if isRetryableFetchErr(errors.New("fetch catalog: HTTP 404")) {
		t.Fatal("404 should not retry")
	}
}

func TestListProvidersOffset(t *testing.T) {
	cat := Catalog{
		"a": {ID: "a", Name: "Alpha"},
		"b": {ID: "b", Name: "Beta"},
		"c": {ID: "c", Name: "Gamma"},
	}
	all := ListProviders(cat, "", 10, 0)
	if len(all) != 3 {
		t.Fatalf("got %d providers", len(all))
	}
	page := ListProviders(cat, "", 1, 1)
	if len(page) != 1 {
		t.Fatalf("page len = %d", len(page))
	}
}

func TestSearchRawLines(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	cat := Catalog{"openai": {ID: "openai", Name: "OpenAI", Models: map[string]Model{}}}
	if err := st.SaveCatalog(cat, Meta{SyncedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	lines, total, err := st.SearchRawLines("openai", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total == 0 || len(lines) == 0 {
		t.Fatalf("expected matches, total=%d lines=%d", total, len(lines))
	}
}
