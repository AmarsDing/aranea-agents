package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func buildCircuitBreakerRegistry(s *biz.AgentRuntimeSettings, lg loggateway.Logger) *biztool.CircuitBreakerRegistry {
	if !s.ToolsEnabled || !s.ToolsCircuitBreakerEnabled {
		return nil
	}
	registry := biztool.NewCircuitBreakerRegistry(
		biztool.WithLogger(lg),
		biztool.WithRegistryOnStateChange(func(name string, from, to biztool.CircuitState) {
			lg.Info("circuit breaker state changed",
				loggateway.StepID("tool.circuit_breaker.state_change"),
				loggateway.Str("tool", name),
				loggateway.Str("from", string(from)),
				loggateway.Str("to", string(to)),
			)
		}),
	)
	if s.ToolsCircuitBreakerOverridesJSON != "" {
		var overrides map[string]biztool.CircuitBreakerConfig
		if err := json.Unmarshal([]byte(s.ToolsCircuitBreakerOverridesJSON), &overrides); err == nil {
			for name, cfg := range overrides {
				registry.SetOverride(name, cfg)
			}
		} else {
			lg.Warn("circuit breaker overrides JSON parse failed",
				loggateway.StepID("agent.circuit_breaker_config_parse_fail"),
				loggateway.Err(err),
			)
		}
	}
	return registry
}

var (
	categoryCache   map[string]string
	categoryCacheMu sync.RWMutex
)

func categoryForTool(toolName string) string {
	categoryCacheMu.RLock()
	if categoryCache != nil {
		if cat, ok := categoryCache[toolName]; ok {
			categoryCacheMu.RUnlock()
			return cat
		}
	}
	categoryCacheMu.RUnlock()

	// Lazy-build the cache on first miss.
	categoryCacheMu.Lock()
	defer categoryCacheMu.Unlock()
	if categoryCache == nil {
		regs := tools.Registry()
		categoryCache = make(map[string]string, len(regs))
		for _, reg := range regs {
			categoryCache[reg.Name] = reg.Category
		}
		if cat, ok := categoryCache[toolName]; ok {
			return cat
		}
	}
	return ""
}

// InvalidateCategoryCache clears the category cache so that newly
// registered tools (e.g. MCP tools) are picked up on the next lookup.
func InvalidateCategoryCache() {
	categoryCacheMu.Lock()
	categoryCache = nil
	categoryCacheMu.Unlock()
}

func newCircuitBreakerBeforeHook(registry *biztool.CircuitBreakerRegistry, lg loggateway.Logger) *circuitBreakerBeforeHook {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &circuitBreakerBeforeHook{registry: registry, lg: lg}
}

type circuitBreakerBeforeHook struct {
	registry *biztool.CircuitBreakerRegistry
	lg       loggateway.Logger
}

func (h *circuitBreakerBeforeHook) Point() callbacks.CallbackPoint { return callbacks.PointBeforeTool }
func (h *circuitBreakerBeforeHook) Priority() int                  { return 5 }

func (h *circuitBreakerBeforeHook) HandleBeforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	cb := h.registry.Get(args.ToolName, categoryForTool(args.ToolName))
	allowed, state := cb.Allow()
	if !allowed {
		h.lg.Warn("circuit breaker blocked tool call",
			loggateway.StepID("agent.circuit_breaker"),
			loggateway.Str("tool", args.ToolName),
			loggateway.Str("state", string(state)),
		)
		return &trpctool.BeforeToolResult{
			CustomResult: fmt.Sprintf("Tool %q is temporarily unavailable due to repeated failures (circuit breaker state: %s). Please try an alternative tool or retry later.", args.ToolName, state),
		}, nil
	}
	return &trpctool.BeforeToolResult{}, nil
}

func newCircuitBreakerAfterHook(registry *biztool.CircuitBreakerRegistry, lg loggateway.Logger) *circuitBreakerAfterHook {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &circuitBreakerAfterHook{registry: registry, lg: lg}
}

type circuitBreakerAfterHook struct {
	registry *biztool.CircuitBreakerRegistry
	lg       loggateway.Logger
}

func (h *circuitBreakerAfterHook) Point() callbacks.CallbackPoint { return callbacks.PointAfterTool }
func (h *circuitBreakerAfterHook) Priority() int                  { return 5 }

func (h *circuitBreakerAfterHook) HandleAfterTool(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
	cb := h.registry.Get(args.ToolName, categoryForTool(args.ToolName))
	if args.Error != nil {
		if biztool.IsTransientError(args.Error) {
			h.lg.Debug("circuit breaker skipping transient error",
				loggateway.StepID("tool.circuit_breaker.transient_skip"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Err(args.Error),
			)
			return &trpctool.AfterToolResult{}, nil
		}
		cb.RecordFailure()
	} else {
		cb.RecordSuccess()
	}
	return &trpctool.AfterToolResult{}, nil
}

func buildCircuitBreakerSystemPrompt(registry *biztool.CircuitBreakerRegistry) string {
	openBreakers := registry.OpenBreakers()
	if len(openBreakers) == 0 {
		return ""
	}
	prompt := "The following tools are currently unavailable due to service issues: "
	for i, name := range openBreakers {
		if i > 0 {
			prompt += ", "
		}
		prompt += name
	}
	prompt += ". Please use alternative tools or approaches instead."
	return prompt
}
