package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/stepv2"
)

// globalMessageSearchRepo implements biz.GlobalMessageSearcher over steps_v2
// (M71). Replaces the removed messages_fts FTS5 search: kind IN (task, reply),
// content LIKE, started_at desc.
type globalMessageSearchRepo struct {
	data *Data
}

var _ biz.GlobalMessageSearcher = (*globalMessageSearchRepo)(nil)

// NewGlobalMessageSearchRepo creates the global message searcher.
func NewGlobalMessageSearchRepo(d *Data) biz.GlobalMessageSearcher {
	return &globalMessageSearchRepo{data: d}
}

// snippetRadius is the number of chars kept on each side of the first keyword
// occurrence when building the result snippet.
const snippetRadius = 100

// SearchGlobalMessages runs the cross-session keyword search scoped to one
// tenant workspace. steps_v2 has no workspace column and Ent cannot traverse
// step→session (no edge), so the workspace/agent filter is resolved by a raw
// SQL JOIN that returns the ordered matching step IDs (bounded by limit); the
// full rows are then loaded through Ent to keep time/JSON decoding identical
// to the rest of the data layer.
func (r *globalMessageSearchRepo) SearchGlobalMessages(ctx context.Context, keyword, agentID, workspaceID string, limit int) ([]biz.GlobalMessageHit, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" || limit <= 0 {
		return nil, nil
	}
	ids, err := r.matchStepIDs(ctx, keyword, agentID, workspaceID, limit)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	rows, err := r.data.RW().Read(ctx).StepV2.Query().
		Where(stepv2.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SESSION_SEARCH")
	}
	byID := make(map[string]*ent.StepV2, len(rows))
	for _, e := range rows {
		byID[e.ID] = e
	}
	out := make([]biz.GlobalMessageHit, 0, len(ids))
	for _, id := range ids {
		e, ok := byID[id]
		if !ok {
			continue
		}
		out = append(out, biz.GlobalMessageHit{
			ID:             id,
			SessionID:      e.SessionID,
			Kind:           e.Kind,
			AuthorAgentKey: e.AuthorAgentKey,
			Snippet:        buildSnippet(e.Content, keyword),
			StartedAt:      e.StartedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return out, nil
}

// matchStepIDs returns up to limit step IDs whose content matches the keyword,
// newest first, restricted to non-deleted sessions of the given workspace
// (empty workspaceID = no tenant filter, system callers only) and optionally
// to sessions of one agent.
func (r *globalMessageSearchRepo) matchStepIDs(ctx context.Context, keyword, agentID, workspaceID string, limit int) ([]string, error) {
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return nil, nil
	}
	query := `
SELECT st.id
FROM steps_v2 st
JOIN sessions s ON s.id = st.session_id
WHERE st.kind IN ('task','reply')
  AND LOWER(st.content) LIKE '%' || LOWER(?) || '%'
  AND s.deleted_at = ''`
	args := []any{keyword}
	if ws := strings.TrimSpace(workspaceID); ws != "" {
		query += ` AND s.workspace_id = ?`
		args = append(args, ws)
	}
	if aid := strings.TrimSpace(agentID); aid != "" {
		query += ` AND s.agent_id = ?`
		args = append(args, aid)
	}
	query += ` ORDER BY st.started_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(query), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "SESSION_SEARCH")
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, entErrToBizErr(err, "SESSION_SEARCH")
		}
		ids = append(ids, id)
	}
	return ids, entErrToBizErr(rows.Err(), "SESSION_SEARCH")
}

// buildSnippet extracts a window around the first (case-insensitive) keyword
// occurrence; falls back to the head of the content when not found.
func buildSnippet(content, keyword string) string {
	rContent := []rune(content)
	rKeyword := []rune(keyword)
	var idx int
	if len(rKeyword) == 0 {
		idx = -1
	} else {
		idx = -1
		for i := 0; i <= len(rContent)-len(rKeyword); i++ {
			match := true
			for j := 0; j < len(rKeyword); j++ {
				if strings.ToLower(string(rContent[i+j])) != strings.ToLower(string(rKeyword[j])) {
					match = false
					break
				}
			}
			if match {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		if len(rContent) > 2*snippetRadius {
			return string(rContent[:2*snippetRadius]) + "…"
		}
		return content
	}
	start := idx - snippetRadius
	if start < 0 {
		start = 0
	}
	end := idx + len(rKeyword) + snippetRadius
	if end > len(rContent) {
		end = len(rContent)
	}
	snippet := string(rContent[start:end])
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(rContent) {
		snippet += "…"
	}
	return snippet
}
