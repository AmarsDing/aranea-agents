package crawl_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/internal/knowledge/crawl"
)

func TestParseSitemapURLSet(t *testing.T) {
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/a</loc></url>
  <url>
    <loc>https://example.com/b</loc>
    <lastmod>2026-01-01</lastmod>
  </url>
  <url><loc></loc></url>
</urlset>`
	urls, subs, err := crawl.ParseSitemap([]byte(doc))
	if err != nil {
		t.Fatalf("ParseSitemap: %v", err)
	}
	if len(subs) != 0 {
		t.Errorf("subSitemaps = %v, want none", subs)
	}
	want := []string{"https://example.com/a", "https://example.com/b"}
	if len(urls) != len(want) {
		t.Fatalf("urls = %v, want %v", urls, want)
	}
	for i := range want {
		if urls[i] != want[i] {
			t.Errorf("urls[%d] = %q, want %q", i, urls[i], want[i])
		}
	}
}

func TestParseSitemapIndexRecursion(t *testing.T) {
	// Three levels: index -> sub-index -> urlset. The caller recurses by
	// fetching each sub-sitemap and parsing again.
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/sm.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>%s/sub.xml</loc></sitemap>
</sitemapindex>`, srv.URL)
	})
	mux.HandleFunc("/sub.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>%s/leaf.xml</loc></sitemap>
</sitemapindex>`, srv.URL)
	})
	mux.HandleFunc("/leaf.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s/page1</loc></url>
  <url><loc>%s/page2</loc></url>
</urlset>`, srv.URL, srv.URL)
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	fetch := func(u string) []byte {
		resp, err := http.Get(u)
		if err != nil {
			t.Fatalf("GET %s: %v", u, err)
		}
		defer func() { _ = resp.Body.Close() }()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read %s: %v", u, err)
		}
		return data
	}

	urls, subs, err := crawl.ParseSitemap(fetch(srv.URL + "/sm.xml"))
	if err != nil {
		t.Fatalf("ParseSitemap index: %v", err)
	}
	if len(urls) != 0 || len(subs) != 1 || subs[0] != srv.URL+"/sub.xml" {
		t.Fatalf("level1: urls=%v subs=%v", urls, subs)
	}

	urls, subs, err = crawl.ParseSitemap(fetch(subs[0]))
	if err != nil {
		t.Fatalf("ParseSitemap sub-index: %v", err)
	}
	if len(urls) != 0 || len(subs) != 1 || subs[0] != srv.URL+"/leaf.xml" {
		t.Fatalf("level2: urls=%v subs=%v", urls, subs)
	}

	urls, subs, err = crawl.ParseSitemap(fetch(subs[0]))
	if err != nil {
		t.Fatalf("ParseSitemap urlset: %v", err)
	}
	if len(subs) != 0 {
		t.Errorf("leaf: subs = %v, want none", subs)
	}
	if len(urls) != 2 || urls[0] != srv.URL+"/page1" || urls[1] != srv.URL+"/page2" {
		t.Errorf("leaf: urls = %v", urls)
	}
}

func TestParseSitemapErrors(t *testing.T) {
	for name, doc := range map[string]string{
		"empty":     "   ",
		"not xml":   "this is not xml",
		"unknown":   `<feed><entry><loc>http://x/</loc></entry></feed>`,
		"truncated": `<urlset><url><loc>http://x/</loc>`,
	} {
		if _, _, err := crawl.ParseSitemap([]byte(doc)); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}
