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

// docVisibilityPredicate is the knowledge_documents row predicate for ctx.
// System callers get empty pred (no filter). Empty user sees collection-only.
func docVisibilityPredicate(ctx context.Context, argIdx int) (pred string, args []any) {
	if workspace.IsSystem(ctx) {
		return "", nil
	}
	uid := viewerUserID(ctx)
	if uid == "" {
		return `COALESCE(NULLIF(visibility, ''), 'collection') = 'collection'`, nil
	}
	return fmt.Sprintf(`(COALESCE(NULLIF(visibility, ''), 'collection') = 'collection' OR owner_user_id = $%d)`, argIdx), []any{uid}
}

func visibleDocIDsSubquery(ctx context.Context, collectionPH string, argIdx int) (sql string, args []any) {
	pred, args := docVisibilityPredicate(ctx, argIdx)
	if pred == "" {
		return "", nil
	}
	return fmt.Sprintf(`SELECT id FROM knowledge_documents WHERE collection_id = %s AND %s`, collectionPH, pred), args
}

// docChunkVisibilityClause filters chunks whose parent document is visible to ctx.
func docChunkVisibilityClause(ctx context.Context, collectionPH string, argIdx int) (clause string, args []any) {
	sub, args := visibleDocIDsSubquery(ctx, collectionPH, argIdx)
	if sub == "" {
		return "", nil
	}
	return "AND doc_id IN (" + sub + ")", args
}

// docRowVisibilityClause filters rows of knowledge_documents for list/tree queries.
func docRowVisibilityClause(ctx context.Context, argIdx int) (clause string, args []any) {
	pred, args := docVisibilityPredicate(ctx, argIdx)
	if pred == "" {
		return "", nil
	}
	return "AND " + pred, args
}

// docBothEndpointsVisibleClause requires both link endpoints to be visible.
// Same $n is reused in both IN subqueries.
func docBothEndpointsVisibleClause(ctx context.Context, collectionPH string, argIdx int) (clause string, args []any) {
	sub, args := visibleDocIDsSubquery(ctx, collectionPH, argIdx)
	if sub == "" {
		return "", nil
	}
	return fmt.Sprintf(`AND doc_id IN (%s) AND target_doc_id IN (%s)`, sub, sub), args
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
