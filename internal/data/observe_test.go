package data

import (
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

func TestObserveQuery_Success(t *testing.T) {
	lg := loggateway.NewNoop()
	called := false
	err := observeQuery(lg, "session_repo", "get", func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !called {
		t.Fatal("expected fn to be called")
	}
}

func TestObserveQuery_Error(t *testing.T) {
	lg := loggateway.NewNoop()
	inner := errors.New("db error")
	err := observeQuery(lg, "session_repo", "save", func() error {
		return inner
	})
	if err != inner {
		t.Fatalf("expected inner error, got %v", err)
	}
}

func TestObserveQuery_SlowQueryThreshold(t *testing.T) {
	lg := loggateway.NewNoop()
	// Operation that takes longer than 100ms should be logged as slow.
	// We can't easily verify the log output, but we verify the function
	// completes without panic and returns the correct error.
	err := observeQuery(lg, "session_repo", "heavy_query", func() error {
		time.Sleep(110 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestObserveQuery_FastQuery(t *testing.T) {
	lg := loggateway.NewNoop()
	err := observeQuery(lg, "agent_repo", "list", func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
