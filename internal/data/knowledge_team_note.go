package data

import (
	"context"
	"database/sql"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
)

var _ bizknowledge.TeamContentStore = (*knowledgeRepo)(nil)

func (r *knowledgeRepo) ReplaceTeamDocumentContent(ctx context.Context, id, expectedHash, content string) (bool, error) {
	applied := false
	err := r.knowledgeExecInTx(ctx, func(txCtx context.Context, tx *sql.Tx) error {
		var current, collectionID, ws, root, status string
		var oldCount int
		err := tx.QueryRowContext(txCtx, "SELECT d.content_text, d.collection_id, c.workspace, c.root_path, d.status, d.chunk_count "+
			"FROM knowledge_documents d JOIN knowledge_collections c ON c.id = d.collection_id "+
			"WHERE d.id = $1 FOR UPDATE OF d", id).Scan(&current, &collectionID, &ws, &root, &status, &oldCount)
		if err != nil {
			return entErrToBizErr(err, "KNOWLEDGE")
		}
		if !workspace.IsSystem(ctx) {
			if err := workspace.AssertWorkspaceMutate(workspace.IDFromContext(ctx), ws); err != nil {
				return apierror.NotFound(apierror.DomainKnowledge, "document not found")
			}
		}
		if root != "" {
			return apierror.BadRequest(apierror.DomainKnowledge, "managed note requires a team library")
		}
		if bizknowledge.HashContent(current) != expectedHash {
			return nil
		}
		if err := tx.QueryRowContext(txCtx, "SELECT COUNT(*) FROM knowledge_chunks WHERE doc_id = $1 AND chunk_role = 'child'", id).Scan(&oldCount); err != nil {
			return entErrToBizErr(err, "KNOWLEDGE")
		}
		if _, err := tx.ExecContext(txCtx, "DELETE FROM knowledge_chunks WHERE doc_id = $1", id); err != nil {
			return entErrToBizErr(err, "KNOWLEDGE")
		}
		if _, err := tx.ExecContext(txCtx, "UPDATE knowledge_documents SET content_text = $2, content_hash = $3, "+
			"organized = TRUE, status = 'pending', chunk_count = 0, error_message = '', stale_at = NULL, "+
			"updated_at = NOW() WHERE id = $1", id, content, bizknowledge.HashContent(content)); err != nil {
			return entErrToBizErr(err, "KNOWLEDGE")
		}
		docDelta := 0
		if status == string(bizknowledge.DocumentStateIndexed) || oldCount > 0 {
			docDelta = 1
		}
		if _, err := tx.ExecContext(txCtx, "UPDATE knowledge_collections SET "+
			"document_count = GREATEST(document_count - $2, 0), "+
			"chunk_count = GREATEST(chunk_count - $3, 0), updated_at = NOW() WHERE id = $1",
			collectionID, docDelta, oldCount); err != nil {
			return entErrToBizErr(err, "KNOWLEDGE")
		}
		applied = true
		return nil
	})
	return applied, err
}
