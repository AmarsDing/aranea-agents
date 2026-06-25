package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/session"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/platformskill"
	entsession "aranea-agents/internal/data/ent/session"
	skillinvocationpkg "aranea-agents/internal/data/ent/skillinvocation"
	toolinvocationpkg "aranea-agents/internal/data/ent/toolinvocation"
	"aranea-agents/internal/llmcontext"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	entsql "entgo.io/ent/dialect/sql"
)

type sessionRepo struct {
	data          *Data
	metricsWriter biz.SessionMetricsWriter // writes to session_metrics table
	metricsCache  *SessionMetricsCache     // optional LRU cache for session_metrics reads
}

var (
	_ biz.SessionReader         = (*sessionRepo)(nil)
	_ biz.SessionWriter         = (*sessionRepo)(nil)
	_ biz.SessionMutator        = (*sessionRepo)(nil)
	_ biz.SessionBatchMutator   = (*sessionRepo)(nil)
	_ session.SessionTreeReader = (*sessionRepo)(nil)
	_ biz.TimelineReader        = (*sessionRepo)(nil)
	_ biz.InvocationReader      = (*sessionRepo)(nil)
	_ biz.SummaryReader         = (*sessionRepo)(nil)
	_ biz.SummaryWriter         = (*sessionRepo)(nil)
	_ biz.StateRepo             = (*sessionRepo)(nil)
	_ biz.TurnRepo              = (*sessionRepo)(nil)
	_ biz.ContextUpdater        = (*sessionRepo)(nil)
	_ biz.CompressRepo          = (*sessionRepo)(nil)
	_ biz.SessionRepo           = (*sessionRepo)(nil)
)

func NewSessionRepo(d *Data, metricsWriter biz.SessionMetricsWriter, metricsCache *SessionMetricsCache) biz.SessionRepo {
	return &sessionRepo{data: d, metricsWriter: metricsWriter, metricsCache: metricsCache}
}

func entSessionToBiz(e *ent.Session) biz.Session {
	if e == nil {
		return biz.Session{}
	}
	return biz.Session{
		ID:                         e.ID,
		WorkspaceID:                e.WorkspaceID,
		UserID:                     e.UserID,
		OwnerType:                  e.OwnerType,
		AgentID:                    e.AgentID,
		TeamID:                     e.TeamID,
		Title:                      e.Title,
		Summary:                    e.Summary,
		TagsJSON:                   e.TagsJSON,
		DialogMode:                 e.DialogMode,
		DefaultProvider:            e.DefaultProvider,
		DefaultModel:               e.DefaultModel,
		DefaultContextWindowTokens: e.DefaultContextWindowTokens,
		LastProvider:               e.LastProvider,
		LastModel:                  e.LastModel,
		LastContextWindowTokens:    e.LastContextWindowTokens,
		Status:                     e.Status,
		StatusReason:               e.StatusReason,
		StatusChangedAt:            e.StatusChangedAt,
		Visibility:                 e.Visibility,
		MessageCount:               e.MessageCount,
		RunCount:                   e.RunCount,
		ModelCallCount:             e.ModelCallCount,
		ToolCallCount:              e.ToolCallCount,
		SkillCallCount:             e.SkillCallCount,
		MCPCallCount:               e.McpCallCount,
		InputTokens:                e.InputTokens,
		OutputTokens:               e.OutputTokens,
		TotalTokens:                e.TotalTokens,
		TotalCostMicroUSD:          e.TotalCostMicroUsd,
		AvgLatencyMs:               e.AvgLatencyMs,
		ErrorCount:                 e.ErrorCount,
		ContextUsedTokens:          e.ContextUsedTokens,
		ContextUsedRatio:           e.ContextUsedRatio,
		MaxContextUsedRatio:        e.MaxContextUsedRatio,
		ContextStatus:              e.ContextStatus,
		FirstMessageAt:             e.FirstMessageAt,
		LastMessageAt:              e.LastMessageAt,
		LastRunAt:                  e.LastRunAt,
		CreatedAt:                  e.CreatedAt,
		UpdatedAt:                  e.UpdatedAt,
		ArchivedAt:                 e.ArchivedAt,
		DeletedAt:                  e.DeletedAt,
		PinnedAt:                   e.PinnedAt,
		RunnerSnapshotJSON:         e.RunnerSnapshotJSON,
		StateJSON:                  e.StateJSON,
		MetadataJSON:               e.MetadataJSON,
		SessionRevision:            e.SessionRevision,
		CompressVersion:            e.CompressVersion,
		ParentSessionID:            e.ParentSessionID,
		RootSessionID:              e.RootSessionID,
		AgentDepth:                 e.AgentDepth,

		// Phase 2: Session tree hierarchy
		SessionType:     e.SessionType,
		MemberAgentKey:  e.MemberAgentKey,
		MemberRole:      e.MemberRole,
		ExecutionStage:  e.ExecutionStage,
		CompletedSteps:  e.CompletedSteps,
		TotalSteps:      e.TotalSteps,
		ProgressPct:     e.ProgressPct,
	}
}

func clampSessionLimit(limit int) int {
	if limit <= 0 || limit > 100 {
		return 20
	}
	return limit
}

func clampOffset(off int) int {
	if off < 0 {
		return 0
	}
	return off
}

func (r *sessionRepo) SearchSessions(ctx context.Context, q biz.SessionSearchQuery) (biz.SessionListResult, error) {
	c := r.data.RW().Read(ctx)
	limit := clampSessionLimit(q.Limit)
	offset := clampOffset(q.Offset)

	wheres := sessionSearchWheres(q)

	wherePred := entsession.And(wheres...)
	total, err := c.Session.Query().Where(wherePred).Count(ctx)
	if err != nil {
		return biz.SessionListResult{}, entErrToBizErr(err, "SESSION")
	}

	orderOpts := sessionSearchOrder(q.SortBy, q.SortOrder)
	rows, err := c.Session.Query().
		Where(wherePred).
		Order(orderOpts...).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return biz.SessionListResult{}, entErrToBizErr(err, "SESSION")
	}
	items := make([]biz.Session, 0, len(rows))
	for _, row := range rows {
		items = append(items, entSessionToBiz(row))
	}
	return biz.SessionListResult{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func (r *sessionRepo) CreateSession(ctx context.Context, in biz.Session) (biz.Session, error) {
	c := r.data.RW().Write(ctx)
	if strings.TrimSpace(in.ID) == "" || strings.TrimSpace(in.Title) == "" {
		return biz.Session{}, apierror.BadRequest("SESSION", "missing required fields")
	}
	if in.OwnerType == "" {
		in.OwnerType = "agent"
	}
	now := nowRFC3339()
	in.CreatedAt = now
	in.UpdatedAt = now
	if in.Status == "" {
		in.Status = string(session.SessionStatusIdle)
	}
	if in.ContextStatus == "" {
		in.ContextStatus = llmcontext.ContextStatusForRatio(in.ContextUsedRatio)
	}

	_, err := c.Session.Create().
		SetID(in.ID).
		SetWorkspaceID(in.WorkspaceID).
		SetUserID(in.UserID).
		SetOwnerType(in.OwnerType).
		SetAgentID(in.AgentID).
		SetTeamID(in.TeamID).
		SetTitle(in.Title).
		SetSummary(in.Summary).
		SetTagsJSON(in.TagsJSON).
		SetDialogMode(in.DialogMode).
		SetDefaultProvider(in.DefaultProvider).
		SetDefaultModel(in.DefaultModel).
		SetDefaultContextWindowTokens(in.DefaultContextWindowTokens).
		SetLastProvider(in.LastProvider).
		SetLastModel(in.LastModel).
		SetLastContextWindowTokens(in.LastContextWindowTokens).
		SetStatus(in.Status).
		SetVisibility(in.Visibility).
		SetMessageCount(in.MessageCount).
		SetRunCount(in.RunCount).
		SetModelCallCount(in.ModelCallCount).
		SetToolCallCount(in.ToolCallCount).
		SetSkillCallCount(in.SkillCallCount).
		SetMcpCallCount(in.MCPCallCount).
		SetInputTokens(in.InputTokens).
		SetOutputTokens(in.OutputTokens).
		SetTotalTokens(in.TotalTokens).
		SetTotalCostMicroUsd(in.TotalCostMicroUSD).
		SetAvgLatencyMs(in.AvgLatencyMs).
		SetErrorCount(in.ErrorCount).
		SetContextUsedTokens(in.ContextUsedTokens).
		SetContextUsedRatio(in.ContextUsedRatio).
		SetMaxContextUsedRatio(in.MaxContextUsedRatio).
		SetContextStatus(in.ContextStatus).
		SetFirstMessageAt(in.FirstMessageAt).
		SetLastMessageAt(in.LastMessageAt).
		SetLastRunAt(in.LastRunAt).
		SetCreatedAt(in.CreatedAt).
		SetUpdatedAt(in.UpdatedAt).
		SetArchivedAt(in.ArchivedAt).
		SetDeletedAt(in.DeletedAt).
		SetRunnerSnapshotJSON(in.RunnerSnapshotJSON).
		SetMetadataJSON(in.MetadataJSON).
		SetParentSessionID(in.ParentSessionID).
		SetRootSessionID(in.RootSessionID).
		SetAgentDepth(in.AgentDepth).
		Save(ctx)
	if err != nil {
		return biz.Session{}, entErrToBizErr(err, "SESSION")
	}
	return r.GetSessionByID(ctx, in.ID)
}

func (r *sessionRepo) GetSessionByID(ctx context.Context, id string) (biz.Session, error) {
	start := time.Now()
	c := r.data.RW().Read(ctx)
	row, err := c.Session.Query().
		Where(entsession.IDEQ(id), entsession.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		r.data.lg.With(loggateway.SessionID(id)).Info("data.GetSessionByID: 失败",
			loggateway.StepID("db.get_session_fail"),
			loggateway.Any("elapsed_ms", time.Since(start).Milliseconds()),
			loggateway.Err(err))
		if ent.IsNotFound(err) {
			return biz.Session{}, apierror.NotFound(apierror.DomainSession, "not found")
		}
		return biz.Session{}, entErrToBizErr(err, "SESSION")
	}
	return entSessionToBiz(row), nil
}

func (r *sessionRepo) UpdateSessionTitle(ctx context.Context, id, title string) (biz.Session, error) {
	c := r.data.RW().Write(ctx)
	_, err := c.Session.Update().
		Where(entsession.IDEQ(id), entsession.DeletedAtEQ("")).
		SetTitle(title).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	if err != nil {
		return biz.Session{}, entErrToBizErr(err, "SESSION")
	}
	return r.GetSessionByID(ctx, id)
}

func (r *sessionRepo) UpdateSession(ctx context.Context, id string, fields biz.SessionUpdateFields) (biz.Session, error) {
	c := r.data.RW().Write(ctx)
	upd := c.Session.Update().
		Where(entsession.IDEQ(id), entsession.DeletedAtEQ("")).
		SetUpdatedAt(nowRFC3339())
	if fields.Title != nil {
		upd = upd.SetTitle(*fields.Title)
	}
	if fields.TagsJSON != nil {
		upd = upd.SetTagsJSON(*fields.TagsJSON)
	}
	if fields.Visibility != nil {
		upd = upd.SetVisibility(*fields.Visibility)
	}
	if fields.MetadataJSON != nil {
		upd = upd.SetMetadataJSON(*fields.MetadataJSON)
	}
	if fields.DialogMode != nil {
		upd = upd.SetDialogMode(*fields.DialogMode)
	}
	if fields.DefaultProvider != nil {
		upd = upd.SetDefaultProvider(*fields.DefaultProvider)
	}
	if fields.DefaultModel != nil {
		upd = upd.SetDefaultModel(*fields.DefaultModel)
	}
	if fields.Status != nil {
		upd = upd.SetStatus(*fields.Status)
	}
	if fields.StatusReason != nil {
		upd = upd.SetStatusReason(*fields.StatusReason)
	}
	if fields.StatusChangedAt != nil {
		upd = upd.SetStatusChangedAt(*fields.StatusChangedAt)
	}
	_, err := upd.Save(ctx)
	if err != nil {
		return biz.Session{}, entErrToBizErr(err, "SESSION")
	}
	return r.GetSessionByID(ctx, id)
}

func (r *sessionRepo) RestoreSession(ctx context.Context, id string) (biz.Session, error) {
	c := r.data.RW().Write(ctx)
	now := nowRFC3339()
	_, err := c.Session.Update().
		Where(entsession.IDEQ(id)).
		SetStatus(string(session.SessionStatusIdle)).
		SetStatusReason("").
		SetStatusChangedAt(now).
		SetArchivedAt("").
		SetDeletedAt("").
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return biz.Session{}, entErrToBizErr(err, "SESSION")
	}
	row, err := c.Session.Get(ctx, id)
	if err != nil {
		return biz.Session{}, entErrToBizErr(err, "SESSION")
	}
	return entSessionToBiz(row), nil
}

func (r *sessionRepo) PinSession(ctx context.Context, id string) (biz.Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return biz.Session{}, apierror.BadRequest("SESSION", "session id is required")
	}
	now := nowRFC3339()
	_, err := r.data.RW().Write(ctx).Session.Update().
		Where(entsession.IDEQ(id), entsession.DeletedAtEQ("")).
		SetPinnedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return biz.Session{}, entErrToBizErr(err, "SESSION")
	}
	return r.GetSessionByID(ctx, id)
}

func (r *sessionRepo) UnpinSession(ctx context.Context, id string) (biz.Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return biz.Session{}, apierror.BadRequest("SESSION", "session id is required")
	}
	now := nowRFC3339()
	_, err := r.data.RW().Write(ctx).Session.Update().
		Where(entsession.IDEQ(id), entsession.DeletedAtEQ("")).
		SetPinnedAt("").
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return biz.Session{}, entErrToBizErr(err, "SESSION")
	}
	return r.GetSessionByID(ctx, id)
}

func (r *sessionRepo) ArchiveSession(ctx context.Context, id string) (int, error) {
	c := r.data.RW().Write(ctx)
	now := nowRFC3339()
	n, err := c.Session.Update().
		Where(entsession.IDEQ(id), entsession.DeletedAtEQ(""), entsession.ArchivedAtEQ(""), entsession.StatusNEQ(string(session.SessionStatusRunning)), entsession.StatusNEQ(string(session.SessionStatusAwaitingConfirmation))).
		SetArchivedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return 0, entErrToBizErr(err, "SESSION")
	}
	return n, nil
}

func (r *sessionRepo) DeleteSession(ctx context.Context, id string) (int, error) {
	var n int
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		c := r.data.RW().Write(txCtx)
		now := nowRFC3339()
		affected, err := c.Session.Update().
			Where(entsession.IDEQ(id), entsession.DeletedAtEQ(""), entsession.StatusNEQ(string(session.SessionStatusRunning)), entsession.StatusNEQ(string(session.SessionStatusAwaitingConfirmation))).
			SetDeletedAt(now).
			SetUpdatedAt(now).
			Save(txCtx)
		if err != nil {
			return entErrToBizErr(err, "SESSION")
		}
		if affected == 0 {
			n = 0
			return nil
		}
		n = affected
		return cascadeDeleteBySession(txCtx, r.data, id)
	})
	return n, entErrToBizErr(err, "SESSION")
}

func (r *sessionRepo) DeleteSessionsByAgentID(ctx context.Context, agentID string) error {
	c := r.data.RW().Write(ctx)
	now := nowRFC3339()
	_, err := c.Session.Update().
		Where(entsession.AgentIDEQ(agentID), entsession.DeletedAtEQ("")).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		r.data.lg.Error("batch delete sessions by agent failed", loggateway.StepID("data.session.delete_by_agent"), loggateway.Err(err))
	}
	return entErrToBizErr(err, "SESSION")
}

func (r *sessionRepo) ListToolInvocationsBySession(ctx context.Context, sessionID string, limit int) ([]biz.ToolInvocationView, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	c := r.data.RW().Read(ctx)
	rows, err := c.ToolInvocation.Query().
		Where(toolinvocationpkg.SessionIDEQ(sessionID)).
		Order(toolinvocationpkg.ByStartedAt(entsql.OrderDesc()), toolinvocationpkg.ByCreatedAt(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SESSION")
	}
	if len(rows) == 0 {
		return nil, nil
	}
	agentNames := map[string]string{}
	agentIDs := make([]string, 0)
	for _, row := range rows {
		if row.AgentID != "" {
			agentIDs = append(agentIDs, row.AgentID)
		}
	}
	agentIDs = dedupeStrings(agentIDs)
	if len(agentIDs) > 0 {
		agents, err := c.Agent.Query().Where(agent.IDIn(agentIDs...), agent.DeletedAtEQ("")).All(ctx)
		if err != nil {
			return nil, entErrToBizErr(err, "SESSION")
		}
		for _, a := range agents {
			agentNames[a.ID] = a.DisplayName
		}
	}
	out := make([]biz.ToolInvocationView, 0, len(rows))
	for _, row := range rows {
		out = append(out, biz.ToolInvocationView{
			ID:               row.ID,
			ToolKey:          row.ToolKey,
			ToolDisplayName:  row.ToolKey,
			AgentID:          row.AgentID,
			AgentDisplayName: agentNames[row.AgentID],
			SessionID:        row.SessionID,
			Source:           row.Source,
			Status:           row.Status,
			StartedAt:        row.StartedAt,
			EndedAt:          row.EndedAt,
			DurationMS:       row.DurationMs,
			InputPreview:     row.InputPreview,
			OutputPreview:    row.OutputPreview,
			ErrorCode:        row.ErrorCode,
			ErrorMessage:     row.ErrorMessage,
			MetadataJSON:     row.MetadataJSON,
			CreatedAt:        row.CreatedAt,
		})
	}
	return out, nil
}

func (r *sessionRepo) ListSkillInvocationsBySession(ctx context.Context, sessionID string, limit int) ([]biz.SkillInvocationView, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	c := r.data.RW().Read(ctx)
	rows, err := c.SkillInvocation.Query().
		Where(skillinvocationpkg.SessionIDEQ(sessionID)).
		Order(skillinvocationpkg.ByStartedAt(entsql.OrderDesc()), skillinvocationpkg.ByCreatedAt(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SESSION")
	}
	if len(rows) == 0 {
		return nil, nil
	}

	skillIDs := dedupeStrings(skillIDsFromSkillInvocations(rows))
	names := map[string]string{}
	if len(skillIDs) > 0 {
		skills, err := c.PlatformSkill.Query().Where(platformskill.IDIn(skillIDs...), platformskill.DeletedAtEQ("")).All(ctx)
		if err != nil {
			return nil, entErrToBizErr(err, "SESSION")
		}
		for _, s := range skills {
			names[s.ID] = s.Name
		}
	}

	agentIDs := dedupeStrings(agentIDsFromSkillInvocations(rows))
	agentNames := map[string]string{}
	if len(agentIDs) > 0 {
		agents, err := c.Agent.Query().Where(agent.IDIn(agentIDs...), agent.DeletedAtEQ("")).All(ctx)
		if err != nil {
			return nil, entErrToBizErr(err, "SESSION")
		}
		for _, a := range agents {
			agentNames[a.ID] = a.DisplayName
		}
	}

	out := make([]biz.SkillInvocationView, 0, len(rows))
	for _, row := range rows {
		started := row.StartedAt
		if strings.TrimSpace(started) == "" {
			started = row.CreatedAt
		}
		out = append(out, biz.SkillInvocationView{
			ID:               row.ID,
			SkillID:          row.SkillID,
			SkillName:        names[row.SkillID],
			SkillVersion:     row.SkillVersion,
			AgentID:          row.AgentID,
			AgentDisplayName: agentNames[row.AgentID],
			SessionID:        row.SessionID,
			Status:           row.Status,
			DurationMS:       row.DurationMs,
			StartedAt:        started,
			EndedAt:          row.EndedAt,
			InputPreview:     row.InputPreview,
			OutputPreview:    row.OutputPreview,
			ErrorCode:        row.ErrorCode,
			ErrorMessage:     row.ErrorMessage,
		})
	}
	return out, nil
}

func skillIDsFromSkillInvocations(rows []*ent.SkillInvocation) []string {
	var ids []string
	for _, row := range rows {
		if row.SkillID != "" {
			ids = append(ids, row.SkillID)
		}
	}
	return ids
}

func agentIDsFromSkillInvocations(rows []*ent.SkillInvocation) []string {
	var ids []string
	for _, row := range rows {
		if row.AgentID != "" {
			ids = append(ids, row.AgentID)
		}
	}
	return ids
}

func dedupeStrings(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func (r *sessionRepo) UpdateRunnerSnapshotJSON(ctx context.Context, sessionID string, snapshotJSON string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return apierror.BadRequest("SESSION", "session id is required")
	}
	_, err := r.data.RW().Write(ctx).Session.Update().
		Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).
		SetRunnerSnapshotJSON(snapshotJSON).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	if err != nil {
		r.data.lg.Warn("update runner snapshot failed", loggateway.StepID("data.session.runner_snapshot"), loggateway.Err(err))
	}
	return entErrToBizErr(err, "SESSION")
}

func (r *sessionRepo) UpdateSessionContextFromLLMUsage(ctx context.Context, sessionID string, promptTokens, _ int, contextWindow int) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return apierror.BadRequest("SESSION", "session id is required")
	}
	ratio := 0.0
	if contextWindow > 0 && promptTokens > 0 {
		ratio = llmcontext.ContextRatio(promptTokens, contextWindow)
	}
	d := r.data.Dialect()
	now := nowRFC3339()
	// PG requires GREATEST() for scalar 2-arg max; SQLite supports multi-arg MAX.
	// `?` will be renumbered to $N by RenumberPlaceholders.
	maxRatioExpr := d.Greatest("max_context_used_ratio", "?")
	sql := `UPDATE sessions
		 SET context_used_ratio = ?,
		     max_context_used_ratio = ` + maxRatioExpr + `,
		     context_status = ?,
		     context_used_tokens = CASE WHEN ? > 0 THEN ? ELSE context_used_tokens END,
		     last_context_window_tokens = CASE WHEN ? > 0 THEN ? ELSE last_context_window_tokens END,
		     updated_at = ?
		 WHERE id = ? AND deleted_at = ''`
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		d.RenumberPlaceholders(sql),
		ratio, ratio, llmcontext.ContextStatusForRatio(ratio),
		promptTokens, promptTokens,
		contextWindow, contextWindow,
		now, sessionID,
	)
	if err != nil {
		r.data.lg.Warn("update session context from llm usage failed", loggateway.StepID("data.session.context_from_llm"), loggateway.Err(err))
		return entErrToBizErr(err, "SESSION")
	}

	// dual_write: also update session_metrics table
	if conf.DAOSessionDualWrite() || conf.DAOSessionMetricsTable() {
		if r.metricsWriter != nil {
			delta := &session.SessionMetricsDelta{
				SessionID:           sessionID,
				ContextUsedTokens:   promptTokens,
				ContextUsedRatio:    ratio,
				MaxContextUsedRatio: ratio,
			}
			if e := r.metricsWriter.ApplyMetricsDelta(ctx, delta); e != nil {
				if conf.DAOSessionDualWrite() {
					// dual_write: new table failure is non-blocking, old table is truth source
					r.data.lg.Warn("dual_write: new table context write failed",
						loggateway.StepID("data.session.dual_write_context"),
						loggateway.SessionID(sessionID),
						loggateway.Err(e))
				} else {
					err = e
				}
			}
		}
		if r.metricsCache != nil {
			r.metricsCache.Invalidate(sessionID)
		}
	}
	return entErrToBizErr(err, "SESSION")
}

func (r *sessionRepo) UpdateSessionContextAfterCompression(ctx context.Context, sessionID string, estimatedPromptTokens int, contextWindow int) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return apierror.BadRequest("SESSION", "session id is required")
	}
	if contextWindow <= 0 {
		contextWindow = 128000
	}
	tok := estimatedPromptTokens
	if tok < 0 {
		tok = 0
	}
	ratio := llmcontext.ContextRatio(tok, contextWindow)
	_, err := r.data.RW().Write(ctx).Session.Update().
		Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).
		SetContextUsedTokens(tok).
		SetContextUsedRatio(ratio).
		SetContextStatus(llmcontext.ContextStatusForRatio(ratio)).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	if err != nil {
		r.data.lg.Warn("update session context after compression failed", loggateway.StepID("data.session.context_after_compress"), loggateway.Err(err))
		return entErrToBizErr(err, "SESSION")
	}

	// dual_write: also update session_metrics table
	if conf.DAOSessionDualWrite() || conf.DAOSessionMetricsTable() {
		if r.metricsWriter != nil {
			delta := &session.SessionMetricsDelta{
				SessionID:           sessionID,
				ContextUsedTokens:   tok,
				ContextUsedRatio:    ratio,
				MaxContextUsedRatio: ratio,
			}
			if e := r.metricsWriter.ApplyMetricsDelta(ctx, delta); e != nil {
				if conf.DAOSessionDualWrite() {
					r.data.lg.Warn("dual_write: new table context write failed",
						loggateway.StepID("data.session.dual_write_context"),
						loggateway.SessionID(sessionID),
						loggateway.Err(e))
				} else {
					err = e
				}
			}
		}
		if r.metricsCache != nil {
			r.metricsCache.Invalidate(sessionID)
		}
	}
	return entErrToBizErr(err, "SESSION")
}

func sessionSearchOrder(sortBy, sortOrder string) []entsession.OrderOption {
	dir := entsql.OrderDesc()
	if strings.EqualFold(sortOrder, "asc") {
		dir = entsql.OrderAsc()
	}
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "created_at":
		return []entsession.OrderOption{entsession.ByCreatedAt(dir)}
	case "updated_at":
		return []entsession.OrderOption{entsession.ByUpdatedAt(dir)}
	case "last_message_at":
		return []entsession.OrderOption{entsession.ByLastMessageAt(dir)}
	case "title":
		return []entsession.OrderOption{entsession.ByTitle(dir)}
	case "pinned_at":
		return []entsession.OrderOption{entsession.ByPinnedAt(dir)}
	default:
		return []entsession.OrderOption{
			entsession.ByPinnedAt(entsql.OrderDesc()),
			entsession.ByLastMessageAt(entsql.OrderDesc()),
			entsession.ByUpdatedAt(entsql.OrderDesc()),
		}
	}
}

func (r *sessionRepo) ApplyMetricsDelta(ctx context.Context, d *session.SessionMetricsDelta) error {
	if d == nil {
		return nil
	}
	sessionID := strings.TrimSpace(d.SessionID)
	if sessionID == "" {
		return nil
	}

	var err error
	switch {
	case conf.DAOSessionDualWrite():
		// 双写：旧表为主，新表为辅。新表写入失败不阻塞旧表，
		// 仅记录日志。dual_write 阶段旧表仍是真相源。
		if e := r.applyMetricsDeltaToSession(ctx, d); e != nil {
			err = e
		}
		if r.metricsWriter != nil {
			if e := r.metricsWriter.ApplyMetricsDelta(ctx, d); e != nil {
				r.data.lg.Warn("dual_write: new table write failed (old table succeeded)",
					loggateway.StepID("data.session.dual_write"),
					loggateway.SessionID(sessionID),
					loggateway.Err(e))
			}
		}
		if r.metricsCache != nil {
			r.metricsCache.Invalidate(sessionID)
		}
	case conf.DAOSessionMetricsTable():
		// 仅写新表
		err = r.metricsWriter.ApplyMetricsDelta(ctx, d)
		if r.metricsCache != nil {
			r.metricsCache.Invalidate(sessionID)
		}
	default:
		// 仅写旧表（当前行为）
		err = r.applyMetricsDeltaToSession(ctx, d)
	}
	return entErrToBizErr(err, "SESSION")
}

// applyMetricsDeltaToSession writes metrics delta to the legacy sessions table.
func (r *sessionRepo) applyMetricsDeltaToSession(ctx context.Context, d *session.SessionMetricsDelta) error {
	upd := r.data.RW().Write(ctx).Session.UpdateOneID(strings.TrimSpace(d.SessionID)).SetUpdatedAt(nowRFC3339())
	if d.MessageCount != 0 {
		upd = upd.AddMessageCount(d.MessageCount)
	}
	if d.ModelCallCount != 0 {
		upd = upd.AddModelCallCount(d.ModelCallCount)
	}
	if d.ToolCallCount != 0 {
		upd = upd.AddToolCallCount(d.ToolCallCount)
	}
	if d.SkillCallCount != 0 {
		upd = upd.AddSkillCallCount(d.SkillCallCount)
	}
	if d.McpCallCount != 0 {
		upd = upd.AddMcpCallCount(d.McpCallCount)
	}
	if d.InputTokens != 0 {
		upd = upd.AddInputTokens(int(d.InputTokens))
	}
	if d.OutputTokens != 0 {
		upd = upd.AddOutputTokens(int(d.OutputTokens))
	}
	if d.TotalTokens != 0 {
		upd = upd.AddTotalTokens(int(d.TotalTokens))
	}
	if d.TotalCostMicroUsd != 0 {
		upd = upd.AddTotalCostMicroUsd(d.TotalCostMicroUsd)
	}
	if d.LastMessageAt != "" {
		upd = upd.SetLastMessageAt(d.LastMessageAt)
	}
	_, err := upd.Save(ctx)
	return entErrToBizErr(err, "SESSION")
}

func (r *sessionRepo) IncrementInvocationCounts(ctx context.Context, sessionID string, toolDelta, mcpDelta, skillDelta int) error {
	if toolDelta == 0 && mcpDelta == 0 && skillDelta == 0 {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	return r.ApplyMetricsDelta(ctx, &session.SessionMetricsDelta{
		SessionID:      sessionID,
		ToolCallCount:  toolDelta,
		McpCallCount:   mcpDelta,
		SkillCallCount: skillDelta,
	})
}

func (r *sessionRepo) BumpSessionRevision(ctx context.Context, sessionID string) (int64, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, apierror.BadRequest("SESSION", "session id is required")
	}
	var rev int64
	err := entQueryRowScan(r.data.RW().Write(ctx), ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE sessions SET session_revision = session_revision + 1, updated_at = ? WHERE id = ? AND deleted_at = '' RETURNING session_revision`),
		[]any{nowRFC3339(), sessionID},
		&rev,
	)
	if err != nil {
		r.data.lg.Warn("bump session revision failed", loggateway.StepID("data.session.bump_revision"), loggateway.Err(err))
		return 0, entErrToBizErr(err, "SESSION")
	}
	return rev, nil
}

func (r *sessionRepo) GetSessionRevision(ctx context.Context, sessionID string) (int64, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, apierror.BadRequest("SESSION", "session id is required")
	}
	row, err := r.data.RW().Read(ctx).Session.Query().
		Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		return 0, entErrToBizErr(err, "SESSION")
	}
	return row.SessionRevision, nil
}

func (r *sessionRepo) TryIncrementCompressVersion(ctx context.Context, sessionID string) (int64, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, apierror.BadRequest("SESSION", "session id is required")
	}
	var old int64
	err := entQueryRowScan(r.data.RW().Write(ctx), ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE sessions SET compress_version = compress_version + 1, updated_at = ? WHERE id = ? AND deleted_at = '' RETURNING compress_version - 1`),
		[]any{nowRFC3339(), sessionID}, &old)
	if err != nil {
		r.data.lg.Error("cas increment compress version failed", loggateway.StepID("data.session.compress_version.cas"), loggateway.Err(err))
	}
	return old, entErrToBizErr(err, "SESSION")
}

func (r *sessionRepo) CompressSessionInTx(ctx context.Context, _ string, fn func(ctx context.Context) error) error {
	return r.data.ExecInTx(ctx, fn)
}

func (r *sessionRepo) ListByParentSessionID(ctx context.Context, parentSessionID string) ([]biz.Session, error) {
	parentSessionID = strings.TrimSpace(parentSessionID)
	if parentSessionID == "" {
		return nil, apierror.BadRequest("SESSION", "parent_session_id is required")
	}
	c := r.data.RW().Read(ctx)
	rows, err := c.Session.Query().
		Where(entsession.ParentSessionIDEQ(parentSessionID), entsession.DeletedAtEQ("")).
		Order(entsession.ByCreatedAt(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SESSION")
	}
	out := make([]biz.Session, 0, len(rows))
	for _, row := range rows {
		out = append(out, entSessionToBiz(row))
	}
	return out, nil
}

// GetSessionTree returns the complete session tree (arbitrary depth) rooted at
// the given spirit session ID. Uses a single query on root_session_id index,
// then builds the recursive tree in memory to avoid N recursive queries.
func (r *sessionRepo) GetSessionTree(ctx context.Context, spiritSessionID string) (*biz.SessionTree, error) {
	spiritSessionID = strings.TrimSpace(spiritSessionID)
	if spiritSessionID == "" {
		return nil, apierror.BadRequest("SESSION", "spirit_session_id is required")
	}
	c := r.data.RW().Read(ctx)
	// One query for all sessions in the tree (root + descendants).
	rows, err := c.Session.Query().
		Where(entsession.RootSessionIDEQ(spiritSessionID), entsession.DeletedAtEQ("")).
		Order(entsession.ByAgentDepth(entsql.OrderAsc()), entsession.ByCreatedAt(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SESSION")
	}

	tree := &biz.SessionTree{}
	nodeMap := make(map[string]*biz.SessionTreeNode, len(rows))

	for _, s := range rows {
		node := &biz.SessionTreeNode{Session: entSessionToBiz(s)}
		nodeMap[s.ID] = node

		switch biz.SessionType(s.SessionType) {
		case biz.SessionTypeSpirit:
			tree.Root = node.Session
		default:
			// Attach to parent node (supports arbitrary depth).
			if parent, ok := nodeMap[s.ParentSessionID]; ok {
				parent.Children = append(parent.Children, node)
			} else {
				// Parent not found (may be filtered out); attach to root.
				tree.Children = append(tree.Children, node)
			}
		}
	}
	return tree, nil
}

// ListChildSessions returns direct child sessions (single level, non-recursive).
func (r *sessionRepo) ListChildSessions(ctx context.Context, parentSessionID string) ([]biz.Session, error) {
	return r.ListByParentSessionID(ctx, parentSessionID)
}

// ListTeamAgentSessions returns all agent-type sessions under a team
// (members and their sub-agents).
func (r *sessionRepo) ListTeamAgentSessions(ctx context.Context, teamID string) ([]biz.Session, error) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil, apierror.BadRequest("SESSION", "team_id is required")
	}
	c := r.data.RW().Read(ctx)
	rows, err := c.Session.Query().
		Where(
			entsession.TeamIDEQ(teamID),
			entsession.SessionTypeEQ(string(biz.SessionTypeAgent)),
			entsession.DeletedAtEQ(""),
		).
		Order(entsession.ByAgentDepth(entsql.OrderAsc()), entsession.ByCreatedAt(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SESSION")
	}
	out := make([]biz.Session, 0, len(rows))
	for _, row := range rows {
		out = append(out, entSessionToBiz(row))
	}
	return out, nil
}
