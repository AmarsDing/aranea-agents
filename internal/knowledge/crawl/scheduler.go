package crawl

import (
	"bytes"
	"context"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

const (
	defaultWorkers      = 4
	defaultPerHostDelay = time.Second
)

// ScopeRule bounds a Crawl run.
type ScopeRule struct {
	// HostLocked restricts the crawl to the seed hosts. Crawls are
	// host-locked by default: a zero ScopeRule keeps the crawl on the
	// seed hosts; set other fields (or leave HostLocked=false alongside
	// them) to opt out explicitly.
	HostLocked bool
	// PathPrefixInclude, when non-empty, admits only URLs whose path
	// carries one of the prefixes.
	PathPrefixInclude []string
	// PathPrefixExclude rejects URLs whose path carries one of the
	// prefixes (evaluated before Include).
	PathPrefixExclude []string
	// MaxPages caps the number of dispatched fetches (0 = unlimited).
	MaxPages int
	// MaxDepth caps link depth from the seeds; seeds are depth 0
	// (0 = unlimited).
	MaxDepth int
	// PerHostDelay is the minimum interval between two fetches against
	// the same host (default 1s). A robots.txt Crawl-delay larger than
	// this value takes precedence.
	PerHostDelay time.Duration
}

// Stats summarizes a finished Crawl run.
type Stats struct {
	// Visited counts unique URLs dequeued for processing (after dedup).
	Visited int
	// Fetched counts successfully fetched and sunk payloads.
	Fetched int
	// Skipped counts URLs rejected by scope rules, robots.txt or the
	// MaxPages cap.
	Skipped int
	// Failed counts fetch, HTTP and sink errors.
	Failed int
}

// Crawl runs a breadth-first crawl over seeds obeying rule, handing each
// fetched page to sink. Four dispatcher workers feed one serial fetch queue
// per host; fetches against the same host are spaced by
// ScopeRule.PerHostDelay. Links are extracted only from <a href> of HTML
// payloads. Cancelling ctx aborts the crawl promptly and returns the
// partial statistics.
func Crawl(ctx context.Context, seeds []string, rule ScopeRule, sink func(context.Context, Payload) error) Stats {
	if sink == nil {
		sink = func(context.Context, Payload) error { return nil }
	}
	s := &scheduler{
		client:    &Client{},
		rule:      rule,
		locked:    rule.hostLocked(),
		sink:      sink,
		seedHosts: make(map[string]struct{}),
		frontier:  newFrontier(),
		stop:      make(chan struct{}),
		visited:   make(map[string]struct{}),
		robots:      make(map[string]*Robots),
		robotsLocks: make(map[string]*sync.Mutex),
		hqs:       make(map[string]*hostQueue),
	}
	delay := rule.PerHostDelay
	if delay <= 0 {
		delay = defaultPerHostDelay
	}
	s.delay = delay
	return s.run(ctx, seeds)
}

// hostLocked resolves the host-lock default: a fully zero rule stays on the
// seed hosts; an explicitly configured rule honors the flag as given.
func (r ScopeRule) hostLocked() bool {
	if !r.HostLocked &&
		r.MaxPages == 0 && r.MaxDepth == 0 && r.PerHostDelay == 0 &&
		len(r.PathPrefixInclude) == 0 && len(r.PathPrefixExclude) == 0 {
		return true
	}
	return r.HostLocked
}

type job struct {
	raw   string
	u     *url.URL
	depth int
}

type hostQueue struct {
	key string // scheme://host
	ch  chan job
}

type scheduler struct {
	client    *Client
	rule      ScopeRule
	locked    bool
	delay     time.Duration
	sink      func(context.Context, Payload) error
	seedHosts map[string]struct{}

	frontier *frontier
	stop     chan struct{}
	ctx      context.Context // set in run before workers start

	statsMu    sync.Mutex
	stats      Stats
	dispatched int

	visitedMu sync.Mutex
	visited   map[string]struct{}

	robotsMu    sync.Mutex
	robots      map[string]*Robots
	robotsLocks map[string]*sync.Mutex

	hostsMu sync.Mutex
	hqs     map[string]*hostQueue
	hostWg  sync.WaitGroup
}

func (s *scheduler) run(ctx context.Context, seeds []string) Stats {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.ctx = ctx
	go func() {
		<-ctx.Done()
		s.frontier.abort()
	}()

	// First pass: collect seed hosts for the lock.
	for _, raw := range seeds {
		if u, err := url.Parse(raw); err == nil && isHTTP(u) {
			s.seedHosts[u.Host] = struct{}{}
		}
	}
	for _, raw := range seeds {
		s.enqueue(raw, 0)
	}

	var wg sync.WaitGroup
	for i := 0; i < defaultWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.dispatcher(ctx)
		}()
	}
	wg.Wait()

	close(s.stop)
	s.hostWg.Wait()

	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	return s.stats
}

// enqueue validates raw against the scope rule and, after dedup, pushes it
// onto the frontier.
func (s *scheduler) enqueue(raw string, depth int) {
	u, err := url.Parse(raw)
	if err != nil || !isHTTP(u) {
		s.addStats(0, 0, 1, 0)
		return
	}
	u.Fragment = ""
	if s.rule.MaxDepth > 0 && depth > s.rule.MaxDepth {
		s.addStats(0, 0, 1, 0)
		return
	}
	if s.locked {
		if _, ok := s.seedHosts[u.Host]; !ok {
			s.addStats(0, 0, 1, 0)
			return
		}
	}
	if !pathInScope(u.Path, s.rule.PathPrefixInclude, s.rule.PathPrefixExclude) {
		s.addStats(0, 0, 1, 0)
		return
	}
	norm := u.String()
	s.visitedMu.Lock()
	if _, dup := s.visited[norm]; dup {
		s.visitedMu.Unlock()
		return
	}
	s.visited[norm] = struct{}{}
	s.visitedMu.Unlock()
	s.frontier.push(job{raw: norm, u: u, depth: depth})
}

// dispatcher pops frontier jobs, gates them on robots.txt and MaxPages, and
// routes them onto the per-host serial queue.
func (s *scheduler) dispatcher(ctx context.Context) {
	for {
		j, ok := s.frontier.pop()
		if !ok {
			return
		}
		s.addStats(1, 0, 0, 0)
		rb := s.robotsFor(ctx, j.u)
		if ctx.Err() != nil {
			s.frontier.finish()
			return
		}
		if !rb.Allowed(robotsPath(j.u), s.client.userAgent()) {
			s.addStats(0, 0, 1, 0)
			s.frontier.finish()
			continue
		}
		if !s.reserveDispatch() {
			s.addStats(0, 0, 1, 0)
			s.frontier.finish()
			continue
		}
		hq := s.hostQueueFor(j.u)
		select {
		case hq.ch <- j: // the host worker calls frontier.finish()
		case <-ctx.Done():
			s.frontier.finish()
			return
		}
	}
}

// hostWorker drains one host's queue serially, spacing fetches by the
// effective per-host delay.
func (s *scheduler) hostWorker(hq *hostQueue) {
	defer s.hostWg.Done()
	ctx := s.ctx
	var lastFetch time.Time
	for {
		select {
		case j := <-hq.ch:
			delay := s.effectiveDelay(ctx, j.u)
			if !lastFetch.IsZero() {
				if wait := delay - time.Since(lastFetch); wait > 0 {
					timer := time.NewTimer(wait)
					select {
					case <-timer.C:
					case <-ctx.Done():
						timer.Stop()
						s.frontier.finish()
						return
					}
				}
			}
			lastFetch = time.Now()
			s.process(ctx, j)
			s.frontier.finish()
		case <-ctx.Done():
			return
		case <-s.stop:
			return
		}
	}
}

// process fetches one page, hands it to the sink, and enqueues discovered
// links at depth+1.
func (s *scheduler) process(ctx context.Context, j job) {
	payload, err := s.client.FetchPage(ctx, j.raw)
	if err != nil {
		s.addStats(0, 0, 0, 1)
		return
	}
	s.addStats(0, 1, 0, 0)
	if err := s.sink(ctx, payload); err != nil {
		// Fetch worked but the consumer rejected the payload.
		s.addStats(0, -1, 0, 1)
	}
	if payload.MIME == "text/html" || payload.MIME == "application/xhtml+xml" {
		base := payload.FinalURL
		if base == "" {
			base = j.raw
		}
		for _, link := range extractLinks(payload.Body, base) {
			s.enqueue(link, j.depth+1)
		}
	}
}

// effectiveDelay returns the pacing delay for a URL's host: the configured
// PerHostDelay, raised to the robots.txt Crawl-delay when that is larger.
func (s *scheduler) effectiveDelay(ctx context.Context, u *url.URL) time.Duration {
	delay := s.delay
	if rb := s.robotsFor(ctx, u); rb != nil {
		if cd := rb.CrawlDelay(s.client.userAgent()); cd > delay {
			delay = cd
		}
	}
	return delay
}

// robotsFor returns the cached robots policy for the URL's origin, fetching
// and parsing <origin>/robots.txt once. Serialization is per origin (one
// origin's fetch never stalls others); duplicate fetches for the same origin
// are deduped by the per-origin lock. Fetch or parse trouble degrades to an
// empty policy (allow all), matching common practice for 4xx/5xx or
// unreachable sites.
func (s *scheduler) robotsFor(ctx context.Context, u *url.URL) *Robots {
	key := u.Scheme + "://" + u.Host

	s.robotsMu.Lock()
	if rb, ok := s.robots[key]; ok {
		s.robotsMu.Unlock()
		return rb
	}
	mu, ok := s.robotsLocks[key]
	if !ok {
		mu = &sync.Mutex{}
		s.robotsLocks[key] = mu
	}
	s.robotsMu.Unlock()

	mu.Lock()
	defer mu.Unlock()

	// Double-check: another caller may have populated the cache while we
	// waited on the per-origin lock.
	s.robotsMu.Lock()
	if rb, ok := s.robots[key]; ok {
		s.robotsMu.Unlock()
		return rb
	}
	s.robotsMu.Unlock()

	rb := &Robots{}
	robotsURL := key + "/robots.txt"
	if ctx.Err() == nil {
		if payload, err := s.client.FetchPage(ctx, robotsURL); err == nil {
			rb = ParseRobots(bytes.NewReader(payload.Body))
		}
	}

	s.robotsMu.Lock()
	s.robots[key] = rb
	s.robotsMu.Unlock()
	return rb
}

// reserveDispatch enforces MaxPages on dispatched fetches.
func (s *scheduler) reserveDispatch() bool {
	if s.rule.MaxPages <= 0 {
		return true
	}
	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	if s.dispatched >= s.rule.MaxPages {
		return false
	}
	s.dispatched++
	return true
}

// hostQueueFor returns the per-host queue, spawning its worker on first use.
func (s *scheduler) hostQueueFor(u *url.URL) *hostQueue {
	key := u.Scheme + "://" + u.Host
	s.hostsMu.Lock()
	defer s.hostsMu.Unlock()
	hq, ok := s.hqs[key]
	if !ok {
		hq = &hostQueue{key: key, ch: make(chan job, 8)}
		s.hqs[key] = hq
		s.hostWg.Add(1)
		go s.hostWorker(hq)
	}
	return hq
}

func (s *scheduler) addStats(visited, fetched, skipped, failed int) {
	s.statsMu.Lock()
	s.stats.Visited += visited
	s.stats.Fetched += fetched
	s.stats.Skipped += skipped
	s.stats.Failed += failed
	s.statsMu.Unlock()
}

func isHTTP(u *url.URL) bool {
	return u.Scheme == "http" || u.Scheme == "https"
}

// pathInScope applies the include/exclude path prefixes of the scope rule.
func pathInScope(path string, include, exclude []string) bool {
	for _, pre := range exclude {
		if pre != "" && strings.HasPrefix(path, pre) {
			return false
		}
	}
	if len(include) == 0 {
		return true
	}
	for _, pre := range include {
		if pre != "" && strings.HasPrefix(path, pre) {
			return true
		}
	}
	return false
}

// robotsPath returns the path a robots policy is matched against.
func robotsPath(u *url.URL) string {
	if p := u.EscapedPath(); p != "" {
		return p
	}
	return "/"
}

// extractLinks returns the absolute http(s) targets of every <a href> in
// body, resolved against base (typically the payload's final URL).
func extractLinks(body []byte, base string) []string {
	baseURL, err := url.Parse(base)
	if err != nil {
		return nil
	}
	root, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if !strings.EqualFold(attr.Key, "href") {
					continue
				}
				href := strings.TrimSpace(attr.Val)
				if href == "" || strings.HasPrefix(href, "#") {
					continue
				}
				switch lower := strings.ToLower(href); {
				case strings.HasPrefix(lower, "javascript:"),
					strings.HasPrefix(lower, "mailto:"),
					strings.HasPrefix(lower, "tel:"),
					strings.HasPrefix(lower, "data:"):
					continue
				}
				ref, err := url.Parse(href)
				if err != nil {
					continue
				}
				abs := baseURL.ResolveReference(ref)
				if !isHTTP(abs) {
					continue
				}
				s := abs.String()
				if _, dup := seen[s]; !dup {
					seen[s] = struct{}{}
					out = append(out, s)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

// frontier is an unbounded multi-producer/multi-consumer queue whose pop
// terminates once it is drained and no job is still being processed.
type frontier struct {
	mu       sync.Mutex
	cond     *sync.Cond
	items    []job
	inflight int
	closed   bool
}

func newFrontier() *frontier {
	f := &frontier{}
	f.cond = sync.NewCond(&f.mu)
	return f
}

func (f *frontier) push(j job) {
	f.mu.Lock()
	if !f.closed {
		f.items = append(f.items, j)
	}
	f.mu.Unlock()
	f.cond.Signal()
}

// pop returns the next job, or false once the queue is empty with no work
// in flight, or after abort.
func (f *frontier) pop() (job, bool) {
	f.mu.Lock()
	for {
		if len(f.items) > 0 {
			j := f.items[0]
			f.items = f.items[1:]
			f.inflight++
			f.mu.Unlock()
			return j, true
		}
		if f.closed || f.inflight == 0 {
			f.closed = true
			f.cond.Broadcast()
			f.mu.Unlock()
			return job{}, false
		}
		f.cond.Wait()
	}
}

// finish marks one popped job as fully processed.
func (f *frontier) finish() {
	f.mu.Lock()
	f.inflight--
	if f.inflight == 0 {
		f.cond.Broadcast()
	}
	f.mu.Unlock()
}

// abort closes the frontier and wakes every waiter.
func (f *frontier) abort() {
	f.mu.Lock()
	f.closed = true
	f.cond.Broadcast()
	f.mu.Unlock()
}
