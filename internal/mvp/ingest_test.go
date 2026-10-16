package mvp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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
