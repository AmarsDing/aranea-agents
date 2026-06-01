package metrics

import (
	"errors"
	"testing"
	"time"
)

func TestIsBlockedErr_Nil(t *testing.T) {
	if IsBlockedErr(nil) {
		t.Fatal("nil error should not be blocked")
	}
}

func TestIsBlockedErr_HookBlocked(t *testing.T) {
	if !IsBlockedErr(errors.New("HOOK_BLOCKED by policy")) {
		t.Fatal("HOOK_BLOCKED should be blocked")
	}
}

func TestIsBlockedErr_Blocked(t *testing.T) {
	if !IsBlockedErr(errors.New("request blocked")) {
		t.Fatal("blocked should be blocked")
	}
}

func TestIsBlockedErr_Forbidden(t *testing.T) {
	if !IsBlockedErr(errors.New("FORBIDDEN access")) {
		t.Fatal("FORBIDDEN should be blocked")
	}
}

func TestIsBlockedErr_Other(t *testing.T) {
	if IsBlockedErr(errors.New("connection refused")) {
		t.Fatal("other error should not be blocked")
	}
}

func TestObserveCallback_NoError(t *testing.T) {
	start := time.Now().Add(-100 * time.Millisecond)
	ObserveCallback("test", "before_turn", start, nil)
}

func TestObserveCallback_WithError(t *testing.T) {
	start := time.Now().Add(-50 * time.Millisecond)
	ObserveCallback("test", "after_turn", start, errors.New("timeout"))
}

func TestObserveCallback_BlockedError(t *testing.T) {
	start := time.Now()
	ObserveCallback("plugin", "pre_send", start, errors.New("HOOK_BLOCKED"))
}

func TestContainsAny_Match(t *testing.T) {
	if !containsAny("hello world", "world") {
		t.Fatal("should find substring")
	}
}

func TestContainsAny_NoMatch(t *testing.T) {
	if containsAny("hello", "world") {
		t.Fatal("should not find substring")
	}
}

func TestContainsAny_EmptySub(t *testing.T) {
	if containsAny("hello", "") {
		t.Fatal("empty substring should not match")
	}
}

func TestContainsAny_EmptyString(t *testing.T) {
	if containsAny("", "hello") {
		t.Fatal("empty string should not match non-empty sub")
	}
}

func TestContainsAny_MultipleSubs(t *testing.T) {
	if !containsAny("hello", "xyz", "hel") {
		t.Fatal("should find second substring")
	}
}
