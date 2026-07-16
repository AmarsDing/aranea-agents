package service

import (
	"context"
	"testing"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
)

func TestAuthorizeMemoryScope(t *testing.T) {
	t.Run("system_bypass", func(t *testing.T) {
		ctx := workspace.WithSystemWorkspace(context.Background())
		id, err := authorizeMemoryScope(ctx, "user", "999", true)
		if err != nil || id != "999" {
			t.Fatalf("system bypass: id=%q err=%v", id, err)
		}
	})

	t.Run("unauthenticated", func(t *testing.T) {
		ctx := workspace.WithContext(context.Background(), "ws-1")
		_, err := authorizeMemoryScope(ctx, "user", "1", false)
		if err != auth.ErrUnauthorized {
			t.Fatalf("expected unauthorized, got %v", err)
		}
	})

	t.Run("user_self", func(t *testing.T) {
		ctx := auth.NewContext(workspace.WithContext(context.Background(), "ws-1"), &auth.Auth{UserID: 42, Access: "user"})
		id, err := authorizeMemoryScope(ctx, "user", "", false)
		if err != nil || id != "42" {
			t.Fatalf("expected scope_id=42, got %q err=%v", id, err)
		}
	})

	t.Run("user_other_forbidden", func(t *testing.T) {
		ctx := auth.NewContext(workspace.WithContext(context.Background(), "ws-1"), &auth.Auth{UserID: 42, Access: "user"})
		_, err := authorizeMemoryScope(ctx, "user", "99", false)
		if err == nil {
			t.Fatal("expected forbidden for other user scope")
		}
		ae, ok := apierror.From(err)
		if !ok || ae.Code != apierror.CodeForbidden {
			t.Fatalf("expected FORBIDDEN, got %v", err)
		}
	})

	t.Run("user_other_admin_ok", func(t *testing.T) {
		ctx := auth.NewContext(workspace.WithContext(context.Background(), "ws-1"), &auth.Auth{UserID: 42, Access: "admin"})
		id, err := authorizeMemoryScope(ctx, "user", "99", false)
		if err != nil || id != "99" {
			t.Fatalf("admin should access other user: id=%q err=%v", id, err)
		}
	})

	t.Run("workspace_mismatch", func(t *testing.T) {
		ctx := auth.NewContext(workspace.WithContext(context.Background(), "ws-1"), &auth.Auth{UserID: 1, Access: "admin"})
		_, err := authorizeMemoryScope(ctx, "workspace", "ws-2", false)
		if err == nil {
			t.Fatal("expected forbidden for cross-workspace")
		}
	})

	t.Run("workspace_empty_forced", func(t *testing.T) {
		ctx := auth.NewContext(workspace.WithContext(context.Background(), "ws-1"), &auth.Auth{UserID: 1, Access: "user"})
		id, err := authorizeMemoryScope(ctx, "workspace", "", false)
		if err != nil || id != "ws-1" {
			t.Fatalf("expected ws-1, got %q err=%v", id, err)
		}
	})

	t.Run("global_write_requires_admin", func(t *testing.T) {
		ctx := auth.NewContext(workspace.WithContext(context.Background(), "ws-1"), &auth.Auth{UserID: 1, Access: "user"})
		_, err := authorizeMemoryScope(ctx, "global", "", true)
		if err == nil {
			t.Fatal("expected forbidden for global write without admin")
		}
	})

	t.Run("global_write_admin_ok", func(t *testing.T) {
		ctx := auth.NewContext(workspace.WithContext(context.Background(), "ws-1"), &auth.Auth{UserID: 1, Access: "admin"})
		_, err := authorizeMemoryScope(ctx, "global", "", true)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("agent_write_requires_scope_id", func(t *testing.T) {
		ctx := auth.NewContext(workspace.WithContext(context.Background(), "ws-1"), &auth.Auth{UserID: 1, Access: "user"})
		_, err := authorizeMemoryScope(ctx, "agent", "", true)
		if err == nil {
			t.Fatal("expected bad request for empty agent scope_id on write")
		}
	})

	t.Run("empty_scope_read_non_admin_forced_user", func(t *testing.T) {
		ctx := auth.NewContext(workspace.WithContext(context.Background(), "ws-1"), &auth.Auth{UserID: 7, Access: "user"})
		id, err := authorizeMemoryScope(ctx, "", "", false)
		if err != nil || id != "7" {
			t.Fatalf("expected user scope 7, got %q err=%v", id, err)
		}
	})

	t.Run("empty_scope_write_rejected", func(t *testing.T) {
		ctx := auth.NewContext(workspace.WithContext(context.Background(), "ws-1"), &auth.Auth{UserID: 1, Access: "admin"})
		_, err := authorizeMemoryScope(ctx, "", "", true)
		if err == nil {
			t.Fatal("expected bad request for empty scope_type on write")
		}
	})
}

func TestAuthorizeMemoryWorkspaceField(t *testing.T) {
	t.Run("force_caller_ws", func(t *testing.T) {
		ctx := workspace.WithContext(context.Background(), "ws-1")
		id, err := authorizeMemoryWorkspaceField(ctx, "")
		if err != nil || id != "ws-1" {
			t.Fatalf("got %q err=%v", id, err)
		}
	})
	t.Run("reject_other_ws", func(t *testing.T) {
		ctx := workspace.WithContext(context.Background(), "ws-1")
		_, err := authorizeMemoryWorkspaceField(ctx, "ws-2")
		if err == nil {
			t.Fatal("expected forbidden")
		}
	})
}
