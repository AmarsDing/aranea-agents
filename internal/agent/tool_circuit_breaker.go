package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ADR-84：工具熔断注册表为进程级共享单例（经 TRPCBuilderDeps.CircuitBreakerRegistry
// 注入），breaker 名全限定——默认 tool:<workspaceID>:<tool>（跨 Agent 共享、
// StateRepo 持久化）；Agent 配置 per-agent overrides 时用
// tool_agent:<agentID>:<tool>（隔离、易失）。旧的 per-build 新建注册表做法
// 会在 Agent 重建/进程重启时静默复位 Open 状态，且跨 Agent 失败计数互不认账，
// 已废弃。

// toolBreakerWorkspace 解析熔断命名空间的工作空间 ID：Agent 归属工作空间优先，
// 空（历史数据）回退 default。
func toolBreakerWorkspace(ag biz.Agent) string {
	if ws := strings.TrimSpace(ag.WorkspaceID); ws != "" {
		return ws
	}
	return workspace.DefaultWorkspaceID
}

// toolBreakerUsesAgentScope 判定该 Agent 是否走专属（易失）熔断命名空间：
// 仅当其显式配置了 per-agent overrides JSON。
func toolBreakerUsesAgentScope(ag biz.Agent) bool {
	return ag.Settings != nil && strings.TrimSpace(ag.Settings.ToolsCircuitBreakerOverridesJSON) != ""
}

// toolBreakerName 组合该 Agent 视角下某工具的熔断器全限定名。
func toolBreakerName(ag biz.Agent, toolName string) string {
	if toolBreakerUsesAgentScope(ag) {
		return biztool.ToolBreakerNameAgent(ag.ID, toolName)
	}
	return biztool.ToolBreakerNameShared(toolBreakerWorkspace(ag), toolName)
}

// configureSharedToolBreakers 在共享注册表上应用该 Agent 的 per-agent
// overrides（若有）。SetOverride 按 breaker 名索引，Agent 全限定名不产生
// 跨 Agent 串扰。registry 为 nil（未注入/禁用）时返回 nil。
func configureSharedToolBreakers(registry *biztool.CircuitBreakerRegistry, ag biz.Agent, lg loggateway.Logger) *biztool.CircuitBreakerRegistry {
	if registry == nil {
		return nil
	}
	if !toolBreakerUsesAgentScope(ag) {
		return registry
	}
	var overrides map[string]biztool.CircuitBreakerConfig
	if err := json.Unmarshal([]byte(ag.Settings.ToolsCircuitBreakerOverridesJSON), &overrides); err == nil {
		for name, cfg := range overrides {
			registry.SetOverride(biztool.ToolBreakerNameAgent(ag.ID, name), cfg)
		}
	} else {
		lg.Warn("circuit breaker overrides JSON parse failed",
			loggateway.StepID("agent.circuit_breaker_config_parse_fail"),
			loggateway.Err(err),
		)
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

func newCircuitBreakerBeforeHook(registry *biztool.CircuitBreakerRegistry, ag biz.Agent, lg loggateway.Logger) *circuitBreakerBeforeHook {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &circuitBreakerBeforeHook{registry: registry, ag: ag, lg: lg}
}

type circuitBreakerBeforeHook struct {
	registry *biztool.CircuitBreakerRegistry
	ag       biz.Agent
	lg       loggateway.Logger
}

func (h *circuitBreakerBeforeHook) Point() callbacks.CallbackPoint { return callbacks.PointBeforeTool }
func (h *circuitBreakerBeforeHook) Priority() int                  { return 5 }

func (h *circuitBreakerBeforeHook) HandleBeforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	name := toolBreakerName(h.ag, args.ToolName)
	cb := h.registry.Get(name, categoryForTool(args.ToolName))
	allowed, state := cb.Allow()
	if !allowed {
		h.lg.Warn("circuit breaker blocked tool call",
			loggateway.StepID("agent.circuit_breaker"),
			loggateway.Str("breaker", name),
			loggateway.Str("tool", args.ToolName),
			loggateway.Str("state", string(state)),
		)
		return &trpctool.BeforeToolResult{
			CustomResult: fmt.Sprintf("Tool %q is temporarily unavailable due to repeated failures (circuit breaker state: %s). Please try an alternative tool or retry later.", args.ToolName, state),
		}, nil
	}
	return &trpctool.BeforeToolResult{}, nil
}

func newCircuitBreakerAfterHook(registry *biztool.CircuitBreakerRegistry, ag biz.Agent, lg loggateway.Logger) *circuitBreakerAfterHook {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &circuitBreakerAfterHook{registry: registry, ag: ag, lg: lg}
}

type circuitBreakerAfterHook struct {
	registry *biztool.CircuitBreakerRegistry
	ag       biz.Agent
	lg       loggateway.Logger
}

func (h *circuitBreakerAfterHook) Point() callbacks.CallbackPoint { return callbacks.PointAfterTool }
func (h *circuitBreakerAfterHook) Priority() int                  { return 5 }

func (h *circuitBreakerAfterHook) HandleAfterTool(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
	name := toolBreakerName(h.ag, args.ToolName)
	cb := h.registry.Get(name, categoryForTool(args.ToolName))
	if args.Error != nil {
		if biztool.IsTransientError(args.Error) {
			h.lg.Debug("circuit breaker skipping transient error",
				loggateway.StepID("tool.circuit_breaker.transient_skip"),
				loggateway.Str("breaker", name),
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

// buildCircuitBreakerSystemPrompt 只聚合该 Agent 可见命名空间内的 Open
// 熔断器（共享工作空间前缀 + 本 Agent 专属前缀），提示里展示裸工具名。
func buildCircuitBreakerSystemPrompt(registry *biztool.CircuitBreakerRegistry, ag biz.Agent) string {
	if registry == nil {
		return ""
	}
	sharedPrefix := biztool.ToolCBNamespaceShared + ":" + toolBreakerWorkspace(ag) + ":"
	agentPrefix := biztool.ToolCBNamespaceAgent + ":" + ag.ID + ":"
	var names []string
	for _, full := range registry.OpenBreakers() {
		if rest, ok := strings.CutPrefix(full, sharedPrefix); ok {
			names = append(names, rest)
		} else if rest, ok := strings.CutPrefix(full, agentPrefix); ok {
			names = append(names, rest)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return "The following tools are currently unavailable due to service issues: " +
		strings.Join(names, ", ") +
		". Please use alternative tools or approaches instead."
}
