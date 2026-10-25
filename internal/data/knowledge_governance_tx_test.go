package data

import (
	"context"
	"strings"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"

	"github.com/stretchr/testify/require"
)

func insertGovernanceTestProposal(t *testing.T, r *knowledgeRepo, collectionID, kind, payload string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, r.data.Postgres().QueryRowContext(context.Background(),
		"INSERT INTO knowledge_governance_proposal(collection_id,kind,payload,risk,status) VALUES($1,$2,$3::jsonb,'high','pending') RETURNING id",
		collectionID, kind, payload).Scan(&id))
	return id
}

func TestKnowledgeGovernance_TenantScopeAndSharedWriteBoundary(t *testing.T) {
	r := setupCurateRepo(t)
	sys := workspace.WithSystemWorkspace(context.Background())
	for _, c := range []bizknowledge.Collection{
		{ID: "own", Name: "own", Workspace: "ws-a"},
		{ID: "foreign", Name: "foreign", Workspace: "ws-b"},
		{ID: "shared", Name: "shared"},
	} {
		_, err := r.CreateCollection(sys, c)
		require.NoError(t, err)
		insertGovernanceTestProposal(t, r, c.ID, bizknowledge.ProposalKindOrphan, "{}")
	}
	ctx := workspace.WithContext(context.Background(), "ws-a")
	views, total, err := r.ListGovernanceProposals(ctx, "", "", 50, 0)
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, views, 2)
	for _, v := range views {
		require.NotEqual(t, "foreign", v.CollectionID)
	}
	var foreign, shared int64
	require.NoError(t, r.data.Postgres().QueryRowContext(sys, "SELECT id FROM knowledge_governance_proposal WHERE collection_id='foreign'").Scan(&foreign))
	require.NoError(t, r.data.Postgres().QueryRowContext(sys, "SELECT id FROM knowledge_governance_proposal WHERE collection_id='shared'").Scan(&shared))
	_, err = r.GetGovernanceProposal(ctx, foreign)
	require.True(t, apierror.IsCode(err, apierror.CodeNotFound))
	for _, id := range []int64{foreign, shared} {
		require.True(t, apierror.IsCode(r.ResolveGovernanceProposal(ctx, id, bizknowledge.ProposalStatusRejected), apierror.CodeNotFound))
		called := false
		err = r.ResolveGovernanceAtomically(ctx, id, func(context.Context, bizknowledge.GovernanceProposalView) (string, error) {
			called = true
			return bizknowledge.ProposalStatusRejected, nil
		})
		require.True(t, apierror.IsCode(err, apierror.CodeNotFound))
		require.False(t, called)
	}
}

func TestKnowledgeGovernance_FactDisposalInvalidatesEvidenceAndRollsBackAtomically(t *testing.T) {
	for _, failTerminal := range []bool{false, true} {
		name := "commit"
		if failTerminal {
			name = "rollback"
		}
		t.Run(name, func(t *testing.T) {
			ctx, r, colID, docID := setupTeamNoteRepo(t)
			body := "# Entry\n\n## Old\n- fact_id: " + "`old`" + "\nOld rejected statement\n\n## New\n- fact_id: " + "`new`" + "\nNew accepted statement\n"
			require.NoError(t, r.UpdateDocumentContent(ctx, docID, body, true))
			payload := `{"doc_id":"` + docID + `","target_fact_id":"old","new_fact_id":"new"}`
			id := insertGovernanceTestProposal(t, r, colID, bizknowledge.ProposalKindConflict, payload)
			if failTerminal {
				_, err := r.data.Postgres().ExecContext(ctx, `CREATE FUNCTION reject_governance_terminal() RETURNS trigger LANGUAGE plpgsql AS $$
					BEGIN RAISE EXCEPTION 'injected terminal persistence failure'; END $$;
					CREATE TRIGGER reject_governance_terminal BEFORE UPDATE ON knowledge_governance_proposal
					FOR EACH ROW EXECUTE FUNCTION reject_governance_terminal()`)
				require.NoError(t, err)
			}
			u := bizknowledge.NewUsecaseFromRepo(r)
			u.SetCurateRepo(r)
			u.SetEvolutionRepos(r, r)
			err := u.ResolveGovernanceProposal(ctx, id, bizknowledge.ProposalDecisionKeepNew)
			doc, readErr := r.GetDocument(ctx, docID)
			require.NoError(t, readErr)
			view, readErr := r.GetGovernanceProposal(ctx, id)
			require.NoError(t, readErr)
			var chunks, versions int
			require.NoError(t, r.data.Postgres().QueryRowContext(ctx, "SELECT COUNT(*) FROM knowledge_chunks WHERE doc_id=$1", docID).Scan(&chunks))
			require.NoError(t, r.data.Postgres().QueryRowContext(ctx, "SELECT COUNT(*) FROM knowledge_fact_version WHERE doc_id=$1", docID).Scan(&versions))
			if failTerminal {
				require.Error(t, err)
				require.Equal(t, body, doc.ContentText)
				require.Equal(t, bizknowledge.ProposalStatusPending, view.Status)
				require.Equal(t, 1, chunks)
				require.Zero(t, versions)
				return
			}
			require.NoError(t, err)
			require.False(t, strings.Contains(doc.ContentText, "Old rejected statement"))
			require.Contains(t, doc.ContentText, "New accepted statement")
			require.Equal(t, bizknowledge.ProposalStatusApplied, view.Status)
			require.Equal(t, "pending", doc.Status)
			require.Zero(t, chunks, "rejected evidence must not remain searchable before repair")
			require.Equal(t, 1, versions)
			pending, err := r.ListDocumentsPendingReembed(ctx, colID, 100)
			require.NoError(t, err)
			require.NotEmpty(t, pending)
		})
	}
}
