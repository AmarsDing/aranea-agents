package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// DefaultToolTimeout is the default per-call timeout applied by ToolDecorator
// when ToolDecoratorConfig.Timeout is zero.
const DefaultToolTimeout = 60 * time.Second

// ResultBudget controls the maximum size of tool execution results.
// When a result's JSON serialization exceeds MaxBytes, it is truncated
// according to Mode and wrapped in a truncation envelope.
type ResultBudget struct {
	MaxBytes int
	Mode     string // "tail" (default, keep head), "head" (keep tail), "middle" (keep both ends)
}

// DefaultResultBudget is the default budget for tool results.
// 10KB is chosen as a practical upper bound for a single tool result in the
// LLM context window: large enough for typical file reads / search snippets,
// small enough to prevent a single tool from consuming excessive context.
//
// Read-only: treated as a shared default. Do not mutate MaxBytes/Mode after
// initialization; callers needing custom budgets should construct a new
// *ResultBudget rather than modifying this variable.
var DefaultResultBudget = &ResultBudget{MaxBytes: 10 * 1024, Mode: "tail"}

// truncationEnvelopeOverhead is the byte budget reserved for the JSON envelope
// wrapper fields ({"truncated":true,"original_size":N,"mode":"M","content":"..."}).
const truncationEnvelopeOverhead = 200

// ToolDecoratorConfig configures the ToolDecorator behavior.
type ToolDecoratorConfig struct {
	Timeout      time.Duration // 0 = use DefaultToolTimeout
	ResultBudget *ResultBudget // nil = no truncation
	EnableCache  bool          // cache ConcurrentSafe tools
	Logger       loggateway.Logger
}

// ToolDecorator wraps a CallableTool with three capabilities:
//   - P0-G3: per-call execution timeout
//   - P0-D: result size budget with truncation
//   - P2-E: deterministic cache for ConcurrentSafe tools
//
// ToolDecorator satisfies the CallableTool interface. It also implements
// StreamableCall as a pass-through so streaming tools retain their
// streaming capability (timeout/budget/cache apply only to Call).
type ToolDecorator struct {
	inner   trpctool.CallableTool
	cfg     ToolDecoratorConfig
	cache   map[string]any
	cacheMu sync.RWMutex
}

// NewToolDecorator wraps a CallableTool with the given config.
// When EnableCache is true, a cache map is allocated only for
// ConcurrentSafe tools (per IsCacheable) to avoid wasting memory
// on non-cacheable tools.
//
// Returns nil if inner is nil so callers can chain without extra guards;
// ApplyDecorators already skips nil entries, this is a defensive check
// for direct construction.
//
// The return type is trpctool.CallableTool (not *ToolDecorator) so that
// streaming-capable inner tools can be wrapped with the streamable variant
// (see streamableToolDecorator). Callers that need to detect streaming
// capability should use the StreamableTool interface assertion, which only
// succeeds when the inner tool is streamable.
func NewToolDecorator(inner trpctool.CallableTool, cfg ToolDecoratorConfig) trpctool.CallableTool {
	if inner == nil {
		return nil
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultToolTimeout
	}
	if cfg.Logger == nil {
		cfg.Logger = loggateway.NewNoop()
	}
	d := &ToolDecorator{inner: inner, cfg: cfg}
	if cfg.EnableCache {
		if name := d.toolName(); IsCacheable(name) {
			d.cache = make(map[string]any)
		}
	}
	// If the inner tool supports streaming, wrap with streamableToolDecorator
	// so the decorated tool also satisfies StreamableTool. Otherwise return
	// the plain *ToolDecorator, which does NOT satisfy StreamableTool —
	// preventing the framework from misclassifying non-streaming tools.
	if _, ok := inner.(trpctool.StreamableTool); ok {
		return &streamableToolDecorator{ToolDecorator: d}
	}
	return d
}

// streamableToolDecorator embeds *ToolDecorator and adds StreamableCall as
// a pass-through to the inner tool. This type exists (rather than defining
// StreamableCall on *ToolDecorator) so that only streaming-capable tools
// satisfy the StreamableTool interface after decoration.
//
// All Call/Declaration/cache/timeout/budget behavior is inherited from
// the embedded *ToolDecorator; only StreamableCall is added here.
type streamableToolDecorator struct {
	*ToolDecorator
}

// Compile-time interface assertions.
var (
	_ trpctool.CallableTool   = (*ToolDecorator)(nil)
	_ trpctool.StreamableTool = (*streamableToolDecorator)(nil)
)

// StreamableCall passes through to the inner tool's StreamableCall.
// Timeout/budget/cache do not apply to streaming calls; callers should
// rely on the context deadline for stream-level cancellation.
func (s *streamableToolDecorator) StreamableCall(ctx context.Context, jsonArgs []byte) (*trpctool.StreamReader, error) {
	if s.ToolDecorator == nil || s.ToolDecorator.inner == nil {
		return nil, fmt.Errorf("streamableToolDecorator: inner tool is nil")
	}
	st, ok := s.ToolDecorator.inner.(trpctool.StreamableTool)
	if !ok {
		// Should not happen: streamableToolDecorator is only constructed
		// when inner satisfies StreamableTool. Defensive guard.
		return nil, fmt.Errorf("tool %q is not streamable", s.ToolDecorator.toolName())
	}
	return st.StreamableCall(ctx, jsonArgs)
}

// Declaration returns the inner tool's declaration unchanged.
func (d *ToolDecorator) Declaration() *trpctool.Declaration {
	if d == nil || d.inner == nil {
		return nil
	}
	return d.inner.Declaration()
}

// Call invokes the inner tool with timeout, result budget, and caching
// applied. For ConcurrentSafe tools with caching enabled, repeated calls
// with identical arguments return the cached result without invoking
// the inner tool.
func (d *ToolDecorator) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if d.cache != nil {
		if cached, ok := d.lookupCache(jsonArgs); ok {
			return cached, nil
		}
	}
	callCtx, cancel := d.applyTimeout(ctx)
	defer cancel()
	result, err := d.inner.Call(callCtx, jsonArgs)
	if err != nil {
		return nil, err
	}
	result = d.truncateResult(result)
	if d.cache != nil {
		d.storeCache(jsonArgs, result)
	}
	return result, nil
}

// StreamableCall is intentionally NOT defined on *ToolDecorator so that
// `*ToolDecorator` does not satisfy trpctool.StreamableTool unless the
// inner tool does. Framework code uses `tool.(StreamableTool)` to detect
// streaming capability; if *ToolDecorator always implemented StreamableCall,
// every decorated tool would be misclassified as streamable.
//
// Streaming tools are instead wrapped with *streamableToolDecorator (see
// NewToolDecorator), which embeds *ToolDecorator and adds StreamableCall
// as a pass-through. Timeout/budget/cache apply only to Call, not to
// streaming calls; callers should rely on the context deadline for
// stream-level cancellation.

// applyTimeout returns a context with deadline if timeout > 0.
// If the existing context deadline is sooner than the timeout, the
// original context is returned unchanged. The returned CancelFunc MUST
// be deferred by the caller.
func (d *ToolDecorator) applyTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if d.cfg.Timeout <= 0 {
		return ctx, func() {}
	}
	if deadline, ok := ctx.Deadline(); ok {
		if time.Until(deadline) <= d.cfg.Timeout {
			return ctx, func() {}
		}
	}
	return context.WithTimeout(ctx, d.cfg.Timeout)
}

// truncateResult serializes and truncates the result if it exceeds the
// configured budget. Results within budget or when no budget is set are
// returned unchanged. The truncation envelope is a map[string]any with
// truncated/original_size/mode/content fields, which the framework
// serializes via its standard JSON path.
//
// The envelope is returned as map[string]any (not a pre-serialized string)
// so downstream consumers receive a structured object and the framework's
// own JSON marshaling handles escaping. This avoids double-encoding issues
// when the result is later embedded in agent messages.
func (d *ToolDecorator) truncateResult(result any) any {
	if d.cfg.ResultBudget == nil || d.cfg.ResultBudget.MaxBytes <= 0 {
		return result
	}
	data, err := json.Marshal(result)
	if err != nil || len(data) <= d.cfg.ResultBudget.MaxBytes {
		return result
	}
	budget := d.cfg.ResultBudget
	mode := budget.Mode
	if mode == "" {
		mode = "tail"
	}
	targetBytes := budget.MaxBytes - truncationEnvelopeOverhead
	if targetBytes <= 0 {
		targetBytes = budget.MaxBytes / 2
	}
	truncated := sliceForMode(data, targetBytes, mode)
	d.cfg.Logger.Info("tool result truncated",
		loggateway.StepID("tool.decorator.truncate"),
		loggateway.Int("original_size", len(data)),
		loggateway.Int("target_bytes", targetBytes),
		loggateway.Str("mode", mode),
	)
	return map[string]any{
		"truncated":     true,
		"original_size": len(data),
		"mode":          mode,
		"content":       string(truncated),
	}
}

// sliceForMode returns the truncated byte slice according to the mode:
//   - "tail" (default): keep the head (beginning), truncate the tail
//   - "head": keep the tail (end), truncate the head
//   - "middle": keep both head and tail halves, truncate the middle
func sliceForMode(data []byte, target int, mode string) []byte {
	if target >= len(data) {
		return data
	}
	switch mode {
	case "head":
		return data[len(data)-target:]
	case "middle":
		half := target / 2
		return append(append([]byte{}, data[:half]...), data[len(data)-half:]...)
	default: // "tail"
		return data[:target]
	}
}

func (d *ToolDecorator) toolName() string {
	if d == nil || d.inner == nil {
		return ""
	}
	if decl := d.inner.Declaration(); decl != nil {
		return decl.Name
	}
	return ""
}

func (d *ToolDecorator) cacheKey(jsonArgs []byte) string {
	name := d.toolName()
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte(":"))
	h.Write(jsonArgs)
	return hex.EncodeToString(h.Sum(nil))
}

func (d *ToolDecorator) lookupCache(jsonArgs []byte) (any, bool) {
	d.cacheMu.RLock()
	defer d.cacheMu.RUnlock()
	v, ok := d.cache[d.cacheKey(jsonArgs)]
	return v, ok
}

func (d *ToolDecorator) storeCache(jsonArgs []byte, result any) {
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	d.cache[d.cacheKey(jsonArgs)] = result
}
