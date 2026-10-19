package webresearch_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/internal/tools/webresearch"
	"aranea-agents/pkg/loggateway"
)

func TestSearch_PublicAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "A", "url": "https://example.com/a", "content": "snippet-a"},
			},
		})
	}))
	defer srv.Close()

	cfg := webresearch.Config{
		Provider:        webresearch.ProviderTavily,
		APIKey:          "test-key",
		MaxResults:      5,
		FetchTop:        0,
		Timeout:         0,
		TavilySearchURL: srv.URL,
	}
	resp, err := webresearch.Search(context.Background(), cfg, "q", loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 || resp.Results[0].Title != "A" {
		t.Fatalf("%+v", resp)
	}
}

func TestSearch_RequiresAPIKey(t *testing.T) {
	_, err := webresearch.Search(context.Background(), webresearch.Config{Provider: "tavily"}, "q", nil)
	if err == nil {
		t.Fatal("expected api_key error")
	}
}
