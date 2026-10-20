package mvp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aranea-agents/internal/biz/content"
	"aranea-agents/internal/connector/ingest/search"
	"aranea-agents/internal/mvp"
)

func TestIngestRSSFeed_HTTP(t *testing.T) {
	mvp.ResetForTest()
	const xml = `<?xml version="1.0"?><rss version="2.0"><channel>
<item><title>Hello RSS</title><link>https://example.com/a</link><description>body</description></item>
</channel></rss>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(xml))
	}))
	defer srv.Close()
	res, err := mvp.IngestRSSFeed(context.Background(), srv.URL, "ws")
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 1 {
		t.Fatalf("created=%d fetched=%d", res.Created, res.Fetched)
	}
	items, total, err := mvp.Ensure().Content.List(content.ListFilter{WorkspaceID: "ws", SourceType: content.SourceRSS})
	if err != nil {
		t.Fatal(err)
	}
	if total < 1 || len(items) < 1 {
		t.Fatalf("list total=%d len=%d", total, len(items))
	}
}

func TestIngestSearchHits(t *testing.T) {
	mvp.ResetForTest()
	res, err := mvp.IngestSearchHits(context.Background(), "ws", "q", []search.Hit{
		{URL: "https://example.com/x", Title: "X", Snippet: "sn"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 1 {
		t.Fatalf("%+v", res)
	}
}

func TestIngestSearchHits_ViaConfiguredSearcher(t *testing.T) {
	mvp.ResetForTest()
	mvp.ConfigureSearcher(func(ctx context.Context, query string) ([]search.Hit, error) {
		if query != "widget" {
			t.Fatalf("query=%q", query)
		}
		return []search.Hit{{URL: "https://example.com/w", Title: "Widget", Snippet: "hit"}}, nil
	})
	res, err := mvp.IngestSearchHits(context.Background(), "ws", "widget", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 1 || res.Fetched != 1 {
		t.Fatalf("%+v", res)
	}
}

func TestIngestSearchHits_ProviderUnsetClearError(t *testing.T) {
	mvp.ResetForTest()
	mvp.ConfigureSearcher(mvp.DefaultSearcher())
	t.Setenv("TAVILY_API_KEY", "")
	t.Setenv("SERPAPI_API_KEY", "")
	_, err := mvp.IngestSearchHits(context.Background(), "ws", "need-provider", nil)
	if err == nil {
		t.Fatal("expected clear error when provider unset")
	}
	if !strings.Contains(err.Error(), "web_research provider unset") {
		t.Fatalf("error=%v", err)
	}
}

func TestIngestSearchHits_QueryRequiredWhenNoHits(t *testing.T) {
	mvp.ResetForTest()
	mvp.ConfigureSearcher(func(context.Context, string) ([]search.Hit, error) {
		t.Fatal("searcher should not run without query")
		return nil, nil
	})
	_, err := mvp.IngestSearchHits(context.Background(), "ws", "  ", nil)
	if err == nil || !strings.Contains(err.Error(), "query required") {
		t.Fatalf("err=%v", err)
	}
}
