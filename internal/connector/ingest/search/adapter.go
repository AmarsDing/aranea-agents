package search

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz/content"
	"aranea-agents/internal/connector/ingest"
)

// Hit is a normalized web-search result (wrapper over webresearch / mocks).
type Hit struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Snippet string `json:"snippet,omitempty"`
}

// Searcher abstracts webresearch so outboundguard stays in that package.
type Searcher func(ctx context.Context, query string) ([]Hit, error)

// Adapter maps search hits → ingest.RawItem (source_type=search).
type Adapter struct {
	Query    string
	Searcher Searcher
}

func (a *Adapter) Name() string { return "search" }

func (a *Adapter) Fetch(ctx context.Context, cursor string) ([]ingest.RawItem, string, error) {
	if a.Searcher == nil {
		return nil, "", fmt.Errorf("search: searcher not configured")
	}
	hits, err := a.Searcher(ctx, a.Query)
	if err != nil {
		return nil, "", err
	}
	out := make([]ingest.RawItem, 0, len(hits))
	for _, h := range hits {
		out = append(out, ingest.RawItem{
			SourceType: content.SourceSearch,
			SourceURL:  h.URL,
			Title:      h.Title,
			Body:       h.Snippet,
		})
	}
	return out, "", nil
}
