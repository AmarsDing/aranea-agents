package safego_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/safego"
)

func TestGoRunsFunction(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	called := false
	safego.Go(context.Background(), "test", func() {
		defer wg.Done()
		called = true
	})
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not complete in time")
	}
	if !called {
		t.Fatal("expected function to be called")
	}
}

func TestGoRecoversPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	// This should not cause the test to panic.
	safego.Go(context.Background(), "panic-test", func() {
		defer wg.Done()
		panic("intentional test panic")
	})
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not complete in time")
	}
}
