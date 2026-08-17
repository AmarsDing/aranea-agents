package data

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/auth"
)

var _ bizknowledge.DocumentACLStore = (*knowledgeRepo)(nil)

func viewerUserID(ctx context.Context) string {
	if a, ok := auth.FromContext(ctx); ok && a != nil && a.UserID > 0 {
		return strconv.FormatInt(a.UserID, 10)
	}
	return ""
}

// docChunkVisibilityClause filters chunks whose parent document is visible to ctx.
// System callers see all documents (cron/index). Empty user id sees collection-visible only.
func docChunkVisibilityClause(ctx context.Context, collectionPH string, argIdx int) (clause string, args []any) {
	if workspace.IsSystem(ctx) {
		return "", nil
	}
	uid := viewerUserID(ctx)
	if uid == "" {
		return fmt.Sprintf(`AND doc_id IN (SELECT id FROM knowledge_documents WHERE collection_id = %s AND COALESCE(NULLIF(visibility, ''), 'collection') = 'collection')`, collectionPH), nil
	}
	return fmt.Sprintf(`AND doc_id IN (SELECT id FROM knowledge_documents WHERE collection_id = %s AND (COALESCE(NULLIF(visibility, ''), 'collection') = 'collection' OR owner_user_id = $%d))`, collectionPH, argIdx), []any{uid}
}

// docRowVisibilityClause filters rows of knowledge_documents for list/tree queries.
func docRowVisibilityClause(ctx context.Context, argIdx int) (clause string, args []any) {
	if workspace.IsSystem(ctx) {
		return "", nil
	}
	uid := viewerUserID(ctx)
	if uid == "" {
		return `AND COALESCE(NULLIF(visibility, ''), 'collection') = 'collection'`, nil
	}
	return fmt.Sprintf(`AND (COALESCE(NULLIF(visibility, ''), 'collection') = 'collection' OR owner_user_id = $%d)`, argIdx), []any{uid}
}

func (r *knowledgeRepo) UpdateDocumentVisibility(ctx context.Context, id, visibility, ownerUserID string) error {
	visibility = strings.TrimSpace(visibility)
	if visibility == "" {
		visibility = "collection"
	}
	_, err := r.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_documents SET visibility = $2, owner_user_id = $3, updated_at = NOW() WHERE id = $1`,
		id, visibility, strings.TrimSpace(ownerUserID))
	return entErrToBizErr(err, "KNOWLEDGE")
}
