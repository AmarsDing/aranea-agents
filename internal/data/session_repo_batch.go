package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	session "aranea-agents/internal/biz/session"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/predicate"
	entsession "aranea-agents/internal/data/ent/session"
	"aranea-agents/pkg/loggateway"

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
	rows, err := r.data.RW().Read(ctx).Session.Query().
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
	c := r.data.RW().Read(ctx)
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
		return upd.SetArchivedAt(now).SetUpdatedAt(now)
	})
}

func (r *sessionRepo) DeleteSessionsByIDs(ctx context.Context, ids []string) (int, []string, error) {
	processed, failed, err := r.batchUpdateSessions(ctx, ids, "delete", func(upd *ent.SessionUpdate, now string) *ent.SessionUpdate {
		return upd.SetDeletedAt(now).SetUpdatedAt(now)
	})
	if err != nil {
		return processed, failed, err
	}

	// Cascade: clean up related records for successfully deleted sessions
	failedSet := make(map[string]struct{}, len(failed))
	for _, f := range failed {
		failedSet[f] = struct{}{}
	}
	var cascadeFailed []string
	for _, id := range ids {
		if _, ok := failedSet[id]; ok {
			continue
		}
		if err := cascadeDeleteBySession(ctx, r.data, id); err != nil {
			r.data.lg.Warn("cascade: delete session related records failed",
				loggateway.StepID("data.session.batch"),
				loggateway.Err(err),
				loggateway.Str("session_id", id))
			cascadeFailed = append(cascadeFailed, id)
		}
	}
	if len(cascadeFailed) > 0 {
		failed = append(failed, cascadeFailed...)
	}

	return processed, failed, nil
}

func batchUpdateWheres(mode string, chunk []string) []predicate.Session {
	wheres := []predicate.Session{
		entsession.IDIn(chunk...),
		entsession.DeletedAtEQ(""),
		entsession.StatusNEQ(string(session.SessionStatusRunning)),
		entsession.StatusNEQ(string(session.SessionStatusAwaitingConfirmation)),
	}
	if mode == "archive" {
		wheres = append(wheres, entsession.ArchivedAtEQ(""))
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
	c := r.data.RW().Write(ctx)
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
			r.data.lg.Warn("batch update sessions chunk failed",
				loggateway.StepID("data.session.batch"),
				loggateway.Err(err),
				loggateway.Str("mode", mode),
				loggateway.Int("chunk_size", len(chunk)))
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
	// P2-C: workspace tenancy filter. Empty WorkspaceID = no filter (system caller bypass).
	// Service layer is responsible for setting WorkspaceID from ctx via workspace.IDFromContext.
	if q.WorkspaceID != "" {
		wheres = append(wheres, entsession.WorkspaceIDEQ(q.WorkspaceID))
	}
	if q.OwnerType != "" {
		wheres = append(wheres, entsession.OwnerTypeEQ(q.OwnerType))
	}
	// root_only：只列根会话（侧边栏/管理列表），排除团队成员等子会话。
	// 79-runtime-governance R6：fork 会话 parent_session_id 记来源会话，但它是
	// 用户发起的根级对话（非编排子会话），必须进侧边栏——血缘徽标的载体。
	if q.RootOnly {
		wheres = append(wheres, entsession.Or(
			entsession.ParentSessionIDEQ(""),
			entsession.ForkFromTurnIDNEQ(""),
		))
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
		switch q.Status {
		case "active":
			wheres = append(wheres, entsession.ArchivedAtEQ(""), entsession.DeletedAtEQ(""))
		case "archived":
			wheres = append(wheres, entsession.ArchivedAtNEQ(""))
		case "deleted":
			wheres = append(wheres, entsession.DeletedAtNEQ(""))
		default:
			wheres = append(wheres, entsession.StatusEQ(q.Status))
		}
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

// ListActiveAgentUserKeys returns distinct (agent_id, user_id) pairs for
// sessions that had activity within the given lookback window. "Activity" is
// defined as last_message_at OR last_run_at falling within the last
// lookbackDays days. Only non-archived, non-deleted sessions with non-empty
// agent_id and user_id are considered.
//
// Uses raw SQL for efficient DISTINCT + COALESCE filtering that would be
// awkward to express in Ent. The query is read-only and uses the read
// connection (RWDB().ReadDB).
func (r *sessionRepo) ListActiveAgentUserKeys(ctx context.Context, lookbackDays int) ([]session.AgentUserKey, error) {
	if lookbackDays <= 0 {
		lookbackDays = 7
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -lookbackDays).Format(time.RFC3339)
	q := r.data.Dialect().RenumberPlaceholders(`SELECT DISTINCT agent_id, user_id FROM sessions
WHERE deleted_at = ''
  AND archived_at = ''
  AND agent_id != ''
  AND user_id != ''
  AND (last_message_at >= ? OR last_run_at >= ?)
ORDER BY agent_id, user_id`)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, cutoff, cutoff)
	if err != nil {
		return nil, entErrToBizErr(err, "SESSION")
	}
	defer rows.Close()
	var out []session.AgentUserKey
	for rows.Next() {
		var agentID, userID string
		if scanErr := rows.Scan(&agentID, &userID); scanErr != nil {
			return nil, entErrToBizErr(scanErr, "SESSION")
		}
		agentID = strings.TrimSpace(agentID)
		userID = strings.TrimSpace(userID)
		if agentID == "" || userID == "" {
			continue
		}
		out = append(out, session.AgentUserKey{AgentID: agentID, UserID: userID})
	}
	return out, entErrToBizErr(rows.Err(), "SESSION")
}
