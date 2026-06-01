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

func TestCircuitBreakerIntegration_FuseAndBlock(t *testing.T) {
	registry := biztool.NewCircuitBreakerRegistry()
	registry.SetOverride("test_tool", biztool.CircuitBreakerConfig{
		FailureThreshold:   3,
		RecoveryTimeoutSec: 1,
		HalfOpenMaxProbe:   1,
	})

	beforeHook := newCircuitBreakerBeforeHook(registry, loggateway.Global())
	afterHook := newCircuitBreakerAfterHook(registry, loggateway.Global())

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

	cb := registry.Get("test_tool", "")
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
	registry.SetOverride("recovery_tool", biztool.CircuitBreakerConfig{
		FailureThreshold:   3,
		RecoveryTimeoutSec: 1,
		HalfOpenMaxProbe:   1,
	})

	beforeHook := newCircuitBreakerBeforeHook(registry, loggateway.Global())
	afterHook := newCircuitBreakerAfterHook(registry, loggateway.Global())

	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, _ = afterHook.HandleAfterTool(ctx, &trpctool.AfterToolArgs{
			ToolName: "recovery_tool",
			Error:    errors.New("tool failed"),
		})
	}

	cb := registry.Get("recovery_tool", "")
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
	registry.SetOverride("tool_alpha", biztool.CircuitBreakerConfig{
		FailureThreshold:   2,
		RecoveryTimeoutSec: 60,
		HalfOpenMaxProbe:   1,
	})
	registry.SetOverride("tool_beta", biztool.CircuitBreakerConfig{
		FailureThreshold:   2,
		RecoveryTimeoutSec: 60,
		HalfOpenMaxProbe:   1,
	})

	prompt := buildCircuitBreakerSystemPrompt(registry)
	if prompt != "" {
		t.Fatalf("expected empty prompt when no breakers are open, got %q", prompt)
	}

	cb1 := registry.Get("tool_alpha", "")
	cb1.RecordFailure()
	cb1.RecordFailure()

	prompt = buildCircuitBreakerSystemPrompt(registry)
	if !strings.Contains(prompt, "tool_alpha") {
		t.Fatalf("expected prompt to contain tool_alpha, got %q", prompt)
	}
	if !strings.Contains(prompt, "unavailable") {
		t.Fatalf("expected prompt to contain 'unavailable', got %q", prompt)
	}

	cb2 := registry.Get("tool_beta", "")
	cb2.RecordFailure()
	cb2.RecordFailure()

	prompt = buildCircuitBreakerSystemPrompt(registry)
	if !strings.Contains(prompt, "tool_alpha") {
		t.Fatalf("expected prompt to contain tool_alpha, got %q", prompt)
	}
	if !strings.Contains(prompt, "tool_beta") {
		t.Fatalf("expected prompt to contain tool_beta, got %q", prompt)
	}
}

func TestCircuitBreakerIntegration_JSONOverride(t *testing.T) {
	settings := &biz.AgentRuntimeSettings{
		ToolsEnabled:               true,
		ToolsCircuitBreakerEnabled: true,
		ToolsCircuitBreakerOverridesJSON: `{
			"json_tool": {
				"failure_threshold": 5,
				"recovery_timeout_sec": 1,
				"half_open_max_probe": 2
			}
		}`,
	}

	registry := buildCircuitBreakerRegistry(settings, loggateway.Global())
	if registry == nil {
		t.Fatal("expected non-nil registry")
	}

	cb := registry.Get("json_tool", "web")
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
	registry.SetOverride("transient_tool", biztool.CircuitBreakerConfig{
		FailureThreshold:   2,
		RecoveryTimeoutSec: 1,
		HalfOpenMaxProbe:   1,
	})

	afterHook := newCircuitBreakerAfterHook(registry, loggateway.Global())
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, _ = afterHook.HandleAfterTool(ctx, &trpctool.AfterToolArgs{
			ToolName: "transient_tool",
			Error:    io.EOF,
		})
	}

	cb := registry.Get("transient_tool", "")
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
