package monitor

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/pkg/loggateway"
)

// DefaultHealActionHandler provides a basic implementation of HealActionHandler.
// It logs fix actions and performs simple retry/reconnect strategies.
// For production use, replace with a handler that integrates with the actual runtime.
type DefaultHealActionHandler struct {
	lg loggateway.Logger
}

// NewDefaultHealActionHandler creates a new default heal action handler.
func NewDefaultHealActionHandler(lg loggateway.Logger) *DefaultHealActionHandler {
	return &DefaultHealActionHandler{lg: lg}
}

func (h *DefaultHealActionHandler) HandleFixAction(ctx context.Context, action FixAction, metadata map[string]any) error {
	if h == nil {
		return fmt.Errorf("DefaultHealActionHandler is nil")
	}

	h.lg.Info("HealActionHandler: executing fix action",
		loggateway.StepID("monitor.heal_action"),
		loggateway.Str("type", action.Type),
		loggateway.Int("max_attempts", action.MaxAttempts),
	)

	switch action.Type {
	case "retry":
		return h.handleRetry(ctx, action, metadata)
	case "reconnect":
		return h.handleReconnect(ctx, action, metadata)
	case "fallback":
		return h.handleFallback(ctx, action, metadata)
	case "log_only":
		h.lg.Info("HealActionHandler: log_only action, no auto-fix applied",
			loggateway.StepID("monitor.heal_log_only"),
			loggateway.Str("metadata", fmt.Sprintf("%v", metadata)))
		return nil
	default:
		h.lg.Warn("HealActionHandler: unknown action type",
			loggateway.StepID("monitor.heal_unknown_action"),
			loggateway.Str("type", action.Type))
		return fmt.Errorf("unknown fix action type: %s", action.Type)
	}
}

func (h *DefaultHealActionHandler) handleRetry(_ context.Context, action FixAction, metadata map[string]any) error {
	backoffMs := intParam(action.Params, "backoff_ms", 1000)
	factor := floatParam(action.Params, "backoff_factor", 2.0)

	h.lg.Info("HealActionHandler: retry strategy",
		loggateway.StepID("monitor.heal_retry"),
		loggateway.Int("max_attempts", action.MaxAttempts),
		loggateway.Int("backoff_ms", backoffMs),
		loggateway.Float64("backoff_factor", factor))

	// In the default handler, we just log the retry strategy.
	// The actual retry is handled by the Agent runtime (trpc-agent-go)
	// which has built-in retry logic for LLM calls.
	// This handler signals that a retry should be attempted.
	_ = time.Duration(backoffMs) * time.Millisecond
	return nil
}

func (h *DefaultHealActionHandler) handleReconnect(_ context.Context, action FixAction, metadata map[string]any) error {
	backoffMs := intParam(action.Params, "backoff_ms", 3000)
	factor := floatParam(action.Params, "backoff_factor", 2.0)

	h.lg.Info("HealActionHandler: reconnect strategy",
		loggateway.StepID("monitor.heal_reconnect"),
		loggateway.Int("max_attempts", action.MaxAttempts),
		loggateway.Int("backoff_ms", backoffMs),
		loggateway.Float64("backoff_factor", factor))

	// The actual MCP reconnection is handled by the MCP broker health check.
	// This handler signals that a reconnection should be attempted.
	return nil
}

func (h *DefaultHealActionHandler) handleFallback(_ context.Context, action FixAction, metadata map[string]any) error {
	strategy := strParam(action.Params, "strategy", "unknown")

	h.lg.Info("HealActionHandler: fallback strategy",
		loggateway.StepID("monitor.heal_fallback"),
		loggateway.Str("strategy", strategy))

	// Fallback strategies are handled by the Agent runtime.
	return nil
}

func intParam(params map[string]any, key string, defaultVal int) int {
	if params == nil {
		return defaultVal
	}
	v, ok := params[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json_number:
		i, err := n.Int64()
		if err != nil {
			return defaultVal
		}
		return int(i)
	}
	return defaultVal
}

func floatParam(params map[string]any, key string, defaultVal float64) float64 {
	if params == nil {
		return defaultVal
	}
	v, ok := params[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json_number:
		f, err := n.Float64()
		if err != nil {
			return defaultVal
		}
		return f
	}
	return defaultVal
}

func strParam(params map[string]any, key string, defaultVal string) string {
	if params == nil {
		return defaultVal
	}
	v, ok := params[key]
	if !ok {
		return defaultVal
	}
	s, ok := v.(string)
	if !ok {
		return defaultVal
	}
	return s
}

type json_number = interface{ Int64() (int64, error); Float64() (float64, error) }
