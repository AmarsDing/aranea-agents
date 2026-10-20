package crawl_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/knowledge/crawl"
)

func TestFetchPageHTMLTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<html><head><title>Hello &amp; World</title></head><body>content</body></html>`)
	}))
	defer srv.Close()

	p, err := (&crawl.Client{}).FetchPage(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if p.MIME != "text/html" {
		t.Errorf("MIME = %q, want text/html", p.MIME)
	}
	if p.Title != "Hello & World" {
		t.Errorf("Title = %q, want %q", p.Title, "Hello & World")
	}
	if p.URL != srv.URL || p.FinalURL != srv.URL {
		t.Errorf("URL/FinalURL = %q/%q, want %q", p.URL, p.FinalURL, srv.URL)
	}
	if !strings.Contains(string(p.Body), "<title>") {
		t.Errorf("body missing expected content: %q", p.Body)
	}
}

func TestFetchPageRedirectRecordsFinalURL(t *testing.T) {
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/final", http.StatusFound)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "landed")
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	p, err := (&crawl.Client{}).FetchPage(context.Background(), srv.URL+"/start")
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if want := srv.URL + "/final"; p.FinalURL != want {
		t.Errorf("FinalURL = %q, want %q", p.FinalURL, want)
	}
	if string(p.Body) != "landed" {
		t.Errorf("Body = %q, want %q", p.Body, "landed")
	}
}

func TestFetchPageUnsupportedMIME(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		fmt.Fprint(w, "binary blob")
	}))
	defer srv.Close()

	_, err := (&crawl.Client{}).FetchPage(context.Background(), srv.URL)
	if !errors.Is(err, crawl.ErrUnsupportedMIME) {
		t.Fatalf("err = %v, want errors.Is ErrUnsupportedMIME", err)
	}
}

func TestFetchPageTooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, strings.Repeat("x", 256))
	}))
	defer srv.Close()

	_, err := (&crawl.Client{MaxBytes: 64}).FetchPage(context.Background(), srv.URL)
	if !errors.Is(err, crawl.ErrTooLarge) {
		t.Fatalf("err = %v, want errors.Is ErrTooLarge", err)
	}
}

func TestFetchPagePlainTextNoTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, "just text")
	}))
	defer srv.Close()

	p, err := (&crawl.Client{}).FetchPage(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if p.MIME != "text/plain" {
		t.Errorf("MIME = %q, want text/plain", p.MIME)
	}
	if p.Title != "" {
		t.Errorf("Title = %q, want empty for non-HTML", p.Title)
	}
}

func TestFetchPageUserAgent(t *testing.T) {
	var got atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	if _, err := (&crawl.Client{}).FetchPage(context.Background(), srv.URL); err != nil {
		t.Fatalf("FetchPage default UA: %v", err)
	}
	if ua, _ := got.Load().(string); !strings.HasPrefix(ua, "aranea-crawler") {
		t.Errorf("default UA = %q, want aranea-crawler*", ua)
	}
	if _, err := (&crawl.Client{UserAgent: "testbot/9.9"}).FetchPage(context.Background(), srv.URL); err != nil {
		t.Fatalf("FetchPage custom UA: %v", err)
	}
	if ua, _ := got.Load().(string); ua != "testbot/9.9" {
		t.Errorf("custom UA = %q, want testbot/9.9", ua)
	}
}

func TestFetchPageHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	if _, err := (&crawl.Client{}).FetchPage(context.Background(), srv.URL); err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func TestFetchPageTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "slow")
	}))
	defer srv.Close()

	c := &crawl.Client{HC: &http.Client{Timeout: 50 * time.Millisecond}}
	if _, err := c.FetchPage(context.Background(), srv.URL); err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestFetchPageContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	if _, err := (&crawl.Client{}).FetchPage(ctx, srv.URL); err == nil {
		t.Fatal("expected error after cancel, got nil")
	}
}
