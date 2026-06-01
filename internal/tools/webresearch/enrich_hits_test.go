package webresearch_test

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/tools/webresearch"
)

func TestEnrichHits(t *testing.T) {
	tests := []struct {
		name     string
		hits     []webresearch.Hit
		fetchTop int
		wantWarn bool
		wantErr  bool
	}{
		{
			name:     "fetchTop zero",
			hits:     []webresearch.Hit{{URL: "https://example.com"}},
			fetchTop: 0,
		},
		{
			name:     "fetchTop negative",
			hits:     []webresearch.Hit{{URL: "https://example.com"}},
			fetchTop: -1,
		},
		{
			name:     "empty hits",
			hits:     nil,
			fetchTop: 3,
		},
		{
			name: "all hits have content",
			hits: []webresearch.Hit{
				{URL: "https://a.com", Content: "already has content"},
				{URL: "https://b.com", Content: "also has content"},
			},
			fetchTop: 3,
		},
		{
			name: "hits with empty content but empty URL",
			hits: []webresearch.Hit{
				{URL: "", Content: ""},
				{URL: "   ", Content: ""},
			},
			fetchTop: 3,
		},
		{
			name: "hits with whitespace only URL",
			hits: []webresearch.Hit{
				{URL: "  ", Content: ""},
			},
			fetchTop: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := webresearch.Config{
				Provider:  "tavily",
				APIKey:    "test-key",
				Timeout:   5 * time.Second,
				FetchTop:  tt.fetchTop,
			}
			warnings, err := webresearch.EnrichHits(context.Background(), tt.hits, tt.fetchTop, cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if (len(warnings) > 0) != tt.wantWarn {
				t.Fatalf("warnings = %v, wantWarn %v", warnings, tt.wantWarn)
			}
		})
	}
}
