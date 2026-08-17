package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
	"aranea-agents/pkg/strutil"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// DefaultToolTimeout is the named 60s decorator timeout for callers that
// explicitly want an inner deadline. Timeout=0 does not use this value;
// it defers to the caller/callback-chain deadline (ToolsExecutionTimeoutSec).
const DefaultToolTimeout = 60 * time.Second

// 2026-08-06 P0-3 工具预算治理（20:45 会话 plan_and_execute budget exhaustion
// 根因）：编排入口工具的子阶段 LLM 预算与外层工具预算。
//
//   - DecomposeLLMTimeout：task_planner 分解（decomposeTask / Stream）单次 LLM
//     调用子超时。
//   - AllocateLLMTimeout：agent_allocator 分配单次 LLM 调用子超时。
//   - PlanAndExecuteTimeout：plan_and_execute 工具外层预算。不变量：外层预算
//     必须 ≥ 分解 + 分配 + 编排委派余量，否则外层装饰器会在子阶段中途掐断
//     （此前外层套用 DefaultToolTimeout=60s < 60s+60s 子阶段之和，必现超时）。
//
// 子超时常量子集中在 tools 包，调用点（internal/agent）引用同一常量，
// 防止预算各自漂移破坏不变量。
const (
	DecomposeLLMTimeout   = 60 * time.Second
	AllocateLLMTimeout    = 60 * time.Second
	PlanAndExecuteTimeout = 3 * time.Minute
)

// builtinTimeoutOverrides maps tool-name suffixes to per-tool execution
// timeouts. Suffix-based matching (consistent with budgetOverrideForTool)
// so ToolSet prefixes are handled transparently. These overrides still
// apply when ToolDecoratorConfig.Timeout is 0.
var builtinTimeoutOverrides = map[string]time.Duration{
	"plan_and_execute": PlanAndExecuteTimeout,
}

// timeoutOverrideForTool returns the per-tool timeout override for a tool
// based on its declaration name, or 0 when no override applies.
func timeoutOverrideForTool(name string) time.Duration {
	if name == "" {
		return 0
	}
	for suffix, timeout := range builtinTimeoutOverrides {
		if name == suffix || strings.HasSuffix(name, "_"+suffix) {
			return timeout
		}
	}
	return 0
}

// DefaultStreamTimeout is the default maximum duration for a streaming tool
// call when ToolDecoratorConfig.StreamTimeout is zero. Streaming tools that
// run longer than this are terminated with a context-deadline error.
const DefaultStreamTimeout = 5 * time.Minute

// DefaultStreamBudget is the default maximum total byte size of stream chunks
// when ToolDecoratorConfig.StreamBudget is zero. Streams exceeding this total
// are terminated with a budget-exceeded error. Set StreamBudget to a negative
// value to disable the budget (unlimited).
const DefaultStreamBudget = 1024 * 1024 // 1MB

// streamProxyBufferSize is the channel buffer size for the proxy stream that
// wraps the inner StreamReader. A small buffer is sufficient because the
// proxy goroutine drains the inner reader as fast as the consumer reads.
const streamProxyBufferSize = 16

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

// builtinResultBudgetOverrides maps tool-name suffixes to custom result
// budgets. Matching is suffix-based (consistent with browser.requiresURLValidation)
// so MCP ToolPrefix prefixes (e.g. "bw_browser_take_screenshot") are handled
// transparently.
//
// Browser tools return larger payloads than typical tools:
//   - browser_take_screenshot: base64-encoded image, 10KB truncation corrupts
//     the image data and makes it undecodable. 100KB allows ~75KB of actual
//     image data after base64 overhead.
//   - browser_snapshot: accessibility tree text, complex pages easily exceed
//     10KB. 50KB preserves most of the page structure for LLM analysis.
var builtinResultBudgetOverrides = map[string]*ResultBudget{
	"browser_take_screenshot": {MaxBytes: 100 * 1024, Mode: "tail"},
	"browser_screenshot":      {MaxBytes: 100 * 1024, Mode: "tail"},
	"browser_snapshot":        {MaxBytes: 50 * 1024, Mode: "tail"},
	// read_upstream_deliverable returns full deliverable text on demand (P2).
	// The tool self-caps at 200000 runes (~600KB worst-case CJK bytes); the
	// override must exceed that so the decorator never corrupts the tool's
	// own truncation contract. Defensive only — it should never fire.
	"read_upstream_deliverable": {MaxBytes: 620 * 1024, Mode: "tail"},
}

// budgetOverrideForTool returns the custom result budget for a tool based on
// its declaration name, or nil if no override applies. Suffix-based matching
// handles MCP ToolPrefix prefixes (e.g. "bw_browser_snapshot" matches
// "browser_snapshot").
func budgetOverrideForTool(name string) *ResultBudget {
	if name == "" {
		return nil
	}
	for suffix, budget := range builtinResultBudgetOverrides {
		if name == suffix || strings.HasSuffix(name, "_"+suffix) {
			return budget
		}
	}
	return nil
}

// truncationEnvelopeOverhead is the byte budget reserved for the JSON envelope
// wrapper fields ({"truncated":true,"original_size":N,"mode":"M","content":"..."}).
const truncationEnvelopeOverhead = 200

// ToolDecoratorConfig configures the ToolDecorator behavior.
type ToolDecoratorConfig struct {
	Timeout      time.Duration // 0 = no decorator timeout; honor caller deadline
	ResultBudget *ResultBudget // nil = no truncation
	EnableCache  bool          // cache ConcurrentSafe tools that IsCacheable (not workspace files)
	// CacheTTL is how long a cached network result stays valid.
	// 0 = DefaultCacheTTL (60s). Negative disables expiry.
	CacheTTL time.Duration
	Logger   loggateway.Logger
	// StreamTimeout is the maximum duration for a streaming tool call.
	// 0 = use DefaultStreamTimeout. A negative value disables the timeout
	// (not recommended for production).
	StreamTimeout time.Duration
	// StreamBudget is the maximum total byte size of stream chunks.
	// 0 = use DefaultStreamBudget. A negative value disables the budget
	// (unlimited, not recommended for production).
	StreamBudget int
}

// ToolDecorator wraps a CallableTool with three capabilities:
//   - P0-G3: per-call execution timeout
//   - P0-D: result size budget with truncation
//   - P2-E: deterministic cache for ConcurrentSafe tools
//
// ToolDecorator satisfies the CallableTool interface. It also implements
// StreamableCall as a pass-through so streaming tools retain their
// streaming capability (timeout/budget/cache apply only to Call).
//
// The cache is bounded by decoratorCacheMaxEntries. When the limit is
// reached, the entire cache is cleared (crude but prevents unbounded
// memory growth from varying arguments on cacheable read-only tools).
type ToolDecorator struct {
	inner   trpctool.CallableTool
	cfg     ToolDecoratorConfig
	cache   map[string]cacheEntry
	cacheMu sync.RWMutex
}

type cacheEntry struct {
	value any
	at    time.Time
}

// decoratorCacheMaxEntries is the per-tool cache limit. 256 is generous for
// typical cacheable network tools while preventing unbounded growth.
const decoratorCacheMaxEntries = 256

// DefaultCacheTTL is how long a cached ConcurrentSafe network result is
// reused. Long enough to collapse identical calls in one turn; short
// enough that a later turn does not treat a stale page as current.
const DefaultCacheTTL = 60 * time.Second

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
	if cfg.Logger == nil {
		cfg.Logger = loggateway.NewNoop()
	}
	d := &ToolDecorator{inner: inner, cfg: cfg}
	// 2026-08-06 P0-3：per-tool 超时覆盖（如 plan_and_execute 的多子阶段
	// 预算）。覆盖在显式 cfg.Timeout 之上——按工具语义修正预算。
	// Timeout=0 不再回落到 DefaultToolTimeout，以便 Agent
	// ToolsExecutionTimeoutSec（回调链）成为普通工具的单一截止来源。
	if override := timeoutOverrideForTool(d.toolName()); override > 0 {
		d.cfg.Timeout = override
	}
	if cfg.EnableCache {
		if name := d.toolName(); IsCacheable(name) {
			d.cache = make(map[string]cacheEntry)
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

// streamableToolDecorator embeds *ToolDecorator and adds StreamableCall
// with P2-02 streaming guards (deadline + byte budget + cancellation).
// This type exists (rather than defining StreamableCall on *ToolDecorator)
// so that only streaming-capable tools satisfy the StreamableTool interface
// after decoration.
//
// All Call/Declaration/cache/timeout/budget behavior is inherited from
// the embedded *ToolDecorator; only StreamableCall is added here, where it
// wraps the inner StreamReader with a proxy goroutine that enforces:
//   - StreamTimeout: maximum stream duration (default 5 min)
//   - StreamBudget: maximum total chunk byte size (default 1 MB)
//   - Context cancellation: propagates caller cancellation to the inner tool
type streamableToolDecorator struct {
	*ToolDecorator
}

// Compile-time interface assertions.
var (
	_ trpctool.CallableTool   = (*ToolDecorator)(nil)
	_ trpctool.StreamableTool = (*streamableToolDecorator)(nil)
)

// StreamableCall wraps the inner tool's StreamableCall with a proxy
// goroutine that enforces stream-level deadline, byte budget, and
// context cancellation (P2-02). The returned StreamReader is a proxy
// reader; the proxy goroutine drains the inner reader and forwards
// chunks to the proxy writer.
//
// When the budget or deadline is exceeded, the proxy sends a final error
// chunk to the consumer and terminates the stream. The inner tool is
// notified via context cancellation so it can release its resources.
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

	jsonArgs = NormalizeInvocationWithLog(s.cfg.Logger, s.toolName(), jsonArgs)
	unlock := lockExclusiveTool(s.toolName(), jsonArgs)

	cfg := s.cfg
	timeout := cfg.StreamTimeout
	if timeout == 0 {
		timeout = DefaultStreamTimeout
	}
	budget := cfg.StreamBudget
	if budget == 0 {
		budget = DefaultStreamBudget
	}

	// Apply a deadline to the stream context. If the caller's context
	// already has a sooner deadline, keep it.
	streamCtx, cancel := context.WithCancel(ctx)
	if timeout > 0 {
		if dl, ok := streamCtx.Deadline(); !ok || time.Until(dl) > timeout {
			var timeoutCancel context.CancelFunc
			streamCtx, timeoutCancel = context.WithTimeout(streamCtx, timeout)
			origCancel := cancel
			cancel = func() { origCancel(); timeoutCancel() }
		}
	}

	innerReader, err := st.StreamableCall(streamCtx, jsonArgs)
	if err != nil {
		cancel()
		unlock()
		return nil, err
	}
	if innerReader == nil {
		cancel()
		unlock()
		return nil, fmt.Errorf("streamable tool %q returned nil reader", s.toolName())
	}

	proxy := trpctool.NewStream(streamProxyBufferSize)
	reader := proxy.Reader
	writer := proxy.Writer
	toolName := s.toolName()
	lg := cfg.Logger
	if lg == nil {
		lg = loggateway.NewNoop()
	}

	safego.Go(streamCtx, "tools.stream_proxy", func() {
		defer unlock()
		defer cancel()
		defer innerReader.Close()
		defer writer.Close()
		s.proxyStreamLoop(streamCtx, innerReader, writer, budget, toolName, lg)
	})

	return reader, nil
}

// proxyStreamLoop drains the inner StreamReader and forwards chunks to the
// proxy StreamWriter, enforcing byte budget and detecting context
// cancellation. It exits when:
//   - The inner reader returns io.EOF (clean stream end)
//   - The inner reader returns an error (propagated to consumer)
//   - The byte budget is exceeded (sends budget-exceeded error)
//   - The stream context is cancelled (sends cancellation error)
//   - The consumer closes the proxy reader (writer.Send returns true)
func (s *streamableToolDecorator) proxyStreamLoop(
	streamCtx context.Context,
	innerReader *trpctool.StreamReader,
	writer *trpctool.StreamWriter,
	budget int,
	toolName string,
	lg loggateway.Logger,
) {
	var totalBytes int
	for {
		// Check context before blocking on Recv to detect deadline/cancel
		// promptly when the inner tool respects context cancellation.
		if streamCtx.Err() != nil {
			writer.Send(trpctool.StreamChunk{}, fmt.Errorf("stream %q cancelled: %w", toolName, streamCtx.Err()))
			return
		}
		chunk, err := innerReader.Recv()
		if err != nil {
			// Check context first: if the deadline/cancellation fired, the
			// inner tool may have closed the writer (causing io.EOF) in
			// response to context cancellation. In that case, report the
			// cancellation error rather than treating it as a clean EOF.
			if cerr := streamCtx.Err(); cerr != nil {
				writer.Send(trpctool.StreamChunk{}, fmt.Errorf("stream %q cancelled: %w", toolName, cerr))
				return
			}
			if err == io.EOF {
				return // Clean stream end
			}
			// Inner tool error: propagate to consumer.
			writer.Send(trpctool.StreamChunk{}, err)
			return
		}
		if budget > 0 {
			chunkBytes := estimateChunkBytes(chunk)
			totalBytes += chunkBytes
			if totalBytes > budget {
				lg.Warn("stream budget exceeded, terminating",
					loggateway.StepID("tool.decorator.stream"),
					loggateway.Str("tool", toolName),
					loggateway.Int("bytes", totalBytes),
					loggateway.Int("budget", budget))
				writer.Send(trpctool.StreamChunk{}, fmt.Errorf("stream %q budget exceeded: %d > %d bytes", toolName, totalBytes, budget))
				return
			}
		}
		if closed := writer.Send(chunk, nil); closed {
			return // Consumer closed the proxy reader
		}
	}
}

// estimateChunkBytes returns the approximate byte size of a StreamChunk's
// content. It uses JSON marshaling for complex types and falls back to
// string length. A nil content contributes zero bytes.
func estimateChunkBytes(chunk trpctool.StreamChunk) int {
	if chunk.Content == nil {
		return 0
	}
	if data, err := json.Marshal(chunk.Content); err == nil {
		return len(data)
	}
	if s, ok := chunk.Content.(string); ok {
		return len(s)
	}
	return 0
}

// Declaration returns the inner tool's declaration unchanged.
func (d *ToolDecorator) Declaration() *trpctool.Declaration {
	if d == nil || d.inner == nil {
		return nil
	}
	return d.inner.Declaration()
}

// Original exposes the wrapped tool via the framework's Original()
// unwrapping convention (NamedTool, toolpipe, and the function-call
// processor's StateDelta discovery all unwrap through it). Without this,
// StateDelta-providing tools (set_deliverable, todo_write, save_artifact)
// decorated by ApplyDecorators silently lose their session-state mutations:
// the framework type-asserts the unwrapped tool to the state-delta
// interfaces, and the decorator itself does not forward them.
func (d *ToolDecorator) Original() trpctool.Tool {
	if d == nil {
		return nil
	}
	return d.inner
}

// stateDeltaProvider mirrors the framework's duck-typing convention in
// flow.processor.attachStateDelta. Tools implementing this exact signature
// turn their result into session/graph state.
type stateDeltaProvider interface {
	StateDelta(toolCallID string, args []byte, result []byte) map[string][]byte
}

// providesStateDelta reports whether the wrapped tool exposes StateDelta,
// unwrapping nested decorators via the Original() convention. Truncating
// such a tool's result corrupts the JSON the framework feeds back into
// StateDelta, so truncateResult skips the budget for them.
func (d *ToolDecorator) providesStateDelta() bool {
	for tl := trpctool.Tool(d.inner); tl != nil; {
		if _, ok := tl.(stateDeltaProvider); ok {
			return true
		}
		originator, ok := tl.(interface{ Original() trpctool.Tool })
		if !ok {
			return false
		}
		inner := originator.Original()
		if inner == nil || inner == tl {
			return false
		}
		tl = inner
	}
	return false
}

// Call invokes the inner tool with timeout, result budget, and caching
// applied. For ConcurrentSafe tools with caching enabled, repeated calls
// with identical arguments return the cached result without invoking
// the inner tool.
func (d *ToolDecorator) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	jsonArgs = NormalizeInvocationWithLog(d.cfg.Logger, d.toolName(), jsonArgs)
	unlock := lockExclusiveTool(d.toolName(), jsonArgs)
	defer unlock()
	// E2E-P1-09: only cache when invocation identity is present so results
	// never leak across user/session/agent scopes on a shared Agent cache.
	scope, scoped := cacheScopeFromCtx(ctx)
	if d.cache != nil && scoped {
		if cached, ok := d.lookupCache(scope, jsonArgs); ok {
			return cached, nil
		}
	}
	callCtx, cancel := d.applyTimeout(ctx)
	defer cancel()
	result, err := d.inner.Call(callCtx, jsonArgs)
	if err != nil {
		return nil, err
	}
	result = d.truncateResult(ctx, jsonArgs, result)
	result = FlagTransientResult(d.toolName(), result)
	if _, flagged := result.(retryFlaggedResult); flagged {
		// Do not cache transient failures; the retry runner must re-invoke.
		return result, nil
	}
	if d.cache != nil && scoped {
		d.storeCache(scope, jsonArgs, result)
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
// with P2-02 streaming guards (deadline + byte budget + cancellation).
// See streamableToolDecorator.StreamableCall for details.

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

// truncateResult serializes the result and enforces the configured budget.
// Results within budget or when no budget is set are returned unchanged.
//
// Oversized results are OFFLOADED rather than truncated whenever the
// invocation can persist artifacts (see offloadResult): the full JSON is
// saved via the artifact service and the LLM receives an offload envelope
// with a resolvable artifact:// ref plus dual-end previews, so no data is
// ever lost. When offloading is unavailable (no invocation/session/service,
// excluded tool, or save failure) the legacy truncation envelope is
// produced, preserving the original behavior byte-for-byte.
//
// The envelope is returned as map[string]any (not a pre-serialized string)
// so downstream consumers receive a structured object and the framework's
// own JSON marshaling handles escaping. This avoids double-encoding issues
// when the result is later embedded in agent messages.
//
// StateDelta-providing tools bypass the budget entirely: the framework
// feeds the decorated result back into StateDelta (flow.processor
// attachStateDelta), and a truncation envelope fails to parse there —
// the session-state write is silently dropped (bff43a17 incident:
// set_deliverable's 28.9KB echo was enveloped, deliverable lost).
// Truncation also corrupts the JSON the LLM sees for such tools, so
// skipping it is strictly safer than enveloping.
func (d *ToolDecorator) truncateResult(ctx context.Context, jsonArgs []byte, result any) any {
	if d.providesStateDelta() {
		return result
	}
	budget := d.cfg.ResultBudget
	if override := budgetOverrideForTool(d.toolName()); override != nil {
		budget = override
	}
	if budget == nil || budget.MaxBytes <= 0 {
		return result
	}
	data, err := json.Marshal(result)
	if err != nil || len(data) <= budget.MaxBytes {
		return result
	}
	if envelope, ok := d.offloadResult(ctx, jsonArgs, data); ok {
		return envelope
	}
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

// offloadPreviewHeadBytes / offloadPreviewTailBytes bound the dual-end
// preview embedded in an offload envelope. The head carries the beginning
// of the payload; the tail often carries conclusions / error summaries.
const (
	offloadPreviewHeadBytes = 2048
	offloadPreviewTailBytes = 512
)

// offloadExcludedTools lists tool names that must never be offloaded.
// read_file is the read-back path for offloaded refs — offloading its
// result would force the LLM to call read_file to fetch a read_file
// result, a circular regress. Suffix matching mirrors
// budgetOverrideForTool so MCP ToolPrefix prefixes are handled.
var offloadExcludedTools = []string{"read_file"}

// offloadResult persists the full oversized result JSON via the invocation's
// artifact service and returns an offload envelope:
//
//	{offloaded, ref, tool, original_size, preview_head, preview_tail, read_hint}
//
// The artifact filename is deterministic (tool + sha256(args)[:16]) so the
// same call rewrites the same artifact instead of bloating storage. The
// second return value is false when offloading is unavailable, in which
// case the caller falls back to the legacy truncation envelope.
func (d *ToolDecorator) offloadResult(ctx context.Context, jsonArgs, data []byte) (any, bool) {
	name := d.toolName()
	for _, suffix := range offloadExcludedTools {
		if name == suffix || strings.HasSuffix(name, "_"+suffix) {
			return nil, false
		}
	}
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.ArtifactService == nil || inv.Session == nil ||
		inv.Session.AppName == "" || inv.Session.UserID == "" || inv.Session.ID == "" {
		return nil, false
	}
	ctxIO := codeexecutor.WithArtifactService(ctx, inv.ArtifactService)
	ctxIO = codeexecutor.WithArtifactSession(ctxIO, trpcartifact.SessionInfo{
		AppName:   inv.Session.AppName,
		UserID:    inv.Session.UserID,
		SessionID: inv.Session.ID,
	})
	sum := sha256.Sum256(jsonArgs)
	filename := "tool_results/" + sanitizeOffloadName(name) + "/" +
		hex.EncodeToString(sum[:])[:16] + ".json"
	ver, err := codeexecutor.SaveArtifactHelper(ctxIO, filename, data, "application/json")
	if err != nil {
		d.cfg.Logger.Warn("tool result offload failed, falling back to truncation",
			loggateway.StepID("tool.decorator.offload"),
			loggateway.Str("tool", name),
			loggateway.Int("original_size", len(data)),
			loggateway.Err(err),
		)
		return nil, false
	}
	ref := "artifact://" + filename + "@" + strconv.Itoa(ver)
	head := data
	if len(head) > offloadPreviewHeadBytes {
		head = head[:offloadPreviewHeadBytes]
	}
	tail := data
	if len(tail) > offloadPreviewTailBytes {
		tail = tail[len(tail)-offloadPreviewTailBytes:]
	}
	d.cfg.Logger.Info("tool result offloaded",
		loggateway.StepID("tool.decorator.offload"),
		loggateway.Str("tool", name),
		loggateway.Int("original_size", len(data)),
		loggateway.Str("ref", ref),
	)
	return map[string]any{
		"offloaded":     true,
		"ref":           ref,
		"tool":          name,
		"original_size": len(data),
		"preview_head":  string(head),
		"preview_tail":  string(tail),
		"read_hint": "Result too large for context. Full JSON saved to ref. " +
			"Use read_file with file_name=ref and start_line/num_lines to page through it.",
	}, true
}

// sanitizeOffloadName makes a tool name safe for use as an artifact
// directory component.
func sanitizeOffloadName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "tool"
	}
	return b.String()
}

// sliceForMode returns the truncated byte slice according to the mode:
//   - "tail" (default): keep the head (beginning), truncate the tail
//   - "head": keep the tail (end), truncate the head
//   - "middle": keep both head and tail halves, truncate the middle
func sliceForMode(data []byte, target int, mode string) []byte {
	// P1-3：切点对齐 rune 边界，避免 CJK 多字节字符被切成非法 UTF-8
	// （序列化成 JSON envelope 后产生 U+FFFD 污染模型输入）。
	return strutil.SliceBytesRuneSafe(data, target, mode)
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

// cacheScopeFromCtx returns an identity scope for tool-result caching.
// Without an Invocation Session, caching is disabled (ok=false) so unscoped
// calls cannot poison or read a shared cross-tenant bucket.
// C-03: scope includes workspace ID so results never leak across tenants
// that share the same session/user identifiers.
func cacheScopeFromCtx(ctx context.Context) (scope string, ok bool) {
	inv, has := trpcagent.InvocationFromContext(ctx)
	if !has || inv == nil || inv.Session == nil {
		return "", false
	}
	s := inv.Session
	if strings.TrimSpace(s.ID) == "" && strings.TrimSpace(s.UserID) == "" {
		return "", false
	}
	wsID := workspace.IDFromContext(ctx)
	return wsID + "\x00" + s.AppName + "\x00" + s.UserID + "\x00" + s.ID, true
}

func (d *ToolDecorator) cacheKey(scope string, jsonArgs []byte) string {
	name := d.toolName()
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte(":"))
	h.Write([]byte(scope))
	h.Write([]byte(":"))
	h.Write(jsonArgs)
	return hex.EncodeToString(h.Sum(nil))
}

func (d *ToolDecorator) lookupCache(scope string, jsonArgs []byte) (any, bool) {
	key := d.cacheKey(scope, jsonArgs)
	ttl := d.cacheTTL()
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	e, ok := d.cache[key]
	if !ok {
		return nil, false
	}
	if ttl > 0 && time.Since(e.at) > ttl {
		delete(d.cache, key)
		return nil, false
	}
	return e.value, true
}

func (d *ToolDecorator) storeCache(scope string, jsonArgs []byte, result any) {
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	if len(d.cache) >= decoratorCacheMaxEntries {
		// Bounded eviction: clear the cache when the limit is reached.
		// This is crude (no LRU) but prevents unbounded memory growth.
		d.cache = make(map[string]cacheEntry)
	}
	d.cache[d.cacheKey(scope, jsonArgs)] = cacheEntry{value: result, at: time.Now()}
}

func (d *ToolDecorator) cacheTTL() time.Duration {
	if d.cfg.CacheTTL < 0 {
		return 0
	}
	if d.cfg.CacheTTL == 0 {
		return DefaultCacheTTL
	}
	return d.cfg.CacheTTL
}
