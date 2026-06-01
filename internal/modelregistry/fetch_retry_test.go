package modelregistry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestCatalogFetchRetryLoop(t *testing.T) {
	calls := 0
	res, err := attemptCatalogFetch(context.Background(), func() (FetchResult, error) {
		calls++
		if calls < 3 {
			return FetchResult{}, fmt.Errorf("fetch catalog: HTTP 503")
		}
		return FetchResult{Body: []byte(`ok`), ETag: `"v1"`}, nil
	}, loggateway.NewNoop())
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
	}, loggateway.NewNoop())
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
	cat := Directory{
		"a": {ID: "a", Name: "Alpha"},
		"b": {ID: "b", Name: "Beta"},
		"c": {ID: "c", Name: "Gamma"},
	}
	all := ListProviders(cat, "", 10, 0)
	if len(all) != 3 {
		t.Fatalf("got %d providers", len(all))
	}
	if all[0].ID != "a" || all[1].ID != "b" || all[2].ID != "c" {
		t.Fatalf("providers not sorted by name: %+v", []string{all[0].ID, all[1].ID, all[2].ID})
	}
	page := ListProviders(cat, "", 1, 1)
	if len(page) != 1 {
		t.Fatalf("page len = %d", len(page))
	}
	if page[0].ID != "b" {
		t.Fatalf("offset page = %+v, want Beta", page[0].ID)
	}
}

func TestListModelsSorted(t *testing.T) {
	p := Provider{
		ID: "test",
		Models: map[string]Model{
			"z": {ID: "z", Name: "Zeta"},
			"a": {ID: "a", Name: "Alpha"},
			"d": {ID: "d", Name: "Deprecated", Status: "deprecated"},
		},
	}
	all := ListModels(p, "", true, 10, 0)
	if len(all) != 3 {
		t.Fatalf("got %d models", len(all))
	}
	if all[0].ID != "a" || all[1].ID != "z" || all[2].ID != "d" {
		t.Fatalf("models not sorted: %+v", []string{all[0].ID, all[1].ID, all[2].ID})
	}
	active := ListModels(p, "", false, 10, 0)
	if len(active) != 2 || active[0].ID != "a" {
		t.Fatalf("active models = %+v", active)
	}
}

func TestSearchRawLines(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir, loggateway.NewNoop())
	cat := Directory{
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]Model{
				"gpt-4o": {ID: "gpt-4o", Name: "GPT-4o"},
			},
		},
	}
	if err := st.SaveDirectory(cat, Meta{SyncedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	lines, total, err := st.SearchRawLines("gpt-4o", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total == 0 || len(lines) == 0 {
		t.Fatalf("expected matches, total=%d lines=%d", total, len(lines))
	}
	if !strings.Contains(lines[0], "gpt-4o") {
		t.Fatalf("expected model in block: %s", lines[0][:min(120, len(lines[0]))])
	}
	browse, browseTotal, err := st.SearchRawLines("", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if browseTotal != 1 || len(browse) != 1 {
		t.Fatalf("browse total=%d len=%d", browseTotal, len(browse))
	}
	if !strings.Contains(browse[0], `"id": "openai"`) {
		t.Fatalf("expected provider block")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
