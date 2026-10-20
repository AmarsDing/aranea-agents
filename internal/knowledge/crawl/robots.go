package crawl

import (
	"bufio"
	"io"
	"strconv"
	"strings"
	"time"
)

// Robots holds the parsed rule groups of a robots.txt file.
// It implements an RFC 9309 subset: User-agent groups ("*" wildcard and
// exact product token), Allow/Disallow with longest-prefix wins, empty
// Disallow meaning "allow all", and Crawl-delay recording.
type Robots struct {
	groups []*robotsGroup
}

type robotsGroup struct {
	agents     []string // lowercased user-agent values
	rules      []robotsRule
	hasRules   bool
	crawlDelay time.Duration
}

type robotsRule struct {
	allow bool
	path  string // path prefix, compared verbatim
}

// ParseRobots reads robots.txt content. A nil reader (or unreadable
// content) yields an empty set, which allows everything.
func ParseRobots(r io.Reader) *Robots {
	rb := &Robots{}
	if r == nil {
		return rb
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var cur *robotsGroup
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			// A user-agent line after rule lines starts a new group.
			if cur == nil || cur.hasRules {
				cur = &robotsGroup{}
				rb.groups = append(rb.groups, cur)
			}
			cur.agents = append(cur.agents, strings.ToLower(value))
		case "allow":
			if cur != nil && value != "" {
				cur.rules = append(cur.rules, robotsRule{allow: true, path: value})
				cur.hasRules = true
			}
		case "disallow":
			if cur != nil {
				cur.hasRules = true
				// Empty Disallow means "allow everything"; skip the rule.
				if value != "" {
					cur.rules = append(cur.rules, robotsRule{allow: false, path: value})
				}
			}
		case "crawl-delay":
			if cur != nil {
				if secs, err := strconv.ParseFloat(value, 64); err == nil && secs > 0 {
					cur.crawlDelay = time.Duration(secs * float64(time.Second))
				}
			}
		}
	}
	return rb
}

// Allowed reports whether the robots policy permits fetching path with the
// given user-agent. With no matching group, or no matching rule, access is
// allowed. For same-length Allow/Disallow prefixes, Allow wins (the less
// restrictive rule per RFC 9309).
func (rb *Robots) Allowed(path, userAgent string) bool {
	g := rb.groupFor(userAgent)
	if g == nil {
		return true
	}
	bestLen := -1
	bestAllow := true
	for _, rule := range g.rules {
		if !strings.HasPrefix(path, rule.path) {
			continue
		}
		l := len(rule.path)
		if l > bestLen || (l == bestLen && rule.allow) {
			bestLen = l
			bestAllow = rule.allow
		}
	}
	if bestLen < 0 {
		return true
	}
	return bestAllow
}

// CrawlDelay returns the crawl-delay recorded for the group matching
// userAgent, or zero when none is set.
func (rb *Robots) CrawlDelay(userAgent string) time.Duration {
	g := rb.groupFor(userAgent)
	if g == nil {
		return 0
	}
	return g.crawlDelay
}

// groupFor selects the group whose user-agent line prefix-matches the
// caller's product token per RFC 9309 §2.2.2 (longest agent value wins);
// when no group prefix-matches it falls back to the "*" wildcard group.
func (rb *Robots) groupFor(userAgent string) *robotsGroup {
	if rb == nil {
		return nil
	}
	token := agentToken(userAgent)
	var best *robotsGroup
	bestScore := -1
	for _, g := range rb.groups {
		for _, a := range g.agents {
			if a != "*" && strings.HasPrefix(token, a) && len(a) > bestScore {
				bestScore = len(a)
				best = g
			}
		}
	}
	if best != nil {
		return best
	}
	for _, g := range rb.groups {
		for _, a := range g.agents {
			if a == "*" {
				return g
			}
		}
	}
	return nil
}

// agentToken reduces a full User-Agent header to its product token: the
// text up to the first "/", whitespace or separator, lowercased.
func agentToken(userAgent string) string {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	if i := strings.IndexAny(ua, "/ \t"); i > 0 {
		return ua[:i]
	}
	return ua
}
