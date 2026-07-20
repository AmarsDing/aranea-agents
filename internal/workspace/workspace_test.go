package workspace

import (
	"testing"

	"aranea-agents/pkg/apierror"
)

// TestAssertWorkspace_SystemBypass verifies that the system workspace
// (used by cron/admin background tasks) bypasses the check regardless
// of the resource workspace.
func TestAssertWorkspace_SystemBypass(t *testing.T) {
	cases := []struct {
		name       string
		callerWS   string
		resourceWS string
	}{
		{"system caller, non-empty resource", SystemWorkspaceID, "ws-1"},
		{"system caller, empty resource", SystemWorkspaceID, ""},
		{"system caller, default resource", SystemWorkspaceID, DefaultWorkspaceID},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := AssertWorkspace(c.callerWS, c.resourceWS); err != nil {
				t.Fatalf("system workspace should bypass, got %v", err)
			}
		})
	}
}

// TestAssertWorkspace_SameWorkspace verifies that a caller can access
// a resource in the same workspace.
func TestAssertWorkspace_SameWorkspace(t *testing.T) {
	cases := []string{"ws-1", "default", "tenant-abc"}
	for _, ws := range cases {
		t.Run("caller="+ws, func(t *testing.T) {
			if err := AssertWorkspace(ws, ws); err != nil {
				t.Fatalf("same workspace %q should pass, got %v", ws, err)
			}
		})
	}
}

// TestAssertWorkspace_EmptyResourceTreatedAsDefault verifies the P1-2
// security fix: empty resourceWorkspaceID is normalized to DefaultWorkspaceID.
// Legacy data (no WorkspaceID) belongs to the default workspace; only default
// workspace callers may access it. This closes the pre-P1-2 IDOR hole where
// empty resource = "allow any caller".
func TestAssertWorkspace_EmptyResourceTreatedAsDefault(t *testing.T) {
	// Default-workspace caller → allowed (legacy data belongs to default).
	if err := AssertWorkspace(DefaultWorkspaceID, ""); err != nil {
		t.Fatalf("default caller accessing empty resource should pass, got %v", err)
	}

	// Non-default caller → forbidden (cross-tenant on legacy data).
	err := AssertWorkspace("ws-1", "")
	if err == nil {
		t.Fatal("expected error for ws-1 caller accessing empty (default) resource, got nil")
	}
	ae, ok := apierror.From(err)
	if !ok {
		t.Fatal("expected *apierror.Error")
	}
	if ae.Code != apierror.CodeForbidden {
		t.Fatalf("expected FORBIDDEN code, got %s", ae.Code)
	}
}

// TestAssertWorkspace_DifferentWorkspace verifies that cross-tenant
// access is rejected with Forbidden.
func TestAssertWorkspace_DifferentWorkspace(t *testing.T) {
	err := AssertWorkspace("ws-1", "ws-2")
	if err == nil {
		t.Fatal("expected error for cross-workspace access, got nil")
	}
	ae, ok := apierror.From(err)
	if !ok {
		t.Fatal("expected *apierror.Error")
	}
	if ae.Code != apierror.CodeForbidden {
		t.Fatalf("expected FORBIDDEN code, got %s", ae.Code)
	}
	if ae.Domain != DomainWorkspace {
		t.Fatalf("expected domain %q, got %q", DomainWorkspace, ae.Domain)
	}
}

// TestAssertWorkspace_NonSystemCallerWithSystemResource verifies that
// a non-system caller cannot access a resource nominally owned by the
// system workspace (system bypass is one-way: only system *callers* bypass).
func TestAssertWorkspace_NonSystemCallerWithSystemResource(t *testing.T) {
	err := AssertWorkspace("ws-1", SystemWorkspaceID)
	if err == nil {
		t.Fatal("expected error for non-system caller accessing system resource, got nil")
	}
}
