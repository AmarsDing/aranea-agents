package tools

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// slowHandler returns a handler that sleeps for the given duration and records
// the call ID into the provided slice (under mutex). Useful for verifying
// parallelism: if N calls run in parallel, total wall time ≈ duration (not N*duration).
func slowHandler(d time.Duration, recorded *[]string, mu *sync.Mutex) ToolHandler {
	return func(ctx context.Context, call ToolCall) ToolResult {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return ToolResult{CallID: call.ID, Name: call.Name, Success: false, Error: ctx.Err().Error()}
		}
		if mu != nil && recorded != nil {
			mu.Lock()
			*recorded = append(*recorded, call.ID)
			mu.Unlock()
		}
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true, Output: "ok"}
	}
}

// TestParallelExecutor_IndependentCallsRunInParallel verifies that 5
// independent calls (each sleeping 80ms) complete in less than 40% of the
// serial time (5*80ms = 400ms; 40% = 160ms). This is the AC-2 acceptance criterion.
func TestParallelExecutor_IndependentCallsRunInParallel(t *testing.T) {
	const (
		numCalls    = 5
		callDelay   = 80 * time.Millisecond
		serialTotal = numCalls * callDelay   // 400ms
		parallelMax = serialTotal * 40 / 100 // 160ms (40% threshold)
	)

	var mu sync.Mutex
	var recorded []string
	handler := slowHandler(callDelay, &recorded, &mu)

	exec := NewParallelToolExecutor(handler, loggateway.NewNoop(),
		WithMaxConcurrency(numCalls))

	calls := make([]ToolCall, numCalls)
	for i := range calls {
		calls[i] = ToolCall{ID: string(rune('a' + i)), Name: "slow"}
	}

	start := time.Now()
	results, err := exec.Execute(context.Background(), calls)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
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
	if len(recorded) != numCalls {
		t.Errorf("expected %d recorded calls, got %d", numCalls, len(recorded))
	}
}

// TestParallelExecutor_LayersRunSequentially verifies that a -> b dependency
// chain forces sequential execution: layer [a] runs first, then layer [b].
// Total time should be approximately 2*delay, not delay.
func TestParallelExecutor_LayersRunSequentially(t *testing.T) {
	const callDelay = 50 * time.Millisecond

	var mu sync.Mutex
	var order []string
	handler := slowHandler(callDelay, &order, &mu)

	exec := NewParallelToolExecutor(handler, loggateway.NewNoop())

	calls := []ToolCall{
		{ID: "a", Name: "first"},
		{ID: "b", Name: "second", DependsOn: []string{"a"}},
	}

	start := time.Now()
	_, err := exec.Execute(context.Background(), calls)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	// Sequential: 2 * 50ms = 100ms. Allow some scheduling slack.
	if elapsed < 2*callDelay-10*time.Millisecond {
		t.Errorf("layers ran too fast (%v), expected sequential ~%v",
			elapsed, 2*callDelay)
	}
	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Errorf("expected order [a, b], got %v", order)
	}
}

// TestParallelExecutor_DiamondRunsInTwoPhases verifies the diamond DAG
// a -> (b, c) -> d: layer 0 = [a], layer 1 = [b, c] (parallel), layer 2 = [d].
// Total time ≈ 3 * delay (a + max(b,c) + d), not 4 * delay.
func TestParallelExecutor_DiamondRunsInTwoPhases(t *testing.T) {
	const callDelay = 50 * time.Millisecond

	var mu sync.Mutex
	var order []string
	handler := slowHandler(callDelay, &order, &mu)

	exec := NewParallelToolExecutor(handler, loggateway.NewNoop(),
		WithMaxConcurrency(4))

	calls := []ToolCall{
		{ID: "a", Name: "root"},
		{ID: "b", Name: "left", DependsOn: []string{"a"}},
		{ID: "c", Name: "right", DependsOn: []string{"a"}},
		{ID: "d", Name: "join", DependsOn: []string{"b", "c"}},
	}

	start := time.Now()
	results, err := exec.Execute(context.Background(), calls)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	// 3 layers * 50ms = 150ms. b and c run in parallel so they count once.
	// Allow scheduling slack.
	if elapsed < 3*callDelay-15*time.Millisecond {
		t.Errorf("diamond ran too fast (%v), expected ~%v", elapsed, 3*callDelay)
	}
	if elapsed >= 4*callDelay {
		t.Errorf("diamond ran too slow (%v), expected < %v (b,c parallel)",
			elapsed, 4*callDelay)
	}
}

// TestParallelExecutor_HandlerErrorPropagatesAsFailedResult verifies that a
// handler returning a failed result does NOT abort the whole batch; the
// failure is recorded in the result and other calls still complete.
func TestParallelExecutor_HandlerErrorPropagatesAsFailedResult(t *testing.T) {
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		if call.ID == "fail" {
			return ToolResult{CallID: call.ID, Name: call.Name, Success: false, Error: "boom"}
		}
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true, Output: "ok"}
	}

	exec := NewParallelToolExecutor(handler, loggateway.NewNoop())
	calls := []ToolCall{
		{ID: "ok1", Name: "x"},
		{ID: "fail", Name: "y"},
		{ID: "ok2", Name: "z"},
	}

	results, err := exec.Execute(context.Background(), calls)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	byID := make(map[string]ToolResult, len(results))
	for _, r := range results {
		byID[r.CallID] = r
	}
	if byID["fail"].Success {
		t.Error("expected 'fail' result to be unsuccessful")
	}
	if byID["fail"].Error != "boom" {
		t.Errorf("expected error 'boom', got %q", byID["fail"].Error)
	}
	if !byID["ok1"].Success || !byID["ok2"].Success {
		t.Error("expected 'ok1' and 'ok2' to succeed despite sibling failure")
	}
}

// TestParallelExecutor_ContextCancelAbortsExecution verifies that cancelling
// the context before completion returns ctx.Err() and partial/empty results.
func TestParallelExecutor_ContextCancelAbortsExecution(t *testing.T) {
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		select {
		case <-time.After(200 * time.Millisecond):
			return ToolResult{CallID: call.ID, Name: call.Name, Success: true}
		case <-ctx.Done():
			return ToolResult{CallID: call.ID, Name: call.Name, Success: false, Error: ctx.Err().Error()}
		}
	}

	exec := NewParallelToolExecutor(handler, loggateway.NewNoop())
	calls := []ToolCall{
		{ID: "a", Name: "x"},
		{ID: "b", Name: "y"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	results, err := exec.Execute(ctx, calls)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.DeadlineExceeded or Canceled, got %v", err)
	}
	// Results may be partial; whatever is returned must have Success=false.
	for _, r := range results {
		if r.Success {
			t.Errorf("result %s should not be successful after cancel", r.CallID)
		}
	}
}

// TestParallelExecutor_EmptyInputReturnsNil verifies the no-op path.
func TestParallelExecutor_EmptyInputReturnsNil(t *testing.T) {
	exec := NewParallelToolExecutor(nil, loggateway.NewNoop())
	results, err := exec.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty input, got %v", results)
	}
}

// TestParallelExecutor_RespectsMaxConcurrency verifies that with
// maxConcurrency=1, calls effectively run serially (no parallelism).
func TestParallelExecutor_RespectsMaxConcurrency(t *testing.T) {
	const callDelay = 40 * time.Millisecond

	var active int32
	var maxActive int32
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		cur := atomic.AddInt32(&active, 1)
		// Track high-water mark of concurrent executions.
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

	exec := NewParallelToolExecutor(handler, loggateway.NewNoop(),
		WithMaxConcurrency(1))

	calls := []ToolCall{
		{ID: "a", Name: "x"},
		{ID: "b", Name: "y"},
		{ID: "c", Name: "z"},
	}

	_, err := exec.Execute(context.Background(), calls)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Errorf("expected max concurrency 1, got %d", got)
	}
}

// TestParallelExecutor_ParallelWithinLayerTracksConcurrency verifies that
// with maxConcurrency=4 and 4 independent calls, all 4 run concurrently.
func TestParallelExecutor_ParallelWithinLayerTracksConcurrency(t *testing.T) {
	const callDelay = 50 * time.Millisecond

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

	exec := NewParallelToolExecutor(handler, loggateway.NewNoop(),
		WithMaxConcurrency(4))

	calls := []ToolCall{
		{ID: "a", Name: "x"},
		{ID: "b", Name: "y"},
		{ID: "c", Name: "z"},
		{ID: "d", Name: "w"},
	}

	_, err := exec.Execute(context.Background(), calls)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := atomic.LoadInt32(&maxActive); got != 4 {
		t.Errorf("expected max concurrency 4, got %d", got)
	}
}

// TestParallelExecutor_NilHandlerReturnsFailedResult verifies that when no
// handler is configured, direct-execution calls return a clear error.
func TestParallelExecutor_NilHandlerReturnsFailedResult(t *testing.T) {
	exec := NewParallelToolExecutor(nil, loggateway.NewNoop())
	calls := []ToolCall{{ID: "a", Name: "x"}}

	results, err := exec.Execute(context.Background(), calls)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("expected failure when handler is nil")
	}
	if results[0].Error == "" {
		t.Error("expected non-empty error message")
	}
}

// TestIsolationStrategyForTool verifies that file-write tools (canonical names
// and UI aliases) are tagged for worktree isolation while read-only and
// unrelated tools execute directly (Phase C).
func TestIsolationStrategyForTool(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		// Write-capable file tools (canonical).
		{"save_file", IsolationStrategyWorktree},
		{"diff_edit", IsolationStrategyWorktree},
		{"patch_file", IsolationStrategyWorktree},
		{"replace_content", IsolationStrategyWorktree},
		// UI aliases must resolve to the same strategy.
		{"write_file", IsolationStrategyWorktree},
		{"edit_file", IsolationStrategyWorktree},
		// Read-only file tools execute directly.
		{"read_file", ""},
		{"read_multiple_files", ""},
		{"list_file", ""},
		{"search_file", ""},
		{"search_content", ""},
		// Unrelated tools and empty input execute directly.
		{"web_research", ""},
		{"shell_exec", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := IsolationStrategyForTool(c.name); got != c.want {
			t.Errorf("IsolationStrategyForTool(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestParallelExecutor_CycleReturnsError verifies that a cyclic dependency
// causes Execute to return an error without invoking the handler.
func TestParallelExecutor_CycleReturnsError(t *testing.T) {
	called := false
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		called = true
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true}
	}

	exec := NewParallelToolExecutor(handler, loggateway.NewNoop())
	calls := []ToolCall{
		{ID: "a", Name: "x", DependsOn: []string{"b"}},
		{ID: "b", Name: "y", DependsOn: []string{"a"}},
	}

	_, err := exec.Execute(context.Background(), calls)
	if err == nil {
		t.Fatal("expected error for cycle, got nil")
	}
	if !errors.Is(err, ErrCycleDetected) {
		t.Errorf("expected ErrCycleDetected, got %v", err)
	}
	if called {
		t.Error("handler should not be called when cycle is detected")
	}
}
