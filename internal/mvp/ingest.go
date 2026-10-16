package mvp

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz/content"
	"aranea-agents/internal/connector/ingest"
	"aranea-agents/internal/connector/ingest/rss"
	"aranea-agents/internal/connector/ingest/search"
)

// IngestResult summarizes one ingest run for HTTP / cron.
type IngestResult struct {
	Source  string   `json:"source"`
	Fetched int      `json:"fetched"`
	Created int      `json:"created"`
	Deduped int      `json:"deduped"`
	FeedURL string   `json:"feed_url,omitempty"`
	Query   string   `json:"query,omitempty"`
	Errors  []string `json:"errors,omitempty"`
}

// IngestRSSFeed fetches one RSS/Atom URL and upserts ContentItems.
func IngestRSSFeed(ctx context.Context, feedURL, workspaceID string) (IngestResult, error) {
	feedURL = strings.TrimSpace(feedURL)
	if feedURL == "" {
		return IngestResult{}, fmt.Errorf("mvp: feed_url required")
	}
	adapter := &rss.Adapter{FeedURL: feedURL}
	items, _, err := adapter.Fetch(ctx, "")
	if err != nil {
		return IngestResult{Source: "rss", FeedURL: feedURL}, err
	}
	return persistRawItems("rss", workspaceID, feedURL, "", items)
}

// IngestRSSFeeds runs multiple feeds (cron / ops batch). Continues on per-feed errors.
func IngestRSSFeeds(ctx context.Context, feedURLs []string, workspaceID string) (IngestResult, error) {
	out := IngestResult{Source: "rss"}
	for _, u := range feedURLs {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		res, err := IngestRSSFeed(ctx, u, workspaceID)
		out.Fetched += res.Fetched
		out.Created += res.Created
		out.Deduped += res.Deduped
		if err != nil {
			out.Errors = append(out.Errors, u+": "+err.Error())
		}
	}
	if out.Fetched == 0 && len(out.Errors) > 0 {
		return out, fmt.Errorf("mvp: rss ingest failed: %s", strings.Join(out.Errors, "; "))
	}
	return out, nil
}

// IngestSearchHits maps search hits → ContentItems (source_type=search).
func IngestSearchHits(ctx context.Context, workspaceID, query string, hits []search.Hit) (IngestResult, error) {
	_ = ctx
	if len(hits) == 0 {
		return IngestResult{}, fmt.Errorf("mvp: search hits required (provide hits[] or configure a searcher)")
	}
	adapter := &search.Adapter{
		Query: query,
		Searcher: func(context.Context, string) ([]search.Hit, error) {
			return hits, nil
		},
	}
	items, _, err := adapter.Fetch(context.Background(), "")
	if err != nil {
		return IngestResult{Source: "search", Query: query}, err
	}
	return persistRawItems("search", workspaceID, "", query, items)
}

func persistRawItems(source, workspaceID, feedURL, query string, items []ingest.RawItem) (IngestResult, error) {
	rt := Ensure()
	if rt == nil || rt.Content == nil {
		return IngestResult{}, fmt.Errorf("mvp: content usecase not configured")
	}
	out := IngestResult{Source: source, Fetched: len(items), FeedURL: feedURL, Query: query}
	for _, raw := range items {
		title := strings.TrimSpace(raw.Title)
		if title == "" {
			title = strings.TrimSpace(raw.SourceURL)
		}
		if title == "" {
			continue
		}
		_, dup, err := rt.Content.CreateFromIngest(content.Item{
			WorkspaceID: workspaceID,
			SourceType:  raw.SourceType,
			SourceURL:   raw.SourceURL,
			Title:       title,
			Body:        raw.Body,
			Lang:        raw.Lang,
			RawPayload:  raw.RawPayload,
		})
		if err != nil {
			out.Errors = append(out.Errors, err.Error())
			continue
		}
		if dup {
			out.Deduped++
		} else {
			out.Created++
		}
	}
	return out, nil
}
