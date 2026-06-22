package tools

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// TestBatchExecuteSpiritTools_ParallelFasterThanSerial verifies that 5
// independent calls (each sleeping 80ms) complete in less than 40% of the
// serial time when a ParallelToolExecutor is supplied. This is the B5
// acceptance criterion from the integration plan.
func TestBatchExecuteSpiritTools_ParallelFasterThanSerial(t *testing.T) {
	const (
		numCalls    = 5
		callDelay   = 80 * time.Millisecond
		serialTotal = numCalls * callDelay   // 400ms
		parallelMax = serialTotal * 40 / 100 // 160ms (40% threshold)
	)

	var active int32
	var maxActive int32
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		cur := atomic.AddInt32(&active, 1)
		for {
			old := atomic.LoadInt32(&maxActive)
			if cur <= old || atomic.CompareAndSwapInt32(&maxActive, old, cur) {
				break
			}
		}
		select {
		case <-time.After(callDelay):
		case <-ctx.Done():
			return ToolResult{CallID: call.ID, Name: call.Name, Success: false, Error: ctx.Err().Error()}
		}
		atomic.AddInt32(&active, -1)
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true, Output: "ok"}
	}

	exec := NewParallelToolExecutor(nil, loggateway.NewNoop(),
		WithMaxConcurrency(numCalls))

	calls := make([]ToolCall, numCalls)
	for i := range calls {
		calls[i] = ToolCall{ID: string(rune('a' + i)), Name: "slow"}
	}

	start := time.Now()
	results := BatchExecuteSpiritTools(context.Background(), exec, handler, calls, loggateway.NewNoop())
	elapsed := time.Since(start)

	if len(results) != numCalls {
		t.Fatalf("expected %d results, got %d", numCalls, len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("result %s was not successful: %s", r.CallID, r.Error)
		}
	}
	if elapsed >= parallelMax {
		t.Errorf("parallel execution took %v, expected < %v (40%% of serial %v)",
			elapsed, parallelMax, serialTotal)
	}
	if got := atomic.LoadInt32(&maxActive); got < 2 {
		t.Errorf("expected concurrent execution, max active = %d", got)
	}
}

// TestBatchExecuteSpiritTools_NilExecutorFallsBackToSerial verifies that when
// the ParallelToolExecutor is nil, calls run serially (no concurrency).
func TestBatchExecuteSpiritTools_NilExecutorFallsBackToSerial(t *testing.T) {
	const callDelay = 40 * time.Millisecond

	var active int32
	var maxActive int32
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		cur := atomic.AddInt32(&active, 1)
		for {
			old := atomic.LoadInt32(&maxActive)
			if cur <= old || atomic.CompareAndSwapInt32(&maxActive, old, cur) {
				break
			}
		}
		time.Sleep(callDelay)
		atomic.AddInt32(&active, -1)
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true}
	}

	calls := []ToolCall{
		{ID: "a", Name: "x"},
		{ID: "b", Name: "y"},
		{ID: "c", Name: "z"},
	}

	results := BatchExecuteSpiritTools(context.Background(), nil, handler, calls, loggateway.NewNoop())

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("result %s was not successful: %s", r.CallID, r.Error)
		}
	}
	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Errorf("expected max concurrency 1 for serial fallback, got %d", got)
	}
}

// TestBatchExecuteSpiritTools_CycleFallsBackToSerial verifies that when
// ParallelToolExecutor.Execute returns an error (e.g., dependency cycle), the
// function falls back to serial execution and still returns results for all
// calls.
func TestBatchExecuteSpiritTools_CycleFallsBackToSerial(t *testing.T) {
	var mu sync.Mutex
	var order []string
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		mu.Lock()
		order = append(order, call.ID)
		mu.Unlock()
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true, Output: "ok"}
	}

	exec := NewParallelToolExecutor(handler, loggateway.NewNoop())

	calls := []ToolCall{
		{ID: "a", Name: "x", DependsOn: []string{"b"}},
		{ID: "b", Name: "y", DependsOn: []string{"a"}},
	}

	results := BatchExecuteSpiritTools(context.Background(), exec, handler, calls, loggateway.NewNoop())

	if len(results) != 2 {
		t.Fatalf("expected 2 results after fallback, got %d", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("result %s was not successful after fallback: %s", r.CallID, r.Error)
		}
	}
}

// TestBatchExecuteSpiritTools_EmptyInputReturnsNil verifies the no-op path.
func TestBatchExecuteSpiritTools_EmptyInputReturnsNil(t *testing.T) {
	exec := NewParallelToolExecutor(nil, loggateway.NewNoop())
	results := BatchExecuteSpiritTools(context.Background(), exec, nil, nil, loggateway.NewNoop())
	if results != nil {
		t.Errorf("expected nil results for empty input, got %v", results)
	}
}

// TestBatchExecuteSpiritTools_NilHandlerWithExecutor verifies that when the
// handler is nil but the executor is non-nil, the function falls back to serial
// execution which produces failure results (no panic).
func TestBatchExecuteSpiritTools_NilHandlerWithExecutor(t *testing.T) {
	exec := NewParallelToolExecutor(nil, loggateway.NewNoop())
	calls := []ToolCall{{ID: "a", Name: "x"}}

	results := BatchExecuteSpiritTools(context.Background(), exec, nil, calls, loggateway.NewNoop())

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("expected failure when handler is nil")
	}
}

// TestBatchExecuteSpiritTools_ContextCancelAbortsSerial verifies that context
// cancellation during serial fallback stops further calls and marks the
// in-flight call as failed.
func TestBatchExecuteSpiritTools_ContextCancelAbortsSerial(t *testing.T) {
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		select {
		case <-time.After(100 * time.Millisecond):
			return ToolResult{CallID: call.ID, Name: call.Name, Success: true}
		case <-ctx.Done():
			return ToolResult{CallID: call.ID, Name: call.Name, Success: false, Error: ctx.Err().Error()}
		}
	}

	calls := []ToolCall{
		{ID: "a", Name: "x"},
		{ID: "b", Name: "y"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	results := BatchExecuteSpiritTools(ctx, nil, handler, calls, loggateway.NewNoop())

	// Serial fallback: first call hits the deadline, second is skipped.
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	for _, r := range results {
		if r.Success {
			t.Errorf("result %s should not be successful after cancel", r.CallID)
		}
	}
}

// TestBatchExecuteSpiritTools_DependencyOrderPreserved verifies that when
// calls have dependencies, the parallel executor respects the topological
// order: "a" runs before "b" which depends on it.
func TestBatchExecuteSpiritTools_DependencyOrderPreserved(t *testing.T) {
	const callDelay = 30 * time.Millisecond

	var mu sync.Mutex
	var order []string
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		mu.Lock()
		order = append(order, call.ID)
		mu.Unlock()
		time.Sleep(callDelay)
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true}
	}

	exec := NewParallelToolExecutor(handler, loggateway.NewNoop())

	calls := []ToolCall{
		{ID: "a", Name: "first"},
		{ID: "b", Name: "second", DependsOn: []string{"a"}},
	}

	results := BatchExecuteSpiritTools(context.Background(), exec, handler, calls, loggateway.NewNoop())

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Errorf("expected order [a, b], got %v", order)
	}
}

// TestExecuteToolCallsSerial_NilHandlerProducesFailure verifies the serial
// helper returns failure results (not panics) when handler is nil.
func TestExecuteToolCallsSerial_NilHandlerProducesFailure(t *testing.T) {
	calls := []ToolCall{
		{ID: "a", Name: "x"},
		{ID: "b", Name: "y"},
	}
	results := executeToolCallsSerial(context.Background(), nil, calls, loggateway.NewNoop())
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Success {
			t.Error("expected failure for nil handler")
		}
		if r.Error == "" {
			t.Error("expected non-empty error message")
		}
	}
}
