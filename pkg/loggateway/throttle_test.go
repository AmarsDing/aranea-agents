package loggateway

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestThrottle_Allow(t *testing.T) {
	th := NewThrottle(50 * time.Millisecond)
	if ok, sup := th.Allow(); !ok || sup != 0 {
		t.Fatalf("first call: ok=%v sup=%d, want ok=true sup=0", ok, sup)
	}
	if ok, _ := th.Allow(); ok {
		t.Fatal("second call within window must be suppressed")
	}
	if ok, _ := th.Allow(); ok {
		t.Fatal("third call within window must be suppressed")
	}
	time.Sleep(80 * time.Millisecond)
	ok, sup := th.Allow()
	if !ok {
		t.Fatal("call after window must be allowed")
	}
	if sup != 2 {
		t.Fatalf("suppressed = %d, want 2", sup)
	}
	// The window restarted; the next call is suppressed again and the
	// suppressed counter was reset by the allowed call.
	if ok, _ := th.Allow(); ok {
		t.Fatal("call within new window must be suppressed")
	}
}

func TestThrottle_NilAllowsEverything(t *testing.T) {
	var th *Throttle
	for i := 0; i < 3; i++ {
		if ok, sup := th.Allow(); !ok || sup != 0 {
			t.Fatalf("nil throttle: ok=%v sup=%d, want ok=true sup=0", ok, sup)
		}
	}
}

func TestThrottle_Concurrent(t *testing.T) {
	th := NewThrottle(time.Hour) // never expires during the test
	var allowed atomic.Int64
	done := make(chan struct{})
	for i := 0; i < 16; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				if ok, _ := th.Allow(); ok {
					allowed.Add(1)
				}
			}
		}()
	}
	for i := 0; i < 16; i++ {
		<-done
	}
	if got := allowed.Load(); got != 1 {
		t.Fatalf("allowed = %d, want exactly 1 under contention", got)
	}
}

func TestThrottle_ZeroIntervalDefaults(t *testing.T) {
	th := NewThrottle(0)
	if ok, _ := th.Allow(); !ok {
		t.Fatal("first call must be allowed")
	}
	if ok, _ := th.Allow(); ok {
		t.Fatal("zero interval must default to a positive window")
	}
}
