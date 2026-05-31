package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/internal/server/middleware"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
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

func TestAssertWorkspace_EmptyResource(t *testing.T) {
	if err := middleware.AssertWorkspace("ws-1", ""); err != nil {
		t.Fatalf("empty resource workspace should pass, got %v", err)
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
