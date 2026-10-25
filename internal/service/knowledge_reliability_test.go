package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
)

func knowledgeViewer(ws, access string, id int64) context.Context {
	return auth.NewContext(workspace.WithContext(context.Background(), ws), &auth.Auth{UserID: id, Access: access, WorkspaceID: ws})
}

func TestKnowledgeGovernanceRequiresPrivilegedIdentity(t *testing.T) {
	svc, _ := newGovernanceService(t, &stubGovEntityRepo{}, nil)
	svc.uc.SetCurateRepo(&stubGovCurateRepo{views: []bizknowledge.GovernanceProposalView{{
		ID: 1, CollectionID: "private-library", Status: bizknowledge.ProposalStatusPending,
	}}})
	for _, ctx := range []context.Context{context.Background(), knowledgeViewer("ws-a", "member", 7)} {
		if _, err := svc.ListGovernanceProposals(ctx, &v1.ListGovernanceProposalsRequest{}); err == nil {
			t.Fatal("unprivileged caller must not read global governance proposals")
		}
		if _, err := svc.ResolveGovernanceProposal(ctx, &v1.ResolveGovernanceProposalRequest{Id: 1, Decision: "rejected"}); err == nil {
			t.Fatal("unprivileged caller must not dispose proposals")
		}
	}
}

func TestKnowledgeGovernanceResolveChecksTargetWorkspace(t *testing.T) {
	for _, ws := range []string{"ws-b", ""} {
		svc, repo := newGovernanceService(t, &stubGovEntityRepo{}, nil)
		curate := &stubGovCurateRepo{views: []bizknowledge.GovernanceProposalView{{
			ID: 1, CollectionID: "other-library", Status: bizknowledge.ProposalStatusPending,
		}}}
		svc.uc.SetCurateRepo(curate)
		if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "other-library", Name: "other", Workspace: ws}); err != nil {
			t.Fatal(err)
		}
		_, err := svc.ResolveGovernanceProposal(knowledgeViewer("ws-a", "admin", 7), &v1.ResolveGovernanceProposalRequest{Id: 1, Decision: "rejected"})
		if !apierror.IsCode(err, apierror.CodeNotFound) || curate.resolvedID != 0 {
			t.Fatalf("foreign/shared proposal must remain untouched: err=%v resolved=%d", err, curate.resolvedID)
		}
	}
}

func TestKnowledgeSharedLibraryRejectsDirectoryAndNoteWrites(t *testing.T) {
	svc, repo := newGovernanceService(t, &stubGovEntityRepo{}, nil)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "shared", Name: "shared", RootPath: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	ctx := knowledgeViewer("ws-a", "admin", 7)
	if _, err := svc.CreateVaultDir(ctx, &v1.CreateVaultDirRequest{CollectionId: "shared", DirPath: "injected"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("shared directory write: %v", err)
	}
	if _, err := svc.CreateVaultDocument(ctx, &v1.CreateVaultDocumentRequest{CollectionId: "shared", RelPath: "injected.md"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("shared note write: %v", err)
	}
}

func TestKnowledgeGetDocumentEnforcesOwnerAndWorkspace(t *testing.T) {
	svc, repo := newGovernanceService(t, &stubGovEntityRepo{}, nil)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c", Name: "notes", Workspace: "ws-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(context.Background(), biz.KnowledgeDocument{ID: "d", CollectionID: "c", Source: "draft.md",
		Visibility: bizknowledge.DocVisibilityPrivate, OwnerUserID: "7"}); err != nil {
		t.Fatal(err)
	}
	for _, ctx := range []context.Context{knowledgeViewer("ws-a", "member", 8), knowledgeViewer("ws-b", "member", 7)} {
		if _, err := svc.GetDocument(ctx, &v1.GetDocumentRequest{Id: "d"}); !apierror.IsCode(err, apierror.CodeNotFound) {
			t.Fatalf("unreadable metadata must be hidden: %v", err)
		}
	}
	doc, err := svc.GetDocument(knowledgeViewer("ws-a", "member", 7), &v1.GetDocumentRequest{Id: "d"})
	if err != nil || doc.GetId() != "d" || doc.GetOwnerUserId() != "7" {
		t.Fatalf("owner should resolve the document: doc=%v err=%v", doc, err)
	}
}

func TestKnowledgePendingApplyRejectsMalformedSelection(t *testing.T) {
	svc, repo := newGovernanceService(t, &stubGovEntityRepo{}, nil)
	_, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{
		ID: "own", Name: "own", Workspace: workspace.DefaultWorkspaceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, payload := range []string{`{"fact_ids":[7]}`, `{"fact_id":["one"]}`, `{"fact_ids":["one"]}{}`, `{`} {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
		req = req.WithContext(knowledgeViewer(workspace.DefaultWorkspaceID, "member", 7))
		w := httptest.NewRecorder()
		svc.ServeWriteBackPendingApply(w, req, "own")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("malformed selection must not become apply-all: status=%d", w.Code)
		}
	}
}

func TestKnowledgeVisibilityReadHidesPrivateDocument(t *testing.T) {
	svc, repo := newGovernanceService(t, &stubGovEntityRepo{}, nil)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c", Name: "c", Workspace: "ws-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(context.Background(), biz.KnowledgeDocument{ID: "private", CollectionID: "c", Source: "draft.md",
		Visibility: bizknowledge.DocVisibilityPrivate, OwnerUserID: "7"}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(knowledgeViewer("ws-a", "member", 8))
	w := httptest.NewRecorder()
	svc.ServeDocumentVisibility(w, req, "private")
	if w.Code != http.StatusNotFound {
		t.Fatalf("private ACL metadata must be hidden: %d", w.Code)
	}
}
