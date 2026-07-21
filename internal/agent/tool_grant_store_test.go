package agent

import (
	"testing"
	"time"
)

func TestToolGrantStore_SessionGrantRoundTrip(t *testing.T) {
	t.Parallel()
	now := time.Now()
	s := newToolGrantStore(func() time.Time { return now })
	s.GrantSession("sess-1", "agent-1", "bash")
	if !s.HasSession("sess-1", "agent-1", "bash") {
		t.Fatal("expected session grant to be present")
	}
}

func TestToolGrantStore_Isolation(t *testing.T) {
	t.Parallel()
	now := time.Now()
	s := newToolGrantStore(func() time.Time { return now })
	s.GrantSession("sess-1", "agent-1", "bash")
	// Cross-session leakage must not happen: a grant made in one session
	// must never apply to another session, agent, or tool. (Regression
	// guard for the earlier cross-session state pollution lesson.)
	for _, tc := range []struct{ sess, agent, tool string }{
		{"sess-2", "agent-1", "bash"},
		{"sess-1", "agent-2", "bash"},
		{"sess-1", "agent-1", "file_save"},
	} {
		if s.HasSession(tc.sess, tc.agent, tc.tool) {
			t.Fatalf("unexpected grant hit for (%q,%q,%q)", tc.sess, tc.agent, tc.tool)
		}
	}
}

func TestToolGrantStore_TTLExpiry(t *testing.T) {
	t.Parallel()
	now := time.Now()
	s := newToolGrantStore(func() time.Time { return now })
	s.GrantSession("sess-1", "agent-1", "bash")
	// Advance the clock beyond the TTL: grant must be treated as expired
	// and lazily removed.
	now = now.Add(toolSessionGrantTTL + time.Minute)
	if s.HasSession("sess-1", "agent-1", "bash") {
		t.Fatal("expected session grant to expire after TTL")
	}
	// Expired entry must have been lazily evicted.
	s.mu.Lock()
	remaining := len(s.sessionGrants)
	s.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("expected lazy eviction, %d entries remain", remaining)
	}
}

func TestToolGrantStore_EmptyKeysIgnored(t *testing.T) {
	t.Parallel()
	s := newToolGrantStore(time.Now)
	s.GrantSession("", "agent-1", "bash")
	s.GrantSession("sess-1", "", "bash")
	s.GrantSession("sess-1", "agent-1", "")
	if s.HasSession("", "agent-1", "bash") || s.HasSession("sess-1", "", "bash") || s.HasSession("sess-1", "agent-1", "") {
		t.Fatal("empty keys must never be granted")
	}
}
