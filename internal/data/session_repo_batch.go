package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/predicate"
	entsession "aranea-agents/internal/data/ent/session"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *sessionRepo) ListSessionsByIDs(ctx context.Context, ids []string) ([]biz.Session, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(ids))
	unique := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return nil, nil
	}
	rows, err := r.data.entClient.Session.Query().
		Where(entsession.IDIn(unique...), entsession.DeletedAtEQ("")).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.Session, 0, len(rows))
	for _, row := range rows {
		out = append(out, entSessionToBiz(row))
	}
	return out, nil
}

func (r *sessionRepo) ListSessionsForBatch(ctx context.Context, q biz.SessionSearchQuery) ([]biz.Session, error) {
	c := r.data.entClient
	limit := q.Limit
	if limit <= 0 {
		limit = biz.SessionBatchPageSize
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	wheres := sessionSearchWheres(q)
	rows, err := c.Session.Query().
		Where(entsession.And(wheres...)).
		Order(entsession.ByID(entsql.OrderAsc())).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.Session, 0, len(rows))
	for _, row := range rows {
		out = append(out, entSessionToBiz(row))
	}
	return out, nil
}

func (r *sessionRepo) ArchiveSessionsByIDs(ctx context.Context, ids []string) (int, []string, error) {
	return r.batchUpdateSessions(ctx, ids, "archive", func(upd *ent.SessionUpdate, now string) *ent.SessionUpdate {
		return upd.SetStatus("archived").SetArchivedAt(now).SetUpdatedAt(now)
	})
}

func (r *sessionRepo) DeleteSessionsByIDs(ctx context.Context, ids []string) (int, []string, error) {
	return r.batchUpdateSessions(ctx, ids, "delete", func(upd *ent.SessionUpdate, now string) *ent.SessionUpdate {
		return upd.SetStatus("deleted").SetDeletedAt(now).SetUpdatedAt(now)
	})
}

func batchUpdateWheres(mode string, chunk []string) []predicate.Session {
	wheres := []predicate.Session{
		entsession.IDIn(chunk...),
		entsession.DeletedAtEQ(""),
		entsession.StatusNEQ("running"),
	}
	if mode == "archive" {
		wheres = append(wheres, entsession.StatusNEQ("archived"))
	}
	return wheres
}

func (r *sessionRepo) batchUpdateSessions(
	ctx context.Context,
	ids []string,
	mode string,
	apply func(upd *ent.SessionUpdate, now string) *ent.SessionUpdate,
) (int, []string, error) {
	if len(ids) == 0 {
		return 0, nil, nil
	}
	const chunkSize = 500
	c := r.data.entClient
	now := nowRFC3339()
	processed := 0
	var failed []string
	for i := 0; i < len(ids); i += chunkSize {
		end := i + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[i:end]
		n, err := apply(c.Session.Update(), now).
			Where(entsession.And(batchUpdateWheres(mode, chunk)...)).
			Save(ctx)
		if err != nil {
			failed = append(failed, chunk...)
			continue
		}
		processed += n
	}
	return processed, failed, nil
}

// sessionSearchWheres builds predicates shared by SearchSessions and batch list.
func sessionSearchWheres(q biz.SessionSearchQuery) []predicate.Session {
	wheres := []predicate.Session{entsession.DeletedAtEQ("")}
	if q.OwnerType != "" {
		wheres = append(wheres, entsession.OwnerTypeEQ(q.OwnerType))
	}
	if q.AgentID != "" {
		wheres = append(wheres, entsession.AgentIDEQ(q.AgentID))
	}
	if q.TeamID != "" {
		wheres = append(wheres, entsession.TeamIDEQ(q.TeamID))
	}
	if q.UserID != "" {
		wheres = append(wheres, entsession.UserIDEQ(q.UserID))
	}
	if q.Status != "" {
		wheres = append(wheres, entsession.StatusEQ(q.Status))
	}
	if q.ContextStatus != "" {
		wheres = append(wheres, entsession.ContextStatusEQ(q.ContextStatus))
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		wheres = append(wheres, entsession.Or(
			entsession.TitleContainsFold(kw),
			entsession.SummaryContainsFold(kw),
			entsession.IDContainsFold(kw),
		))
	}
	return wheres
}
