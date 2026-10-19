package tool

import (
	"context"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// ADR-84 工具熔断命名空间：circuit_breaker_states.key 一律全限定。
// tool:<workspaceID>:<toolName> 工作空间内跨 Agent 共享、持久化；
// tool_agent:<agentID>:<toolName> Agent 配置了 per-agent overrides 时使用，
// 保持 per-agent 隔离语义、易失不落表（避免随 Agent 生命周期产生孤儿行）。
const (
	// ToolCBNamespaceShared 共享（持久化）熔断器名前缀。
	ToolCBNamespaceShared = "tool"
	// ToolCBNamespaceAgent Agent 专属（易失）熔断器名前缀。
	ToolCBNamespaceAgent = "tool_agent"
)

// ToolBreakerNameShared 组合工作空间共享熔断器名（持久化）。
func ToolBreakerNameShared(workspaceID, toolName string) string {
	return ToolCBNamespaceShared + ":" + workspaceID + ":" + toolName
}

// ToolBreakerNameAgent 组合 Agent 专属熔断器名（易失、隔离 overrides）。
func ToolBreakerNameAgent(agentID, toolName string) string {
	return ToolCBNamespaceAgent + ":" + agentID + ":" + toolName
}

// ToolCBPersists 是 ADR-84 的持久化过滤：仅共享命名空间落表/恢复。
func ToolCBPersists(name string) bool {
	return strings.HasPrefix(name, ToolCBNamespaceShared+":")
}

// ProvideToolCircuitBreakerRegistry 构建进程级共享工具熔断注册表（ADR-84，
// 与 ProvideNodeCircuitBreakerRegistry 对齐）：接 StateRepo、仅 tool: 前缀
// 持久化、启动即 RestoreStates 恢复 Open 状态。repo 为 nil 时降级为全易失
// 并 Warn（TPM-P2-02：缺配置不拖垮装配）。
func ProvideToolCircuitBreakerRegistry(repo CircuitBreakerStateRepo, lg loggateway.Logger) *CircuitBreakerRegistry {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	opts := []CircuitBreakerRegistryOption{
		WithLogger(lg),
		WithPersistFilter(ToolCBPersists),
		WithRegistryOnStateChange(func(name string, from, to CircuitState) {
			lg.Info("circuit breaker state changed",
				loggateway.StepID("tool.circuit_breaker.state_change"),
				loggateway.Str("breaker", name),
				loggateway.Str("from", string(from)),
				loggateway.Str("to", string(to)),
			)
		}),
	}
	if repo != nil {
		opts = append(opts, WithStateRepo(repo))
	} else {
		lg.Warn("tool circuit breaker state repo unavailable; breakers are volatile",
			loggateway.StepID("tool.circuit_breaker.no_state_repo"))
	}
	reg := NewCircuitBreakerRegistry(opts...)
	reg.RestoreStates(context.Background())
	return reg
}
