package crawl_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/knowledge/crawl"
)

// site is an httptest server with page hit counters and an optional
// robots.txt, used to exercise the scheduler end to end.
type site struct {
	srv *httptest.Server
	mux *http.ServeMux

	mu     sync.Mutex
	counts map[string]int
	robots string
}

func newSite(t *testing.T) *site {
	t.Helper()
	s := &site{counts: make(map[string]int)}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		if s.robots == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, s.robots)
	})
	s.srv = httptest.NewServer(s.mux)
	t.Cleanup(s.srv.Close)
	return s
}

// page registers an HTML page at path linking to each of links.
func (s *site) page(path, title string, links ...string) {
	s.mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.counts[path]++
		s.mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		var b strings.Builder
		fmt.Fprintf(&b, `<html><head><title>%s</title></head><body>`, title)
		for _, l := range links {
			fmt.Fprintf(&b, `<a href="%s">go</a>`, l)
		}
		b.WriteString(`</body></html>`)
		fmt.Fprint(w, b.String())
	})
}

func (s *site) count(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counts[path]
}

// collector gathers payloads handed to the sink.
type collector struct {
	mu  sync.Mutex
	ps  []crawl.Payload
	err error
}

func (c *collector) sink(_ context.Context, p crawl.Payload) error {
	if c.err != nil {
		return c.err
	}
	c.mu.Lock()
	c.ps = append(c.ps, p)
	c.mu.Unlock()
	return nil
}

func (c *collector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.ps)
}

func TestCrawlHostLockIsolation(t *testing.T) {
	build := func(t *testing.T) (*site, *site) {
		other := newSite(t)
		other.page("/other", "other-host")
		main := newSite(t)
		main.page("/", "root", "/local", other.srv.URL+"/other")
		main.page("/local", "local")
		return main, other
	}

	t.Run("locked by default", func(t *testing.T) {
		main, other := build(t)
		var col collector
		// Zero-value rule: host lock is the safe default.
		stats := crawl.Crawl(context.Background(), []string{main.srv.URL}, crawl.ScopeRule{}, col.sink)
		if stats.Fetched != 2 {
			t.Errorf("Fetched = %d, want 2", stats.Fetched)
		}
		if other.count("/other") != 0 {
			t.Errorf("off-host /other fetched %d times, want 0", other.count("/other"))
		}
		if stats.Skipped != 1 {
			t.Errorf("Skipped = %d, want 1 (cross-host link)", stats.Skipped)
		}
	})

	t.Run("locked explicitly", func(t *testing.T) {
		main, other := build(t)
		var col collector
		rule := crawl.ScopeRule{HostLocked: true, PerHostDelay: time.Millisecond}
		stats := crawl.Crawl(context.Background(), []string{main.srv.URL}, rule, col.sink)
		if stats.Fetched != 2 || other.count("/other") != 0 {
			t.Errorf("Fetched=%d other-host=%d, want 2 and 0", stats.Fetched, other.count("/other"))
		}
		if col.count() != 2 {
			t.Errorf("sink payloads = %d, want 2", col.count())
		}
	})

	t.Run("unlocked follows cross-host", func(t *testing.T) {
		main, other := build(t)
		var col collector
		rule := crawl.ScopeRule{HostLocked: false, PerHostDelay: time.Millisecond}
		stats := crawl.Crawl(context.Background(), []string{main.srv.URL}, rule, col.sink)
		if stats.Fetched != 3 {
			t.Errorf("Fetched = %d, want 3", stats.Fetched)
		}
		if other.count("/other") != 1 {
			t.Errorf("off-host /other fetched %d times, want 1", other.count("/other"))
		}
	})
}

func TestCrawlExcludeRule(t *testing.T) {
	s := newSite(t)
	s.page("/", "root", "/open", "/private/x")
	s.page("/open", "open")
	s.page("/private/x", "private")

	var col collector
	rule := crawl.ScopeRule{
		HostLocked:        true,
		PerHostDelay:      time.Millisecond,
		PathPrefixExclude: []string{"/private"},
	}
	stats := crawl.Crawl(context.Background(), []string{s.srv.URL}, rule, col.sink)
	if stats.Fetched != 2 {
		t.Errorf("Fetched = %d, want 2", stats.Fetched)
	}
	if s.count("/private/x") != 0 {
		t.Errorf("/private/x fetched %d times, want 0", s.count("/private/x"))
	}
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", stats.Skipped)
	}
}

func TestCrawlIncludeRule(t *testing.T) {
	s := newSite(t)
	s.page("/docs/index", "index", "/docs/a", "/other/b")
	s.page("/docs/a", "a")
	s.page("/other/b", "b")

	var col collector
	rule := crawl.ScopeRule{
		HostLocked:        true,
		PerHostDelay:      time.Millisecond,
		PathPrefixInclude: []string{"/docs"},
	}
	stats := crawl.Crawl(context.Background(), []string{s.srv.URL + "/docs/index"}, rule, col.sink)
	if stats.Fetched != 2 {
		t.Errorf("Fetched = %d, want 2 (index + /docs/a)", stats.Fetched)
	}
	if s.count("/docs/a") != 1 || s.count("/other/b") != 0 {
		t.Errorf("counts: /docs/a=%d /other/b=%d, want 1 and 0", s.count("/docs/a"), s.count("/other/b"))
	}
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", stats.Skipped)
	}
}

func TestCrawlMaxDepth(t *testing.T) {
	s := newSite(t)
	s.page("/d0", "d0", "/d1")
	s.page("/d1", "d1", "/d2")
	s.page("/d2", "d2")

	var col collector
	rule := crawl.ScopeRule{HostLocked: true, PerHostDelay: time.Millisecond, MaxDepth: 1}
	stats := crawl.Crawl(context.Background(), []string{s.srv.URL + "/d0"}, rule, col.sink)
	if stats.Fetched != 2 {
		t.Errorf("Fetched = %d, want 2 (depth 0 and 1)", stats.Fetched)
	}
	if s.count("/d2") != 0 {
		t.Errorf("/d2 fetched %d times, want 0 (depth 2 exceeds MaxDepth 1)", s.count("/d2"))
	}
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", stats.Skipped)
	}
}

func TestCrawlMaxPages(t *testing.T) {
	s := newSite(t)
	s.page("/", "hub", "/p1", "/p2", "/p3", "/p4", "/p5")
	for _, p := range []string{"/p1", "/p2", "/p3", "/p4", "/p5"} {
		s.page(p, p)
	}

	var col collector
	rule := crawl.ScopeRule{HostLocked: true, PerHostDelay: time.Millisecond, MaxPages: 3}
	stats := crawl.Crawl(context.Background(), []string{s.srv.URL}, rule, col.sink)
	if stats.Fetched != 3 {
		t.Errorf("Fetched = %d, want 3 (MaxPages)", stats.Fetched)
	}
	total := s.count("/") + s.count("/p1") + s.count("/p2") + s.count("/p3") + s.count("/p4") + s.count("/p5")
	if total != 3 {
		t.Errorf("server saw %d page hits, want 3", total)
	}
}

func TestCrawlPerHostDelayPacing(t *testing.T) {
	s := newSite(t)
	s.page("/", "root", "/p1", "/p2")
	s.page("/p1", "p1")
	s.page("/p2", "p2")

	delay := 200 * time.Millisecond
	rule := crawl.ScopeRule{HostLocked: true, PerHostDelay: delay}
	start := time.Now()
	stats := crawl.Crawl(context.Background(), []string{s.srv.URL}, rule, nil)
	elapsed := time.Since(start)

	if stats.Fetched != 3 {
		t.Fatalf("Fetched = %d, want 3", stats.Fetched)
	}
	// Three serial fetches on one host imply at least two full intervals.
	if min := 2*delay - 50*time.Millisecond; elapsed < min {
		t.Errorf("elapsed = %v, want >= %v (per-host serial pacing not enforced)", elapsed, min)
	}
}

func TestCrawlRobotsDenyCountsSkipped(t *testing.T) {
	s := newSite(t)
	s.robots = "User-agent: *\nDisallow: /blocked\n"
	s.page("/", "root", "/open", "/blocked")
	s.page("/open", "open")
	s.page("/blocked", "blocked")

	var col collector
	rule := crawl.ScopeRule{HostLocked: true, PerHostDelay: time.Millisecond}
	stats := crawl.Crawl(context.Background(), []string{s.srv.URL}, rule, col.sink)
	if stats.Fetched != 2 {
		t.Errorf("Fetched = %d, want 2", stats.Fetched)
	}
	if s.count("/blocked") != 0 {
		t.Errorf("/blocked fetched %d times despite robots Disallow", s.count("/blocked"))
	}
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (robots deny)", stats.Skipped)
	}
}

func TestCrawlRobotsAllowAllWhenMissing(t *testing.T) {
	s := newSite(t) // no robots.txt -> 404 -> allow all
	s.page("/", "root", "/a")
	s.page("/a", "a")

	var col collector
	rule := crawl.ScopeRule{HostLocked: true, PerHostDelay: time.Millisecond}
	stats := crawl.Crawl(context.Background(), []string{s.srv.URL}, rule, col.sink)
	if stats.Fetched != 2 {
		t.Errorf("Fetched = %d, want 2", stats.Fetched)
	}
}

func TestCrawlDedupVisited(t *testing.T) {
	s := newSite(t)
	// Root links the same page three times (dup + fragment variants) and the
	// linked page points back at the seed, forming a cycle; dedup must hold.
	s.page("/", "root", "/p", "/p", "/p#frag")
	s.page("/p", "p", s.srv.URL)

	var col collector
	rule := crawl.ScopeRule{HostLocked: true, PerHostDelay: time.Millisecond}
	stats := crawl.Crawl(context.Background(), []string{s.srv.URL}, rule, col.sink)
	if stats.Fetched != 2 {
		t.Errorf("Fetched = %d, want 2 (dedup + cycle safety)", stats.Fetched)
	}
	if stats.Visited != 2 {
		t.Errorf("Visited = %d, want 2", stats.Visited)
	}
	if s.count("/") != 1 || s.count("/p") != 1 {
		t.Errorf("counters: /=%d /p=%d, want 1 and 1", s.count("/"), s.count("/p"))
	}
}

func TestCrawlSinkError(t *testing.T) {
	s := newSite(t)
	s.page("/", "root")

	col := collector{err: errors.New("sink boom")}
	rule := crawl.ScopeRule{HostLocked: true, PerHostDelay: time.Millisecond}
	stats := crawl.Crawl(context.Background(), []string{s.srv.URL}, rule, col.sink)
	if stats.Fetched != 0 || stats.Failed != 1 {
		t.Errorf("Fetched=%d Failed=%d, want 0 and 1", stats.Fetched, stats.Failed)
	}
}

func TestCrawlContextCancel(t *testing.T) {
	hang := make(chan struct{})
	s := &site{counts: make(map[string]int)}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/hang", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-hang:
		}
	})
	s.srv = httptest.NewServer(s.mux)
	t.Cleanup(func() {
		close(hang)
		s.srv.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	rule := crawl.ScopeRule{HostLocked: true, PerHostDelay: time.Millisecond}
	start := time.Now()
	stats := crawl.Crawl(ctx, []string{s.srv.URL + "/hang"}, rule, nil)
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Errorf("Crawl took %v after cancel, want prompt abort", elapsed)
	}
	if stats.Fetched != 0 {
		t.Errorf("Fetched = %d, want 0 (cancelled before any success)", stats.Fetched)
	}
}

func TestCrawlPayloadTitleAndBodyReachTheSink(t *testing.T) {
	s := newSite(t)
	s.page("/", "root", "/child")
	s.page("/child", "child-title")

	var col collector
	rule := crawl.ScopeRule{HostLocked: true, PerHostDelay: time.Millisecond}
	stats := crawl.Crawl(context.Background(), []string{s.srv.URL}, rule, col.sink)
	if stats.Fetched != 2 {
		t.Fatalf("Fetched = %d, want 2", stats.Fetched)
	}
	titles := map[string]bool{}
	for _, p := range col.ps {
		titles[p.Title] = true
		if p.MIME != "text/html" {
			t.Errorf("payload MIME = %q, want text/html", p.MIME)
		}
		if len(p.Body) == 0 {
			t.Error("payload body empty")
		}
	}
	if !titles["root"] || !titles["child-title"] {
		t.Errorf("sink payload titles = %v, want root and child-title", titles)
	}
}
