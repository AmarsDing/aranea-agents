package workspace_test

import (
	"context"
	"testing"

	"aranea-agents/internal/workspace"
)

func TestWithContextAndFromContext(t *testing.T) {
	ctx := workspace.WithContext(context.Background(), "tenant-1")
	id, ok := workspace.FromContext(ctx)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if id != "tenant-1" {
		t.Fatalf("expected tenant-1, got %s", id)
	}
}

func TestFromContextEmpty(t *testing.T) {
	_, ok := workspace.FromContext(context.Background())
	if ok {
		t.Fatal("expected ok=false for empty context")
	}
}

func TestIDFromContextDefault(t *testing.T) {
	id := workspace.IDFromContext(context.Background())
	if id != workspace.DefaultWorkspaceID {
		t.Fatalf("expected default, got %s", id)
	}
}

func TestIDFromContextWithValue(t *testing.T) {
	ctx := workspace.WithContext(context.Background(), "ws-42")
	id := workspace.IDFromContext(ctx)
	if id != "ws-42" {
		t.Fatalf("expected ws-42, got %s", id)
	}
}

func TestWithSystemWorkspace(t *testing.T) {
	ctx := workspace.WithSystemWorkspace(context.Background())
	if !workspace.IsSystem(ctx) {
		t.Fatal("expected IsSystem=true after WithSystemWorkspace")
	}
}

func TestIsSystemFalseForRegular(t *testing.T) {
	ctx := workspace.WithContext(context.Background(), "tenant-x")
	if workspace.IsSystem(ctx) {
		t.Fatal("expected IsSystem=false for regular workspace")
	}
}
