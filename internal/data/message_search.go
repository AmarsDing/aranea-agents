package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

func (r *sessionRepo) SearchMessages(ctx context.Context, q biz.MessageSearchQuery) (biz.MessageSearchResult, error) {
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return biz.MessageSearchResult{}, apierror.Internal("SESSION", "database not configured")
	}
	if strings.TrimSpace(q.SessionID) == "" {
		return biz.MessageSearchResult{}, apierror.BadRequest("SESSION", "session_id is required")
	}
	keyword := strings.TrimSpace(q.Keyword)
	if keyword == "" {
		return biz.MessageSearchResult{}, apierror.BadRequest("SESSION", "keyword is required")
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

	d := r.data.Dialect()
	if ftsAvailable(ctx, db, d) {
		return r.searchMessagesFTS(ctx, db, d, q, keyword, limit, offset)
	}
	return r.searchMessagesLike(ctx, db, d, q, keyword, limit, offset)
}

// ftsAvailable checks whether full-text search is available for the dialect.
// SQLite: checks for messages_fts virtual table.
// Postgres: checks for tsv column on messages table.
func ftsAvailable(ctx context.Context, db execer, d Dialect) bool {
	if d.IsPostgres() {
		// Check for tsv column on messages table via information_schema.
		rows, err := db.QueryContext(ctx,
			`SELECT column_name FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'messages' AND column_name = 'tsv' LIMIT 1`)
		if err != nil {
			return false
		}
		defer rows.Close()
		return rows.Next()
	}
	return tableExists(ctx, db, d, "messages_fts")
}

// tableExists checks whether a table exists in the database.
// SQLite: sqlite_master WHERE type = 'table' AND name = ?
// Postgres: information_schema.tables WHERE table_schema = 'public' AND table_name = $1
func tableExists(ctx context.Context, db execer, d Dialect, name string) bool {
	q, args := d.TableExistsQuery(name)
	var n string
	err := queryRowScan(ctx, db, q, args, &n)
	return err == nil && n != ""
}

func (r *sessionRepo) searchMessagesFTS(ctx context.Context, db execer, d Dialect, q biz.MessageSearchQuery, keyword string, limit, offset int) (biz.MessageSearchResult, error) {
	if d.IsPostgres() {
		return r.searchMessagesPostgresFTS(ctx, db, q, keyword, limit, offset)
	}
	return r.searchMessagesSQLiteFTS(ctx, db, q, keyword, limit, offset)
}

// searchMessagesSQLiteFTS uses FTS5 virtual table for full-text search.
func (r *sessionRepo) searchMessagesSQLiteFTS(ctx context.Context, db execer, q biz.MessageSearchQuery, keyword string, limit, offset int) (biz.MessageSearchResult, error) {
	match := ftsMatchQuery(keyword)
	args := []any{match}
	where := "messages_fts MATCH ?"
	if sid := strings.TrimSpace(q.SessionID); sid != "" {
		where += " AND session_id = ?"
		args = append(args, sid)
	}
	countSQL := fmt.Sprintf(`SELECT COUNT(1) FROM messages_fts WHERE %s`, where)
	var total int
	if err := queryRowScan(ctx, db, countSQL, args, &total); err != nil {
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
		return r.searchMessagesLike(ctx, db, DialectSQLite, q, keyword, limit, offset)
	}
	defer rows.Close()
	return scanMessageSearchRows(rows, total)
}

// searchMessagesPostgresFTS uses tsvector + GIN index for full-text search.
// Uses websearch_to_tsquery for natural-language query syntax and ts_headline
// for highlight snippets.
func (r *sessionRepo) searchMessagesPostgresFTS(ctx context.Context, db execer, q biz.MessageSearchQuery, keyword string, limit, offset int) (biz.MessageSearchResult, error) {
	tsQuery := strings.TrimSpace(keyword)
	hasSessionID := strings.TrimSpace(q.SessionID) != ""

	// Build WHERE clause with Postgres $N placeholders.
	// $1 = tsquery, $2 = session_id (optional), $3 = limit, $4 = offset
	var where string
	var countArgs []any
	if hasSessionID {
		where = "tsv @@ websearch_to_tsquery('simple', $1) AND session_id = $2"
		countArgs = []any{tsQuery, q.SessionID}
	} else {
		where = "tsv @@ websearch_to_tsquery('simple', $1)"
		countArgs = []any{tsQuery}
	}
	countSQL := fmt.Sprintf(`SELECT COUNT(1) FROM messages WHERE %s`, where)
	var total int
	if err := queryRowScan(ctx, db, countSQL, countArgs, &total); err != nil {
		return biz.MessageSearchResult{}, err
	}

	// List query: append limit/offset args.
	listArgs := append([]any{}, countArgs...)
	listArgs = append(listArgs, limit, offset)
	limitIdx := len(countArgs) + 1
	offsetIdx := limitIdx + 1
	listSQL := fmt.Sprintf(`
SELECT id, session_id, role, content_markdown, created_at,
       ts_headline('simple', content_markdown, websearch_to_tsquery('simple', $1), 'StartSel=<mark>, StopSel=</mark>, MaxWords=32, MinWords=8') AS highlight
FROM messages
WHERE %s
ORDER BY ts_rank(tsv, websearch_to_tsquery('simple', $1)) DESC
LIMIT $%d OFFSET $%d`, where, limitIdx, offsetIdx)
	rows, err := db.QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return r.searchMessagesLike(ctx, db, DialectPostgres, q, keyword, limit, offset)
	}
	defer rows.Close()
	return scanMessageSearchRows(rows, total)
}

func (r *sessionRepo) searchMessagesLike(ctx context.Context, db execer, d Dialect, q biz.MessageSearchQuery, keyword string, limit, offset int) (biz.MessageSearchResult, error) {
	pattern := "%" + escapeLike(keyword) + "%"
	args := []any{pattern}
	where := "content_markdown LIKE ? ESCAPE '\\'"
	if sid := strings.TrimSpace(q.SessionID); sid != "" {
		where += " AND session_id = ?"
		args = append(args, sid)
	}
	// For Postgres, convert ? placeholders to $N.
	wherePG := d.RenumberPlaceholders(where)
	countSQL := fmt.Sprintf(`SELECT COUNT(1) FROM messages WHERE %s`, wherePG)
	var total int
	if err := queryRowScan(ctx, db, countSQL, args, &total); err != nil {
		return biz.MessageSearchResult{}, err
	}
	// List query: append LIMIT/OFFSET placeholders.
	listWhere := where + " LIMIT ? OFFSET ?"
	listSQL := fmt.Sprintf(`
SELECT id, session_id, role, content_markdown, created_at, '' AS highlight
FROM messages WHERE %s
ORDER BY created_at DESC`, listWhere)
	listSQL = d.RenumberPlaceholders(listSQL)
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
