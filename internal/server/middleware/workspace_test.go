package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/internal/server/middleware"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
)

func TestHeaderWorkspaceID(t *testing.T) {
	if middleware.HeaderWorkspaceID != "X-Workspace-ID" {
		t.Fatalf("expected X-Workspace-ID, got %s", middleware.HeaderWorkspaceID)
	}
}

func TestQueryWorkspaceID(t *testing.T) {
	if middleware.QueryWorkspaceID != "workspace_id" {
		t.Fatalf("expected workspace_id, got %s", middleware.QueryWorkspaceID)
	}
}

func TestAssertWorkspace_SystemBypasses(t *testing.T) {
	if err := middleware.AssertWorkspace(workspace.SystemWorkspaceID, "other-ws"); err != nil {
		t.Fatalf("system workspace should bypass, got %v", err)
	}
}

func TestAssertWorkspace_SameWorkspace(t *testing.T) {
	if err := middleware.AssertWorkspace("ws-1", "ws-1"); err != nil {
		t.Fatalf("same workspace should pass, got %v", err)
	}
}

// TestAssertWorkspace_EmptyResourceTreatedAsDefault verifies P1-2 semantics:
// empty resourceWorkspaceID is treated as DefaultWorkspaceID, NOT as "allow any
// caller". This closes the legacy-data IDOR hole where resources without
// WorkspaceID were accessible to any tenant.
func TestAssertWorkspace_EmptyResourceTreatedAsDefault(t *testing.T) {
	// Caller from non-default workspace → blocked (was allowed pre-P1-2).
	err := middleware.AssertWorkspace("ws-1", "")
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

	// Caller from default workspace → allowed (legacy data belongs to default).
	if err := middleware.AssertWorkspace(workspace.DefaultWorkspaceID, ""); err != nil {
		t.Fatalf("default workspace caller should access empty (legacy) resource, got %v", err)
	}
}

func TestAssertWorkspace_DifferentWorkspace(t *testing.T) {
	err := middleware.AssertWorkspace("ws-1", "ws-2")
	if err == nil {
		t.Fatal("different workspace should return error")
	}
	ae, ok := apierror.From(err)
	if !ok {
		t.Fatal("expected *apierror.Error")
	}
	if ae.Code != apierror.CodeForbidden {
		t.Fatalf("expected FORBIDDEN code, got %s", ae.Code)
	}
}

func TestAssertWorkspace_SystemWorkspaceID(t *testing.T) {
	if workspace.SystemWorkspaceID == "" {
		t.Fatal("SystemWorkspaceID should not be empty")
	}
}

func TestWorkspaceFilter_SetsDefaultWorkspace(t *testing.T) {
	var gotWS string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := workspace.FromContext(r.Context())
		if ok {
			gotWS = id
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.WorkspaceFilter()(next)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if gotWS != workspace.DefaultWorkspaceID {
		t.Fatalf("expected default workspace %q, got %q", workspace.DefaultWorkspaceID, gotWS)
	}
}

// TestWorkspaceFilter_RejectsForgedHeaderForNonAdmin verifies B-01:
// non-admin principals cannot override JWT workspace via X-Workspace-ID.
func TestWorkspaceFilter_RejectsForgedHeaderForNonAdmin(t *testing.T) {
	handler := middleware.WorkspaceFilter()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example/v1/x", nil)
	req.Header.Set(middleware.HeaderWorkspaceID, "victim-ws")
	req = req.WithContext(auth.NewContext(req.Context(), &auth.Auth{
		UserID:      42,
		Access:      "user",
		WorkspaceID: "default",
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for forged workspace, got %d", rr.Code)
	}
}

// TestWorkspaceFilter_AdminCannotForgeWorkspace verifies B-01 Option A:
// admins are also bound to JWT workspace_id; header forge is rejected.
func TestWorkspaceFilter_AdminCannotForgeWorkspace(t *testing.T) {
	handler := middleware.WorkspaceFilter()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example/v1/x", nil)
	req.Header.Set(middleware.HeaderWorkspaceID, "ops-ws")
	req = req.WithContext(auth.NewContext(req.Context(), &auth.Auth{
		UserID:      1,
		Access:      "admin",
		WorkspaceID: "default",
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for admin forged workspace, got %d", rr.Code)
	}
}

// TestWorkspaceFilter_AdminUsesJWTWorkspace verifies admin with matching
// (or absent) header uses JWT workspace.
func TestWorkspaceFilter_AdminUsesJWTWorkspace(t *testing.T) {
	var gotWS string
	handler := middleware.WorkspaceFilter()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotWS, _ = workspace.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example/v1/x", nil)
	req = req.WithContext(auth.NewContext(req.Context(), &auth.Auth{
		UserID:      1,
		Access:      "admin",
		WorkspaceID: "ws-tenant-a",
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotWS != "ws-tenant-a" {
		t.Fatalf("expected ws-tenant-a, got %q", gotWS)
	}
}

// TestWorkspaceFilter_IgnoresUnauthenticatedHeader verifies public paths
// cannot forge a workspace (always default).
func TestWorkspaceFilter_IgnoresUnauthenticatedHeader(t *testing.T) {
	var gotWS string
	handler := middleware.WorkspaceFilter()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotWS, _ = workspace.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example/v1/x", nil)
	req.Header.Set(middleware.HeaderWorkspaceID, "forged-ws")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if gotWS != workspace.DefaultWorkspaceID {
		t.Fatalf("expected default, got %q", gotWS)
	}
}
