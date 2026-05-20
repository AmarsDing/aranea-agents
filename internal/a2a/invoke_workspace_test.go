package a2a

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"

	"context"
)

func TestValidateAdminInvokeWorkspace(t *testing.T) {
	t.Parallel()
	card := biz.A2AAgentCard{Workspace: "team-a"}
	ctx := workspace.WithContext(context.Background(), "team-a")
	if err := ValidateAdminInvokeWorkspace(ctx, "team-a", card); err != nil {
		t.Fatalf("expected pass: %v", err)
	}
	ctx = workspace.WithContext(context.Background(), "team-b")
	if err := ValidateAdminInvokeWorkspace(ctx, "", card); err == nil {
		t.Fatal("expected cross-workspace error")
	}
	if err := ValidateAdminInvokeWorkspace(context.Background(), "team-b", card); err == nil || !strings.Contains(err.Error(), "requested workspace") {
		t.Fatalf("expected request workspace mismatch, got %v", err)
	}
}
