package security

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestCommandSafetyPermissionChecker_AllowNonProtected(t *testing.T) {
	checker := NewCommandSafetyPermissionChecker(loggateway.NewNoop())

	req := &trpctool.PermissionRequest{
		ToolName:  "web_research",
		Arguments: []byte(`{"query": "golang testing"}`),
	}
	decision, err := checker.CheckPermission(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Action != trpctool.PermissionActionAllow {
		t.Errorf("expected allow, got %s", decision.Action)
	}
}

func TestCommandSafetyPermissionChecker_DenyProtectedPath(t *testing.T) {
	checker := NewCommandSafetyPermissionChecker(loggateway.NewNoop())

	req := &trpctool.PermissionRequest{
		ToolName:  "exec_command",
		Arguments: []byte(`{"command": "cat ~/.ssh/id_rsa"}`),
	}
	decision, err := checker.CheckPermission(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Action != trpctool.PermissionActionDeny {
		t.Errorf("expected deny, got %s", decision.Action)
	}
	if decision.Reason == "" {
		t.Error("expected non-empty deny reason")
	}
}

func TestCommandSafetyPermissionChecker_DenyFileWithSensitivePath(t *testing.T) {
	checker := NewCommandSafetyPermissionChecker(loggateway.NewNoop())

	req := &trpctool.PermissionRequest{
		ToolName:  "file",
		Arguments: []byte(`{"path": "/home/user/.aws/credentials"}`),
	}
	decision, err := checker.CheckPermission(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Action != trpctool.PermissionActionDeny {
		t.Errorf("expected deny, got %s", decision.Action)
	}
}

func TestCommandSafetyPermissionChecker_AllowProtectedToolSafePath(t *testing.T) {
	checker := NewCommandSafetyPermissionChecker(loggateway.NewNoop())

	req := &trpctool.PermissionRequest{
		ToolName:  "exec_command",
		Arguments: []byte(`{"command": "ls /tmp/workdir"}`),
	}
	decision, err := checker.CheckPermission(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Action != trpctool.PermissionActionAllow {
		t.Errorf("expected allow for safe path, got %s", decision.Action)
	}
}

func TestCommandSafetyPermissionChecker_NilRequest(t *testing.T) {
	checker := NewCommandSafetyPermissionChecker(loggateway.NewNoop())

	decision, err := checker.CheckPermission(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Action != trpctool.PermissionActionAllow {
		t.Errorf("expected allow for nil request, got %s", decision.Action)
	}
}

func TestCommandSafetyPermissionChecker_WithCustomPolicy(t *testing.T) {
	policy := NewCommandSafetyPolicyWithConfig(
		loggateway.NewNoop(),
		[]string{"custom_secret/*"},
		map[string]bool{"custom_tool": true},
	)
	checker, err := NewCommandSafetyPermissionCheckerWithPolicy(policy, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Custom protected tool + custom protected path → deny
	req := &trpctool.PermissionRequest{
		ToolName:  "custom_tool",
		Arguments: []byte(`{"path": "/opt/custom_secret/data.pem"}`),
	}
	decision, err := checker.CheckPermission(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Action != trpctool.PermissionActionDeny {
		t.Errorf("expected deny for custom protected path, got %s", decision.Action)
	}

	// Default protected tool + safe path → allow
	req2 := &trpctool.PermissionRequest{
		ToolName:  "exec_command",
		Arguments: []byte(`{"command": "ls /tmp"}`),
	}
	decision2, err := checker.CheckPermission(context.Background(), req2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision2.Action != trpctool.PermissionActionAllow {
		t.Errorf("expected allow for safe path, got %s", decision2.Action)
	}
}

func TestCommandSafetyPermissionChecker_NilPolicyReturnsError(t *testing.T) {
	_, err := NewCommandSafetyPermissionCheckerWithPolicy(nil, loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error when policy is nil")
	}
}
