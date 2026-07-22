package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/internal/artifact"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/artifactfs"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// stubRevealLauncher replaces the OS file-manager launcher with a recorder for
// the duration of the test (revealLauncher is a package-level var).
func stubRevealLauncher(t *testing.T) *[]string {
	t.Helper()
	got := []string{}
	orig := revealLauncher
	revealLauncher = func(abs string) error {
		got = append(got, abs)
		return nil
	}
	t.Cleanup(func() { revealLauncher = orig })
	return &got
}

// newRevealService builds an ArtifactService backed by a real FS repo rooted at
// t.TempDir() so ResolveAbsPath returns genuine on-disk paths.
func newRevealService(t *testing.T, lookup sessionWorkspaceLookup) *ArtifactService {
	t.Helper()
	repo := artifactfs.NewFSArtifactRepoAt(t.TempDir(), loggateway.NewNoop())
	uc := biz.NewArtifactUsecase(repo, loggateway.NewNoop())
	return NewArtifactService(uc, artifact.NewSigner(loggateway.NewNoop()), lookup)
}

// TestArtifactService_RevealLocal verifies the M27 Phase 5 reveal flow:
// auth (workspace owns artifact) -> abs path resolution -> root containment ->
// OS file-manager launch.
func TestArtifactService_RevealLocal(t *testing.T) {
	lookup := &fakeSessionLookup{sessions: map[string]biz.Session{
		"sess-ok": {ID: "sess-ok", WorkspaceID: "ws-ok"},
	}}

	t.Run("ok launches file manager at artifact path", func(t *testing.T) {
		got := stubRevealLauncher(t)
		svc := newRevealService(t, lookup)
		ctx := workspace.WithContext(context.Background(), "ws-ok")
		id := uploadArtifactWithMime(t, svc, ctx, "sess-ok", "report.md", "text/markdown")

		abs, err := svc.revealLocal(ctx, id)
		if err != nil {
			t.Fatalf("revealLocal: %v", err)
		}
		if len(*got) != 1 || (*got)[0] != abs {
			t.Fatalf("launcher calls = %v, want [%s]", *got, abs)
		}
		if _, statErr := os.Stat(abs); statErr != nil {
			t.Fatalf("resolved path not on disk: %v", statErr)
		}
	})

	t.Run("not found", func(t *testing.T) {
		stubRevealLauncher(t)
		svc := newRevealService(t, lookup)
		ctx := workspace.WithContext(context.Background(), "ws-ok")
		if _, err := svc.revealLocal(ctx, "missing-id"); !apierror.IsCode(err, apierror.CodeNotFound) {
			t.Fatalf("err = %v, want NotFound", err)
		}
	})

	t.Run("empty id rejected", func(t *testing.T) {
		stubRevealLauncher(t)
		svc := newRevealService(t, lookup)
		ctx := workspace.WithContext(context.Background(), "ws-ok")
		if _, err := svc.revealLocal(ctx, "  "); !apierror.IsCode(err, apierror.CodeBadRequest) {
			t.Fatalf("err = %v, want BadRequest", err)
		}
	})

	t.Run("cross-workspace forbidden and launcher not invoked", func(t *testing.T) {
		got := stubRevealLauncher(t)
		svc := newRevealService(t, lookup)
		ownerCtx := workspace.WithContext(context.Background(), "ws-ok")
		id := uploadArtifactWithMime(t, svc, ownerCtx, "sess-ok", "report.md", "text/markdown")

		otherCtx := workspace.WithContext(context.Background(), "ws-other")
		if _, err := svc.revealLocal(otherCtx, id); !apierror.IsCode(err, apierror.CodeForbidden) {
			t.Fatalf("err = %v, want Forbidden", err)
		}
		if len(*got) != 0 {
			t.Fatalf("launcher must not run on forbidden reveal, got %v", *got)
		}
	})

	t.Run("non-local backend rejected", func(t *testing.T) {
		stubRevealLauncher(t)
		// mem repo does not implement absPathResolver -> ResolveAbsPath returns "".
		svc := newArtifactServiceWithLookup(lookup)
		ctx := workspace.WithContext(context.Background(), "ws-ok")
		id := uploadArtifactWithMime(t, svc, ctx, "sess-ok", "report.md", "text/markdown")
		if _, err := svc.revealLocal(ctx, id); !apierror.IsCode(err, apierror.CodeBadRequest) {
			t.Fatalf("err = %v, want BadRequest", err)
		}
	})
}

// TestArtifactService_ServeRevealLocal_HTTPContract verifies the HTTP layer of
// POST /v1/system/reveal (status codes + JSON body shape).
func TestArtifactService_ServeRevealLocal_HTTPContract(t *testing.T) {
	lookup := &fakeSessionLookup{sessions: map[string]biz.Session{
		"sess-ok": {ID: "sess-ok", WorkspaceID: "ws-ok"},
	}}
	got := stubRevealLauncher(t)
	svc := newRevealService(t, lookup)
	ctx := workspace.WithContext(context.Background(), "ws-ok")
	id := uploadArtifactWithMime(t, svc, ctx, "sess-ok", "report.md", "text/markdown")

	t.Run("200 with path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/system/reveal", strings.NewReader(`{"artifact_id":"`+id+`"}`))
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		svc.ServeRevealLocal(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["path"] == "" || len(*got) == 0 {
			t.Fatalf("body = %v launcher = %v", body, *got)
		}
	})

	t.Run("400 on empty id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/system/reveal", strings.NewReader(`{}`))
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		svc.ServeRevealLocal(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("404 on unknown id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/system/reveal", strings.NewReader(`{"artifact_id":"nope"}`))
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		svc.ServeRevealLocal(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})
}

// TestRevealPathWithinRoot verifies the filepath.Rel anti-traversal guard
// (M27 Phase 5 security constraint: reveal target must stay under artifact root).
func TestRevealPathWithinRoot(t *testing.T) {
	root := t.TempDir()
	under := filepath.Join(root, "sess", "a-v1.bin")
	cases := []struct {
		name   string
		root   string
		target string
		want   bool
	}{
		{"under root", root, under, true},
		{"root itself", root, root, true},
		{"escape via dotdot", root, filepath.Join(root, "..", "evil"), false},
		{"unrelated sibling", root, filepath.Join(filepath.Dir(root), "other", "x"), false},
		{"empty root", "", under, false},
		{"empty target", root, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := revealPathWithinRoot(tc.root, tc.target); got != tc.want {
				t.Fatalf("revealPathWithinRoot(%q,%q) = %v, want %v", tc.root, tc.target, got, tc.want)
			}
		})
	}
}
