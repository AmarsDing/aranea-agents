package tools

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/tools/alias"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// IsolationStrategy constants define how a tool call is isolated from others.
const (
	IsolationStrategyWorktree    = "worktree"
	IsolationStrategyTransaction = "transaction"
)

// fileWriteToolNames lists the canonical file-tool names that mutate the
// workspace. Parallel calls to these tools must be isolated in a git worktree
// so concurrent edits merge cleanly instead of corrupting each other.
// Read-only file tools (read_file, list_file, search_*) are excluded.
var fileWriteToolNames = map[string]struct{}{
	"save_file":       {},
	"diff_edit":       {},
	"patch_file":      {},
	"replace_content": {},
}

// IsolationStrategyForTool returns the isolation strategy for a tool name.
// File-write tools (canonical name or UI alias such as write_file/edit_file)
// are tagged IsolationStrategyWorktree; all other tools return "" (direct
// execution). ToolCall construction sites use this as the single tagging
// point so classification stays consistent (Phase C).
func IsolationStrategyForTool(toolName string) string {
	name := strings.TrimSpace(toolName)
	if canonical, ok := alias.RuntimeToolNameAliases[name]; ok {
		name = canonical
	}
	if _, ok := fileWriteToolNames[name]; ok {
		return IsolationStrategyWorktree
	}
	return ""
}

// ToolCall represents a single tool invocation request for the parallel executor.
// ID is the unique identifier used by DependsOn to express ordering constraints.
// IsolationStrategy selects how the call is executed:
//   - "" (empty): direct execution via the registered handler
//   - "worktree": file operations isolated in a git worktree
//   - "transaction": DB operations wrapped in a transaction sandbox
type ToolCall struct {
	ID                string
	Name              string
	Arguments         json.RawMessage
	DependsOn         []string
	IsolationStrategy string
}

// ToolResult represents the outcome of a ToolCall execution.
type ToolResult struct {
	CallID     string
	Name       string
	Success    bool
	Output     string
	Error      string
	DurationMS int64
}

// ToolHandler is the function signature for executing a single tool call
// directly (no isolation). Implementations must respect ctx cancellation.
type ToolHandler func(ctx context.Context, call ToolCall) ToolResult

// ExecutorOption configures a ParallelToolExecutor during construction.
type ExecutorOption func(*ParallelToolExecutor)

// WithMaxConcurrency sets the maximum number of concurrent tool executions
// within a single topological layer. Defaults to runtime.GOMAXPROCS(0).
func WithMaxConcurrency(n int) ExecutorOption {
	return func(e *ParallelToolExecutor) {
		if n > 0 {
			e.maxConcurrency = n
		}
	}
}

// WithWorktreeIsolator attaches a worktree isolator for file-operation tools.
func WithWorktreeIsolator(iso *WorktreeIsolator) ExecutorOption {
	return func(e *ParallelToolExecutor) { e.worktreeIso = iso }
}

// WithTransactionSandbox attaches a transaction sandbox for DB tools.
func WithTransactionSandbox(sandbox *TransactionSandbox) ExecutorOption {
	return func(e *ParallelToolExecutor) { e.txSandbox = sandbox }
}

// ParallelToolExecutor runs a batch of ToolCalls with Cursor-style parallelism:
// calls in the same dependency layer run concurrently, layers run sequentially.
type ParallelToolExecutor struct {
	depAnalyzer    *DependencyAnalyzer
	worktreeIso    *WorktreeIsolator
	txSandbox      *TransactionSandbox
	maxConcurrency int
	handler        ToolHandler
	lg             loggateway.Logger
}

// NewParallelToolExecutor creates a ParallelToolExecutor that delegates direct
// tool execution to handler. Isolation strategies (worktree/transaction) are
// only active when the corresponding isolator/sandbox is attached via options.
func NewParallelToolExecutor(handler ToolHandler, lg loggateway.Logger, opts ...ExecutorOption) *ParallelToolExecutor {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	e := &ParallelToolExecutor{
		depAnalyzer:    NewDependencyAnalyzer(),
		maxConcurrency: defaultConcurrency(),
		handler:        handler,
		lg:             lg,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(e)
		}
	}
	return e
}

// Execute runs all tool calls according to their dependency DAG. Calls within
// the same topological layer run concurrently (bounded by maxConcurrency);
// layers run sequentially. Returns the aggregated results in dependency order.
// Returns an error if dependency analysis fails (cycle, missing dep, etc.) or
// if ctx is cancelled before all layers complete.
func (e *ParallelToolExecutor) Execute(ctx context.Context, toolCalls []ToolCall) ([]ToolResult, error) {
	if e == nil {
		return nil, apierror.Internal(apierror.DomainTool, "parallel executor not initialized")
	}
	if len(toolCalls) == 0 {
		return nil, nil
	}

	dag, err := e.depAnalyzer.Analyze(toolCalls)
	if err != nil {
		return nil, err
	}

	layers, err := dag.TopologicalLayers()
	if err != nil {
		return nil, err
	}

	results := make([]ToolResult, 0, len(toolCalls))
	resultByCallID := make(map[string]ToolResult, len(toolCalls))
	for _, layer := range layers {
		if ctx.Err() != nil {
			return results, ctx.Err()
		}

		layerResults := make([]ToolResult, len(layer))
		runnable := make([]ToolCall, 0, len(layer))
		runnableIndexes := make([]int, 0, len(layer))
		for i, call := range layer {
			failedDependencies := make([]string, 0, len(call.DependsOn))
			for _, dependencyID := range call.DependsOn {
				if dependencyResult, ok := resultByCallID[dependencyID]; ok &&
					!dependencyResult.Success {
					failedDependencies = append(failedDependencies, dependencyID)
				}
			}
			if len(failedDependencies) > 0 {
				layerResults[i] = ToolResult{
					CallID: call.ID,
					Name:   call.Name,
					Error: "dependency failed: " +
						strings.Join(failedDependencies, ", "),
				}
				continue
			}
			runnable = append(runnable, call)
			runnableIndexes = append(runnableIndexes, i)
		}

		executedResults := e.executeLayer(ctx, runnable)
		for i, result := range executedResults {
			layerResults[runnableIndexes[i]] = result
		}
		results = append(results, layerResults...)
		for i, call := range layer {
			resultByCallID[call.ID] = layerResults[i]
		}
		// Re-check ctx after the layer: a cancellation during the layer
		// should surface as an error even if all calls returned a result.
		if ctx.Err() != nil {
			return results, ctx.Err()
		}
	}
	return results, nil
}

// executeLayer runs all calls in a single layer concurrently. Results are
// written to pre-allocated slots (no shared-slice mutation, red line #21).
// Each goroutine is launched via safego.Go to guarantee panic recovery (#13)
// and respects ctx cancellation (#23).
func (e *ParallelToolExecutor) executeLayer(ctx context.Context, calls []ToolCall) []ToolResult {
	if len(calls) == 0 {
		return nil
	}
	results := make([]ToolResult, len(calls))
	sem := make(chan struct{}, e.maxConcurrency)
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		idx, tc := i, call
		safego.Go(ctx, "parallel_tool.exec", func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				results[idx] = e.executeOne(ctx, tc)
			case <-ctx.Done():
				results[idx] = ToolResult{
					CallID:  tc.ID,
					Name:    tc.Name,
					Success: false,
					Error:   ctx.Err().Error(),
				}
			}
		})
	}
	wg.Wait()
	return results
}

// executeOne dispatches a single call to the appropriate executor based on its
// IsolationStrategy. Falls back to direct execution when no isolator matches.
func (e *ParallelToolExecutor) executeOne(ctx context.Context, call ToolCall) ToolResult {
	start := time.Now()
	var result ToolResult
	switch call.IsolationStrategy {
	case IsolationStrategyWorktree:
		if e.worktreeIso != nil {
			result = e.worktreeIso.Execute(ctx, call)
		} else {
			result = e.executeDirect(ctx, call)
		}
	case IsolationStrategyTransaction:
		if e.txSandbox != nil {
			result = e.txSandbox.Execute(ctx, call)
		} else {
			result = e.executeDirect(ctx, call)
		}
	default:
		result = e.executeDirect(ctx, call)
	}
	result.DurationMS = time.Since(start).Milliseconds()
	return result
}

// executeDirect runs the call via the registered handler. If no handler is
// configured, returns a failure result with a descriptive error.
func (e *ParallelToolExecutor) executeDirect(ctx context.Context, call ToolCall) ToolResult {
	if e.handler == nil {
		return ToolResult{
			CallID:  call.ID,
			Name:    call.Name,
			Success: false,
			Error:   "no tool handler configured",
		}
	}
	return e.handler(ctx, call)
}

// defaultConcurrency returns the default parallelism for tool execution.
// Uses GOMAXPROCS but clamps to a minimum of 1.
func defaultConcurrency() int {
	n := runtime.GOMAXPROCS(0)
	if n < 1 {
		return 1
	}
	return n
}
