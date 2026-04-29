package middleware

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"arenea/backend/internal/capability/telemetry"
	"arenea/backend/internal/capability/toolctx"
	"arenea/backend/internal/capability/tooldef"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	DefaultCallBudget       = 8
	DefaultFailureBudgetArg = 2
	maxEventStringRunes     = 1024
)

type Next func(*toolctx.ToolContext, tooldef.Tool, map[string]any) (map[string]any, error)

type Middleware interface {
	Run(*toolctx.ToolContext, tooldef.Tool, map[string]any, Next) (map[string]any, error)
}

type MiddlewareFunc func(*toolctx.ToolContext, tooldef.Tool, map[string]any, Next) (map[string]any, error)

func (m MiddlewareFunc) Run(ctx *toolctx.ToolContext, t tooldef.Tool, params map[string]any, next Next) (map[string]any, error) {
	return m(ctx, t, params, next)
}

func BuildChain(mws ...Middleware) Middleware {
	return MiddlewareFunc(func(ctx *toolctx.ToolContext, t tooldef.Tool, params map[string]any, final Next) (map[string]any, error) {
		next := final
		for i := len(mws) - 1; i >= 0; i-- {
			mw := mws[i]
			current := next
			next = func(ctx *toolctx.ToolContext, t tooldef.Tool, params map[string]any) (map[string]any, error) {
				return mw.Run(ctx, t, params, current)
			}
		}
		return next(ctx, t, params)
	})
}

func FinalExecutor() Next {
	return func(ctx *toolctx.ToolContext, t tooldef.Tool, params map[string]any) (map[string]any, error) {
		return t.Execute(ctx, params)
	}
}

func Validation() Middleware {
	return MiddlewareFunc(func(ctx *toolctx.ToolContext, t tooldef.Tool, params map[string]any, next Next) (map[string]any, error) {
		if err := t.Validate(params); err != nil {
			return nil, fmt.Errorf("tool %s validation failed: %w", t.Name(), err)
		}
		return next(ctx, t, params)
	})
}

func Tracing(provider *telemetry.Provider) Middleware {
	if provider == nil {
		provider = telemetry.DefaultProvider()
	}
	tracer := provider.Tracer()
	return MiddlewareFunc(func(ctx *toolctx.ToolContext, t tooldef.Tool, params map[string]any, next Next) (map[string]any, error) {
		var base context.Context
		if ctx != nil {
			base = ctx.Context
		}
		if base == nil {
			base = context.Background()
		}
		spanCtx, span := tracer.Start(base, "tool."+t.Name())
		span.SetAttributes(
			attribute.String("tool.name", t.Name()),
			attribute.String("tool.category", t.Category()),
		)
		if ctx != nil {
			span.SetAttributes(
				attribute.String("session.id", ctx.SessionID),
				attribute.String("message.id", ctx.MessageID),
				attribute.String("user.id", ctx.UserID),
				attribute.String("agent.id", ctx.AgentID),
				attribute.String("agent.key", ctx.AgentKey),
				attribute.String("function_call.id", ctx.FunctionCallID),
			)
			ctx = ctx.Clone(spanCtx)
		}
		out, err := next(ctx, t, params)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
		return out, err
	})
}

type BudgetState struct {
	mu         sync.Mutex
	totalCalls int
	failures   map[string]int
	started    map[string]time.Time
}

func NewBudgetState() *BudgetState {
	return &BudgetState{failures: map[string]int{}, started: map[string]time.Time{}}
}

func Budget(state *BudgetState, callBudget int, failureBudget int, emit tooldef.EventSink) Middleware {
	if state == nil {
		state = NewBudgetState()
	}
	if callBudget <= 0 {
		callBudget = DefaultCallBudget
	}
	if failureBudget <= 0 {
		failureBudget = DefaultFailureBudgetArg
	}
	return MiddlewareFunc(func(ctx *toolctx.ToolContext, t tooldef.Tool, params map[string]any, next Next) (map[string]any, error) {
		id := eventID(ctx, t.Name())
		fingerprint := t.Name() + "|" + fingerprintArgs(params)
		state.mu.Lock()
		if state.totalCalls >= callBudget {
			state.mu.Unlock()
			result := map[string]any{
				"status":  "blocked",
				"reason":  fmt.Sprintf("tool call budget for this turn exceeded (max %d calls)", callBudget),
				"tool":    t.Name(),
				"blocked": true,
			}
			_ = emitEvent(emit, id, "before", "blocked", t, params, nil, nil, 0)
			_ = emitEvent(emit, id, "after", "blocked", t, params, result, nil, 0)
			return result, nil
		}
		if count := state.failures[fingerprint]; count >= failureBudget {
			state.mu.Unlock()
			result := map[string]any{
				"status":  "blocked",
				"reason":  fmt.Sprintf("tool %q already failed %d times with these arguments", t.Name(), count),
				"tool":    t.Name(),
				"blocked": true,
			}
			_ = emitEvent(emit, id, "before", "blocked", t, params, nil, nil, 0)
			_ = emitEvent(emit, id, "after", "blocked", t, params, result, nil, 0)
			return result, nil
		}
		state.totalCalls++
		state.started[id] = time.Now()
		state.mu.Unlock()

		_ = emitEvent(emit, id, "before", "running", t, params, nil, nil, 0)
		result, err := next(ctx, t, params)

		state.mu.Lock()
		started := state.started[id]
		delete(state.started, id)
		if err != nil {
			state.failures[fingerprint]++
		}
		state.mu.Unlock()

		duration := 0
		if !started.IsZero() {
			duration = int(time.Since(started).Milliseconds())
		}
		status := "success"
		if err != nil {
			status = "error"
		}
		_ = emitEvent(emit, id, "after", status, t, params, result, err, duration)
		return result, err
	})
}

func emitEvent(emit tooldef.EventSink, id string, phase string, status string, t tooldef.Tool, args map[string]any, result map[string]any, err error, durationMS int) error {
	if emit == nil {
		return nil
	}
	event := tooldef.Event{
		ID:         id,
		Phase:      phase,
		Status:     status,
		ToolName:   t.Name(),
		ToolLabel:  t.DisplayName(),
		Arguments:  sanitizeArgs(args),
		Result:     summarizeResult(result),
		OccurredAt: time.Now().UTC(),
		DurationMS: durationMS,
	}
	if err != nil {
		event.Error = err.Error()
	}
	switch {
	case phase == "before":
		event.MessageHint = fmt.Sprintf("???? %s", event.ToolLabel)
	case status == "success":
		event.MessageHint = fmt.Sprintf("??? %s", event.ToolLabel)
	default:
		event.MessageHint = fmt.Sprintf("?? %s ??", event.ToolLabel)
	}
	return emit(event)
}

func eventID(ctx *toolctx.ToolContext, fallback string) string {
	if ctx != nil && strings.TrimSpace(ctx.FunctionCallID) != "" {
		return strings.TrimSpace(ctx.FunctionCallID)
	}
	return strings.TrimSpace(fallback)
}

func fingerprintArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := fmt.Sprintf("%v", args[key])
		if len(value) > 96 {
			value = value[:96]
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, "&")
}

func sanitizeArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	out := map[string]any{}
	for key, value := range args {
		out[key] = sanitizeValue(key, value)
	}
	return out
}

func sanitizeValue(key string, value any) any {
	if isSensitiveKey(key) {
		return "***"
	}
	switch v := value.(type) {
	case string:
		return truncateString(v, maxEventStringRunes)
	case fmt.Stringer:
		return truncateString(v.String(), maxEventStringRunes)
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	for _, marker := range []string{"password", "token", "secret", "api_key", "authorization", "cookie"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func truncateString(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}

func summarizeResult(result map[string]any) map[string]any {
	if len(result) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{"path", "written", "replacements", "size", "url", "status_code", "local", "utc"} {
		if value, ok := result[key]; ok {
			out[key] = value
		}
	}
	if items, ok := result["items"].([]map[string]any); ok {
		out["items_count"] = len(items)
	} else if items, ok := result["items"].([]any); ok {
		out["items_count"] = len(items)
	}
	return out
}
