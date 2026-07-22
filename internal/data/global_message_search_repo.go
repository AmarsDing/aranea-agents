package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	entsession "aranea-agents/internal/data/ent/session"
	"aranea-agents/internal/data/ent/stepv2"

	entsql "entgo.io/ent/dialect/sql"
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

func (r *globalMessageSearchRepo) SearchGlobalMessages(ctx context.Context, keyword, agentID string, limit int) ([]biz.GlobalMessageHit, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" || limit <= 0 {
		return nil, nil
	}
	query := r.data.RW().Read(ctx).StepV2.Query().
		Where(
			stepv2.KindIn("task", "reply"),
			stepv2.ContentContains(keyword),
		)
	if agentID = strings.TrimSpace(agentID); agentID != "" {
		sessionIDs, err := r.data.RW().Read(ctx).Session.Query().
			Where(entsession.AgentIDEQ(agentID), entsession.DeletedAtEQ("")).
			IDs(ctx)
		if err != nil {
			return nil, entErrToBizErr(err, "SESSION_SEARCH")
		}
		if len(sessionIDs) == 0 {
			return nil, nil
		}
		query = query.Where(stepv2.SessionIDIn(sessionIDs...))
	}
	rows, err := query.
		Order(stepv2.ByStartedAt(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SESSION_SEARCH")
	}
	out := make([]biz.GlobalMessageHit, 0, len(rows))
	for _, e := range rows {
		out = append(out, biz.GlobalMessageHit{
			ID:             e.ID,
			SessionID:      e.SessionID,
			Kind:           e.Kind,
			AuthorAgentKey: e.AuthorAgentKey,
			Snippet:        buildSnippet(e.Content, keyword),
			StartedAt:      e.StartedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return out, nil
}

// buildSnippet extracts a window around the first (case-insensitive) keyword
// occurrence; falls back to the head of the content when not found.
func buildSnippet(content, keyword string) string {
	idx := strings.Index(strings.ToLower(content), strings.ToLower(keyword))
	if idx < 0 {
		if len(content) > 2*snippetRadius {
			return content[:2*snippetRadius] + "…"
		}
		return content
	}
	start := idx - snippetRadius
	if start < 0 {
		start = 0
	}
	end := idx + len(keyword) + snippetRadius
	if end > len(content) {
		end = len(content)
	}
	snippet := content[start:end]
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(content) {
		snippet += "…"
	}
	return snippet
}
