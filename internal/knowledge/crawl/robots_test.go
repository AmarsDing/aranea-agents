package crawl_test

import (
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/knowledge/crawl"
)

func TestRobotsAllowedLongestPrefix(t *testing.T) {
	rb := crawl.ParseRobots(strings.NewReader(`
User-agent: *
Disallow: /admin
Allow: /admin/public
`))
	cases := []struct {
		path string
		want bool
	}{
		{"/admin/panel", false},
		{"/admin", false},
		{"/admin/public/x", true}, // longer Allow prefix wins
		{"/other", true},
	}
	for _, tc := range cases {
		if got := rb.Allowed(tc.path, "anybot/1.0"); got != tc.want {
			t.Errorf("Allowed(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestRobotsGroupSelection(t *testing.T) {
	rb := crawl.ParseRobots(strings.NewReader(`
User-agent: *
Disallow: /star-only

User-agent: newsbot
Disallow: /private

User-agent: denybot
Disallow: /

User-agent: shopbot
Disallow:
`))
	cases := []struct {
		path string
		ua   string
		want bool
	}{
		{"/star-only/x", "random/1.0", false}, // wildcard group applies
		{"/other", "random/1.0", true},        // no matching wildcard rule
		{"/anything", "denybot/2.0", false},   // exact group, disallow /
		{"/star-only/x", "newsbot/1.0", true}, // exact group beats wildcard
		{"/private/z", "NewsBot/1.0", false},  // newsbot's own rule applies
		{"/foo", "shopbot/1.0", true},         // empty Disallow = allow all
		{"/no-robots", "unknownthing", true},
	}
	for _, tc := range cases {
		if got := rb.Allowed(tc.path, tc.ua); got != tc.want {
			t.Errorf("Allowed(%q, %q) = %v, want %v", tc.path, tc.ua, got, tc.want)
		}
	}
}

func TestRobotsMultiAgentAndGroupBoundaries(t *testing.T) {
	// Two consecutive user-agent lines share one group; a user-agent line
	// after rules starts a new group.
	rb := crawl.ParseRobots(strings.NewReader(`
User-agent: aobot
User-agent: bobot
Disallow: /x

User-agent: cobot
Disallow: /y
`))
	cases := []struct {
		path string
		ua   string
		want bool
	}{
		{"/x/1", "aobot/1", false},
		{"/x/1", "bobot/1", false},
		{"/y/1", "aobot/1", true}, // aobot's group only denies /x
		{"/y/1", "cobot/1", false},
		{"/x/1", "cobot/1", true},
		{"/x/1", "unknown/1", true}, // no wildcard group anywhere
	}
	for _, tc := range cases {
		if got := rb.Allowed(tc.path, tc.ua); got != tc.want {
			t.Errorf("Allowed(%q, %q) = %v, want %v", tc.path, tc.ua, got, tc.want)
		}
	}
}

func TestRobotsSameLengthAllowWins(t *testing.T) {
	rb := crawl.ParseRobots(strings.NewReader(`
User-agent: *
Disallow: /x
Allow: /x
`))
	if !rb.Allowed("/x/y", "bot/1") {
		t.Error("same-length Allow should win over Disallow")
	}
}

func TestRobotsCommentsAndCase(t *testing.T) {
	rb := crawl.ParseRobots(strings.NewReader(`
# a comment
USER-AGENT: * # inline comment
DISALLOW: /tmp
`))
	if rb.Allowed("/tmp/a", "bot/1") {
		t.Error("case-insensitive field names expected to parse; /tmp should be denied")
	}
	if !rb.Allowed("/ok", "bot/1") {
		t.Error("/ok should be allowed")
	}
}

func TestRobotsCrawlDelay(t *testing.T) {
	rb := crawl.ParseRobots(strings.NewReader(`
User-agent: *
Crawl-delay: 2.5
Disallow: /tmp

User-agent: slowbot
Disallow: /
Crawl-delay: 0.5
`))
	if got := rb.CrawlDelay("anything/1.0"); got != 2500*time.Millisecond {
		t.Errorf("CrawlDelay(wildcard) = %v, want 2.5s", got)
	}
	if got := rb.CrawlDelay("slowbot/3"); got != 500*time.Millisecond {
		t.Errorf("CrawlDelay(slowbot) = %v, want 500ms", got)
	}
	if got := rb.CrawlDelay("nothing-matching"); got != 2500*time.Millisecond {
		t.Errorf("CrawlDelay(only wildcard matches) = %v, want 2.5s", got)
	}
}

func TestRobotsEmptyPolicyAllowsAll(t *testing.T) {
	for name, rb := range map[string]*crawl.Robots{
		"nil reader":  crawl.ParseRobots(nil),
		"empty input": crawl.ParseRobots(strings.NewReader("")),
		"garbage":     crawl.ParseRobots(strings.NewReader("<html><body>no colon here</body></html>")),
	} {
		if !rb.Allowed("/anything", "anybot/1.0") {
			t.Errorf("%s: empty policy must allow all", name)
		}
	}
}
