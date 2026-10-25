package data

import (
	"context"
	"database/sql"
	"fmt"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
)

type knowledgeTxKey struct{}

func governanceCollectionPredicate(ctx context.Context, argIdx int, mutate bool) (string, []any) {
	if workspace.IsSystem(ctx) {
		return "", nil
	}
	scope := fmt.Sprintf("workspace = $%d", argIdx)
	if !mutate {
		scope = "(" + scope + " OR workspace = '')"
	}
	return "collection_id IN (SELECT id FROM knowledge_collections WHERE " + scope + ")", []any{workspace.IDFromContext(ctx)}
}

type knowledgeSQL interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r *knowledgeRepo) knowledgeDB(ctx context.Context) knowledgeSQL {
	if tx, ok := ctx.Value(knowledgeTxKey{}).(*sql.Tx); ok {
		return tx
	}
	return r.data.Postgres()
}

func (r *knowledgeRepo) knowledgeExecInTx(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	if tx, ok := ctx.Value(knowledgeTxKey{}).(*sql.Tx); ok {
		return fn(ctx, tx)
	}
	return r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return fn(context.WithValue(ctx, knowledgeTxKey{}, tx), tx)
	})
}

// ResolveGovernanceAtomically locks the proposal before any content mutation.
// Stability: Internal. Derived indexes are replayed after commit by the usecase.
func (r *knowledgeRepo) ResolveGovernanceAtomically(ctx context.Context, id int64, apply func(context.Context, bizknowledge.GovernanceProposalView) (string, error)) error {
	return r.knowledgeExecInTx(ctx, func(txCtx context.Context, tx *sql.Tx) error {
		var collectionID, ws, root string
		err := tx.QueryRowContext(txCtx, "SELECT p.collection_id, c.workspace, c.root_path "+
			"FROM knowledge_governance_proposal p JOIN knowledge_collections c ON c.id = p.collection_id "+
			"WHERE p.id = $1 FOR UPDATE OF p", id).Scan(&collectionID, &ws, &root)
		if err != nil {
			return entErrToBizErr(err, "KNOWLEDGE")
		}
		if !workspace.IsSystem(ctx) {
			if err := workspace.AssertWorkspaceMutate(workspace.IDFromContext(ctx), ws); err != nil {
				return apierror.NotFound(apierror.DomainKnowledge, "proposal not found")
			}
		}
		if root != "" {
			return apierror.BadRequest(apierror.DomainKnowledge, "governance disposal requires a managed knowledge library")
		}
		view, err := r.GetGovernanceProposal(txCtx, id)
		if err != nil {
			return err
		}
		status, err := apply(txCtx, view)
		if err != nil {
			return err
		}
		return r.ResolveGovernanceProposal(txCtx, id, status)
	})
}
