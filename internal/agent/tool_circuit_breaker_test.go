package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ADR-84：熔断器名全限定。测试夹具 Agent 走共享工作空间命名空间。
func cbTestAgent() biz.Agent {
	return biz.Agent{ID: "agent-test", WorkspaceID: "ws-test"}
}

func cbSharedName(tool string) string {
	return biztool.ToolBreakerNameShared("ws-test", tool)
}

func TestCircuitBreakerIntegration_FuseAndBlock(t *testing.T) {
	registry := biztool.NewCircuitBreakerRegistry()
	registry.SetOverride(cbSharedName("test_tool"), biztool.CircuitBreakerConfig{
		FailureThreshold:   3,
		RecoveryTimeoutSec: 1,
		HalfOpenMaxProbe:   1,
	})

	ag := cbTestAgent()
	beforeHook := newCircuitBreakerBeforeHook(registry, ag, loggateway.NewNoop())
	afterHook := newCircuitBreakerAfterHook(registry, ag, loggateway.NewNoop())

	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := afterHook.HandleAfterTool(ctx, &trpctool.AfterToolArgs{
			ToolName: "test_tool",
			Error:    errors.New("tool failed"),
		})
		if err != nil {
			t.Fatalf("afterHook call %d returned error: %v", i+1, err)
		}
	}

	cb := registry.Get(cbSharedName("test_tool"), "")
	if cb.State() != biztool.CircuitOpen {
		t.Fatalf("expected open after 3 failures, got %s", cb.State())
	}

	result, err := beforeHook.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
		ToolName: "test_tool",
	})
	if err != nil {
		t.Fatalf("beforeHook returned error: %v", err)
	}
	if result.CustomResult == nil {
		t.Fatal("expected CustomResult to be set (tool should be blocked)")
	}
	msg, ok := result.CustomResult.(string)
	if !ok {
		t.Fatalf("expected CustomResult to be string, got %T", result.CustomResult)
	}
	if !strings.Contains(msg, "test_tool") || !strings.Contains(msg, "unavailable") {
		t.Fatalf("unexpected CustomResult message: %s", msg)
	}
}

func TestCircuitBreakerIntegration_Recovery(t *testing.T) {
	registry := biztool.NewCircuitBreakerRegistry()
	registry.SetOverride(cbSharedName("recovery_tool"), biztool.CircuitBreakerConfig{
		FailureThreshold:   3,
		RecoveryTimeoutSec: 1,
		HalfOpenMaxProbe:   1,
	})

	ag := cbTestAgent()
	beforeHook := newCircuitBreakerBeforeHook(registry, ag, loggateway.NewNoop())
	afterHook := newCircuitBreakerAfterHook(registry, ag, loggateway.NewNoop())

	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, _ = afterHook.HandleAfterTool(ctx, &trpctool.AfterToolArgs{
			ToolName: "recovery_tool",
			Error:    errors.New("tool failed"),
		})
	}

	cb := registry.Get(cbSharedName("recovery_tool"), "")
	if cb.State() != biztool.CircuitOpen {
		t.Fatalf("expected open, got %s", cb.State())
	}

	time.Sleep(1100 * time.Millisecond)

	result, err := beforeHook.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
		ToolName: "recovery_tool",
	})
	if err != nil {
		t.Fatalf("beforeHook returned error: %v", err)
	}
	if result.CustomResult != nil {
		t.Fatal("expected CustomResult to be nil in half-open state (probe should be allowed)")
	}

	if cb.State() != biztool.CircuitHalfOpen {
		t.Fatalf("expected half_open after recovery timeout, got %s", cb.State())
	}

	_, err = afterHook.HandleAfterTool(ctx, &trpctool.AfterToolArgs{
		ToolName: "recovery_tool",
		Result:   "success",
	})
	if err != nil {
		t.Fatalf("afterHook returned error: %v", err)
	}

	if cb.State() != biztool.CircuitClosed {
		t.Fatalf("expected closed after successful probe, got %s", cb.State())
	}

	result, err = beforeHook.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
		ToolName: "recovery_tool",
	})
	if err != nil {
		t.Fatalf("beforeHook returned error: %v", err)
	}
	if result.CustomResult != nil {
		t.Fatal("expected CustomResult to be nil in closed state")
	}
}

func TestCircuitBreakerIntegration_SystemPrompt(t *testing.T) {
	registry := biztool.NewCircuitBreakerRegistry()
	registry.SetOverride(cbSharedName("tool_alpha"), biztool.CircuitBreakerConfig{
		FailureThreshold:   2,
		RecoveryTimeoutSec: 60,
		HalfOpenMaxProbe:   1,
	})
	registry.SetOverride(cbSharedName("tool_beta"), biztool.CircuitBreakerConfig{
		FailureThreshold:   2,
		RecoveryTimeoutSec: 60,
		HalfOpenMaxProbe:   1,
	})
	// 其它工作空间的熔断器不得泄入本 Agent 的系统提示。
	registry.SetOverride(biztool.ToolBreakerNameShared("ws-other", "tool_gamma"), biztool.CircuitBreakerConfig{
		FailureThreshold:   2,
		RecoveryTimeoutSec: 60,
		HalfOpenMaxProbe:   1,
	})

	ag := cbTestAgent()

	prompt := buildCircuitBreakerSystemPrompt(registry, ag)
	if prompt != "" {
		t.Fatalf("expected empty prompt when no breakers are open, got %q", prompt)
	}

	cb1 := registry.Get(cbSharedName("tool_alpha"), "")
	cb1.RecordFailure()
	cb1.RecordFailure()

	prompt = buildCircuitBreakerSystemPrompt(registry, ag)
	if !strings.Contains(prompt, "tool_alpha") {
		t.Fatalf("expected prompt to contain tool_alpha, got %q", prompt)
	}
	if !strings.Contains(prompt, "unavailable") {
		t.Fatalf("expected prompt to contain 'unavailable', got %q", prompt)
	}

	cb2 := registry.Get(cbSharedName("tool_beta"), "")
	cb2.RecordFailure()
	cb2.RecordFailure()
	cb3 := registry.Get(biztool.ToolBreakerNameShared("ws-other", "tool_gamma"), "")
	cb3.RecordFailure()
	cb3.RecordFailure()

	prompt = buildCircuitBreakerSystemPrompt(registry, ag)
	if !strings.Contains(prompt, "tool_alpha") {
		t.Fatalf("expected prompt to contain tool_alpha, got %q", prompt)
	}
	if !strings.Contains(prompt, "tool_beta") {
		t.Fatalf("expected prompt to contain tool_beta, got %q", prompt)
	}
	if strings.Contains(prompt, "tool_gamma") {
		t.Fatalf("prompt must not leak other workspace breaker, got %q", prompt)
	}
	// 提示内必须是裸工具名（不含命名空间全限定）。
	if strings.Contains(prompt, "tool:ws-test:") {
		t.Fatalf("prompt must show bare tool names, got %q", prompt)
	}
}

func TestCircuitBreakerIntegration_JSONOverride(t *testing.T) {
	ag := biz.Agent{
		ID:          "agent-json",
		WorkspaceID: "ws-test",
		Settings: &biz.AgentRuntimeSettings{
			ToolsEnabled:               true,
			ToolsCircuitBreakerEnabled: true,
			ToolsCircuitBreakerOverridesJSON: `{
				"json_tool": {
					"failure_threshold": 5,
					"recovery_timeout_sec": 1,
					"half_open_max_probe": 2
				}
			}`,
		},
	}

	registry := biztool.NewCircuitBreakerRegistry()
	if configureSharedToolBreakers(registry, ag, loggateway.NewNoop()) == nil {
		t.Fatal("expected non-nil registry")
	}

	// per-agent overrides ⇒ Agent 专属（易失）命名空间。
	cb := registry.Get(biztool.ToolBreakerNameAgent("agent-json", "json_tool"), "web")
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != biztool.CircuitClosed {
		t.Fatalf("expected closed (threshold=5, only 4 failures), got %s", cb.State())
	}

	cb.RecordFailure()
	if cb.State() != biztool.CircuitOpen {
		t.Fatalf("expected open after 5 failures, got %s", cb.State())
	}

	time.Sleep(1100 * time.Millisecond)

	allowed, state := cb.Allow()
	if !allowed || state != biztool.CircuitHalfOpen {
		t.Fatalf("expected half_open after recovery, allowed=%v state=%s", allowed, state)
	}
}

func TestCircuitBreakerIntegration_TransientErrorIgnored(t *testing.T) {
	registry := biztool.NewCircuitBreakerRegistry()
	registry.SetOverride(cbSharedName("transient_tool"), biztool.CircuitBreakerConfig{
		FailureThreshold:   2,
		RecoveryTimeoutSec: 1,
		HalfOpenMaxProbe:   1,
	})

	ag := cbTestAgent()
	afterHook := newCircuitBreakerAfterHook(registry, ag, loggateway.NewNoop())
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, _ = afterHook.HandleAfterTool(ctx, &trpctool.AfterToolArgs{
			ToolName: "transient_tool",
			Error:    io.EOF,
		})
	}

	cb := registry.Get(cbSharedName("transient_tool"), "")
	if cb.State() != biztool.CircuitClosed {
		t.Fatalf("expected closed (transient errors should not count), got %s", cb.State())
	}

	_, _ = afterHook.HandleAfterTool(ctx, &trpctool.AfterToolArgs{
		ToolName: "transient_tool",
		Error:    fmt.Errorf("authentication failed"),
	})
	_, _ = afterHook.HandleAfterTool(ctx, &trpctool.AfterToolArgs{
		ToolName: "transient_tool",
		Error:    fmt.Errorf("service unavailable"),
	})

	if cb.State() != biztool.CircuitOpen {
		t.Fatalf("expected open after 2 persistent failures, got %s", cb.State())
	}
}

// ADR-84 共享语义：同工作空间不同 Agent 打到同一个熔断器（计数互认）。
func TestCircuitBreakerIntegration_SharedAcrossAgentsSameWorkspace(t *testing.T) {
	registry := biztool.NewCircuitBreakerRegistry()
	registry.SetOverride(cbSharedName("shared_tool"), biztool.CircuitBreakerConfig{
		FailureThreshold:   2,
		RecoveryTimeoutSec: 60,
		HalfOpenMaxProbe:   1,
	})

	ctx := context.Background()
	agA := biz.Agent{ID: "agent-a", WorkspaceID: "ws-test"}
	agB := biz.Agent{ID: "agent-b", WorkspaceID: "ws-test"}

	afterA := newCircuitBreakerAfterHook(registry, agA, loggateway.NewNoop())
	_, _ = afterA.HandleAfterTool(ctx, &trpctool.AfterToolArgs{
		ToolName: "shared_tool",
		Error:    errors.New("fail from agent-a"),
	})

	cb := registry.Get(cbSharedName("shared_tool"), "")
	if cb.State() != biztool.CircuitClosed {
		t.Fatalf("expected closed after 1 failure (threshold=2), got %s", cb.State())
	}

	// Agent B 的一次失败应把同一熔断器推过阈值。
	afterB := newCircuitBreakerAfterHook(registry, agB, loggateway.NewNoop())
	_, _ = afterB.HandleAfterTool(ctx, &trpctool.AfterToolArgs{
		ToolName: "shared_tool",
		Error:    errors.New("fail from agent-b"),
	})
	if cb.State() != biztool.CircuitOpen {
		t.Fatalf("expected open after cross-agent failures, got %s", cb.State())
	}

	beforeB := newCircuitBreakerBeforeHook(registry, agB, loggateway.NewNoop())
	res, err := beforeB.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "shared_tool"})
	if err != nil || res.CustomResult == nil {
		t.Fatalf("agent-b should be blocked by the shared open breaker, res=%+v err=%v", res, err)
	}
}

// ADR-84 隔离语义：不同工作空间互不影响。
func TestCircuitBreakerIntegration_WorkspaceIsolation(t *testing.T) {
	registry := biztool.NewCircuitBreakerRegistry()
	nameA := biztool.ToolBreakerNameShared("ws-a", "iso_tool")
	registry.SetOverride(nameA, biztool.CircuitBreakerConfig{
		FailureThreshold:   1,
		RecoveryTimeoutSec: 60,
		HalfOpenMaxProbe:   1,
	})

	ctx := context.Background()
	agA := biz.Agent{ID: "agent-a", WorkspaceID: "ws-a"}
	agB := biz.Agent{ID: "agent-b", WorkspaceID: "ws-b"}

	afterA := newCircuitBreakerAfterHook(registry, agA, loggateway.NewNoop())
	_, _ = afterA.HandleAfterTool(ctx, &trpctool.AfterToolArgs{
		ToolName: "iso_tool",
		Error:    errors.New("ws-a failure"),
	})
	if cb := registry.Get(nameA, ""); cb.State() != biztool.CircuitOpen {
		t.Fatalf("ws-a breaker should be open, got %s", cb.State())
	}

	// ws-b 同名工具仍是独立熔断器，放行。
	beforeB := newCircuitBreakerBeforeHook(registry, agB, loggateway.NewNoop())
	res, err := beforeB.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "iso_tool"})
	if err != nil {
		t.Fatalf("beforeHook returned error: %v", err)
	}
	if res.CustomResult != nil {
		t.Fatalf("ws-b must not be blocked by ws-a breaker, got %v", res.CustomResult)
	}
}

// ADR-84：Agent 专属 overrides 不与共享熔断器串扰。
func TestCircuitBreakerIntegration_AgentScopeIsolatedFromShared(t *testing.T) {
	registry := biztool.NewCircuitBreakerRegistry()
	agOverride := biz.Agent{
		ID:          "agent-ov",
		WorkspaceID: "ws-test",
		Settings: &biz.AgentRuntimeSettings{
			ToolsEnabled:               true,
			ToolsCircuitBreakerEnabled: true,
			ToolsCircuitBreakerOverridesJSON: `{
				"scoped_tool": {"failure_threshold": 1, "recovery_timeout_sec": 60, "half_open_max_probe": 1}
			}`,
		},
	}
	configureSharedToolBreakers(registry, agOverride, loggateway.NewNoop())

	ctx := context.Background()
	after := newCircuitBreakerAfterHook(registry, agOverride, loggateway.NewNoop())
	_, _ = after.HandleAfterTool(ctx, &trpctool.AfterToolArgs{
		ToolName: "scoped_tool",
		Error:    errors.New("boom"),
	})

	// Agent 专属熔断器已开。
	if cb := registry.Get(biztool.ToolBreakerNameAgent("agent-ov", "scoped_tool"), ""); cb.State() != biztool.CircuitOpen {
		t.Fatalf("agent-scoped breaker should be open, got %s", cb.State())
	}
	// 共享熔断器不受影响（甚至尚未创建）。
	shared := biz.Agent{ID: "agent-plain", WorkspaceID: "ws-test"}
	before := newCircuitBreakerBeforeHook(registry, shared, loggateway.NewNoop())
	res, err := before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "scoped_tool"})
	if err != nil || res.CustomResult != nil {
		t.Fatalf("shared (non-override) agent must not see agent-scoped breaker, res=%+v err=%v", res, err)
	}
}
