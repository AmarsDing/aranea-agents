package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func (r *sessionRepo) SearchMessages(ctx context.Context, q biz.MessageSearchQuery) (biz.MessageSearchResult, error) {
	db := r.data.RawDB()
	if db == nil {
		return biz.MessageSearchResult{}, kerrors.InternalServer("SESSION", "database not configured")
	}
	if strings.TrimSpace(q.SessionID) == "" {
		return biz.MessageSearchResult{}, kerrors.BadRequest("SESSION", "session_id is required")
	}
	keyword := strings.TrimSpace(q.Keyword)
	if keyword == "" {
		return biz.MessageSearchResult{}, kerrors.BadRequest("SESSION", "keyword is required")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}

	if tableExists(ctx, db, "messages_fts") {
		return r.searchMessagesFTS(ctx, db, q, keyword, limit, offset)
	}
	return r.searchMessagesLike(ctx, db, q, keyword, limit, offset)
}

func tableExists(ctx context.Context, db *sql.DB, name string) bool {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM sqlite_master WHERE type IN ('table','view') AND name = ?`, name).Scan(&n)
	return err == nil && n > 0
}

func (r *sessionRepo) searchMessagesFTS(ctx context.Context, db *sql.DB, q biz.MessageSearchQuery, keyword string, limit, offset int) (biz.MessageSearchResult, error) {
	match := ftsMatchQuery(keyword)
	args := []any{match}
	where := "messages_fts MATCH ?"
	if sid := strings.TrimSpace(q.SessionID); sid != "" {
		where += " AND session_id = ?"
		args = append(args, sid)
	}
	countSQL := fmt.Sprintf(`SELECT COUNT(1) FROM messages_fts WHERE %s`, where)
	var total int
	if err := db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return biz.MessageSearchResult{}, err
	}
	listSQL := fmt.Sprintf(`
SELECT m.id, m.session_id, m.role, m.content_markdown, m.created_at,
       snippet(messages_fts, 2, '<mark>', '</mark>', '…', 32) AS highlight
FROM messages_fts
JOIN messages m ON m.id = messages_fts.message_id
WHERE %s
ORDER BY bm25(messages_fts)
LIMIT ? OFFSET ?`, where)
	args = append(args, limit, offset)
	rows, err := db.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return r.searchMessagesLike(ctx, db, q, keyword, limit, offset)
	}
	defer rows.Close()
	return scanMessageSearchRows(rows, total)
}

func (r *sessionRepo) searchMessagesLike(ctx context.Context, db *sql.DB, q biz.MessageSearchQuery, keyword string, limit, offset int) (biz.MessageSearchResult, error) {
	pattern := "%" + escapeLike(keyword) + "%"
	args := []any{pattern}
	where := "content_markdown LIKE ? ESCAPE '\\'"
	if sid := strings.TrimSpace(q.SessionID); sid != "" {
		where += " AND session_id = ?"
		args = append(args, sid)
	}
	countSQL := fmt.Sprintf(`SELECT COUNT(1) FROM messages WHERE %s`, where)
	var total int
	if err := db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return biz.MessageSearchResult{}, err
	}
	listSQL := fmt.Sprintf(`
SELECT id, session_id, role, content_markdown, created_at, '' AS highlight
FROM messages WHERE %s
ORDER BY created_at DESC
LIMIT ? OFFSET ?`, where)
	args = append(args, limit, offset)
	rows, err := db.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return biz.MessageSearchResult{}, err
	}
	defer rows.Close()
	return scanMessageSearchRows(rows, total)
}

func scanMessageSearchRows(rows *sql.Rows, total int) (biz.MessageSearchResult, error) {
	items := make([]biz.MessageSearchHit, 0)
	for rows.Next() {
		var hit biz.MessageSearchHit
		if err := rows.Scan(&hit.ID, &hit.SessionID, &hit.Role, &hit.ContentMarkdown, &hit.CreatedAt, &hit.Highlight); err != nil {
			return biz.MessageSearchResult{}, err
		}
		items = append(items, hit)
	}
	return biz.MessageSearchResult{Items: items, Total: total}, rows.Err()
}

func ftsMatchQuery(keyword string) string {
	parts := strings.Fields(keyword)
	if len(parts) == 0 {
		return keyword
	}
	for i, p := range parts {
		p = strings.ReplaceAll(p, `"`, "")
		parts[i] = `"` + p + `"`
	}
	return strings.Join(parts, " ")
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
