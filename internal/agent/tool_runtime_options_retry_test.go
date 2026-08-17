package agent

import (
	"context"
	"io"
	"testing"

	"aranea-agents/internal/biz"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestBuildToolRetryPolicy_Disabled(t *testing.T) {
	if got := buildToolRetryPolicy(&biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsRetryEnabled: false}); got != nil {
		t.Fatalf("retry disabled should yield nil policy, got %#v", got)
	}
	if got := buildToolRetryPolicy(&biz.AgentRuntimeSettings{ToolsEnabled: false, ToolsRetryEnabled: true}); got != nil {
		t.Fatalf("tools disabled should yield nil policy, got %#v", got)
	}
}

func TestBuildToolRetryPolicy_UsesSelectiveRetryOn(t *testing.T) {
	s := biz.DefaultAgentRuntimeSettings()
	policy := buildToolRetryPolicy(&s)
	if policy == nil {
		t.Fatal("default settings should enable a retry policy")
	}
	if policy.MaxAttempts != defaultRetryMaxAttempts {
		t.Fatalf("MaxAttempts = %d, want %d", policy.MaxAttempts, defaultRetryMaxAttempts)
	}
	if policy.RetryOn == nil {
		t.Fatal("RetryOn must be set")
	}

	retry, err := policy.RetryOn(context.Background(), &trpctool.RetryInfo{
		ToolName: "read_file",
		Error:    io.ErrUnexpectedEOF,
	})
	if err != nil {
		t.Fatalf("read_file RetryOn: %v", err)
	}
	if !retry {
		t.Fatal("default policy should retry ConcurrentSafe transient errors")
	}

	retry, err = policy.RetryOn(context.Background(), &trpctool.RetryInfo{
		ToolName: "exec_command",
		Error:    io.ErrUnexpectedEOF,
	})
	if err != nil {
		t.Fatalf("exec_command RetryOn: %v", err)
	}
	if retry {
		t.Fatal("default policy must not retry Exclusive tools")
	}
}
