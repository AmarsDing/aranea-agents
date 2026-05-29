package data

import (
	"context"
	"database/sql"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/platformskill"
	entsession "aranea-agents/internal/data/ent/session"
	skillinvocationpkg "aranea-agents/internal/data/ent/skillinvocation"
	toolinvocationpkg "aranea-agents/internal/data/ent/toolinvocation"
	"aranea-agents/internal/llmcontext"

	entsql "entgo.io/ent/dialect/sql"
	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type sessionRepo struct {
	data *Data
}

var (
	_ biz.SessionReader        = (*sessionRepo)(nil)
	_ biz.SessionWriter        = (*sessionRepo)(nil)
	_ biz.SessionBatchWriter   = (*sessionRepo)(nil)
	_ biz.SessionPinWriter     = (*sessionRepo)(nil)
	_ biz.SessionRevisionWriter = (*sessionRepo)(nil)
	_ biz.MessageReader        = (*sessionRepo)(nil)
	_ biz.MessageSearchReader  = (*sessionRepo)(nil)
	_ biz.MessageWriter        = (*sessionRepo)(nil)
	_ biz.MessageStatusWriter  = (*sessionRepo)(nil)
	_ biz.TimelineReader       = (*sessionRepo)(nil)
	_ biz.InvocationReader     = (*sessionRepo)(nil)
	_ biz.SummaryReader        = (*sessionRepo)(nil)
	_ biz.SummaryWriter        = (*sessionRepo)(nil)
	_ biz.StateRepo            = (*sessionRepo)(nil)
	_ biz.TurnRepo             = (*sessionRepo)(nil)
	_ biz.ContextUpdater       = (*sessionRepo)(nil)
	_ biz.CompressRepo         = (*sessionRepo)(nil)
	_ biz.SessionRepo           = (*sessionRepo)(nil)
)

func NewSessionRepo(d *Data) biz.SessionRepo {
	return &sessionRepo{data: d}
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
	c := r.data.entClient
	limit := clampSessionLimit(q.Limit)
	offset := clampOffset(q.Offset)

	wheres := sessionSearchWheres(q)

	wherePred := entsession.And(wheres...)
	total, err := c.Session.Query().Where(wherePred).Count(ctx)
	if err != nil {
		return biz.SessionListResult{}, err
	}

	orderOpts := sessionSearchOrder(q.SortBy, q.SortOrder)
	rows, err := c.Session.Query().
		Where(wherePred).
		Order(orderOpts...).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return biz.SessionListResult{}, err
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
	c := r.data.entClient
	if strings.TrimSpace(in.ID) == "" || strings.TrimSpace(in.Title) == "" {
		return biz.Session{}, kerrors.BadRequest("SESSION", "missing required fields")
	}
	if in.OwnerType == "" {
		in.OwnerType = "agent"
	}
	now := nowRFC3339()
	in.CreatedAt = now
	in.UpdatedAt = now
	if in.Status == "" {
		in.Status = "active"
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
		Save(ctx)
	if err != nil {
		return biz.Session{}, err
	}
	return r.GetSessionByID(ctx, in.ID)
}

func (r *sessionRepo) GetSessionByID(ctx context.Context, id string) (biz.Session, error) {
	c := r.data.entClient
	row, err := c.Session.Query().
		Where(entsession.IDEQ(id), entsession.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Session{}, sql.ErrNoRows
		}
		return biz.Session{}, err
	}
	return entSessionToBiz(row), nil
}

func (r *sessionRepo) UpdateSessionTitle(ctx context.Context, id, title string) (biz.Session, error) {
	c := r.data.entClient
	_, err := c.Session.Update().
		Where(entsession.IDEQ(id), entsession.DeletedAtEQ("")).
		SetTitle(title).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	if err != nil {
		return biz.Session{}, err
	}
	return r.GetSessionByID(ctx, id)
}

func (r *sessionRepo) UpdateSession(ctx context.Context, id string, fields biz.SessionUpdateFields) (biz.Session, error) {
	c := r.data.entClient
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
	_, err := upd.Save(ctx)
	if err != nil {
		return biz.Session{}, err
	}
	return r.GetSessionByID(ctx, id)
}

func (r *sessionRepo) RestoreSession(ctx context.Context, id string) (biz.Session, error) {
	c := r.data.entClient
	_, err := c.Session.Update().
		Where(entsession.IDEQ(id)).
		SetStatus("active").
		SetArchivedAt("").
		SetDeletedAt("").
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	if err != nil {
		return biz.Session{}, err
	}
	row, err := c.Session.Get(ctx, id)
	if err != nil {
		return biz.Session{}, err
	}
	return entSessionToBiz(row), nil
}

func (r *sessionRepo) PinSession(ctx context.Context, id string) (biz.Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return biz.Session{}, kerrors.BadRequest("SESSION", "session id is required")
	}
	now := nowRFC3339()
	_, err := r.data.entClient.Session.Update().
		Where(entsession.IDEQ(id), entsession.DeletedAtEQ("")).
		SetPinnedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return biz.Session{}, err
	}
	return r.GetSessionByID(ctx, id)
}

func (r *sessionRepo) UnpinSession(ctx context.Context, id string) (biz.Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return biz.Session{}, kerrors.BadRequest("SESSION", "session id is required")
	}
	now := nowRFC3339()
	_, err := r.data.entClient.Session.Update().
		Where(entsession.IDEQ(id), entsession.DeletedAtEQ("")).
		SetPinnedAt("").
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return biz.Session{}, err
	}
	return r.GetSessionByID(ctx, id)
}

func (r *sessionRepo) ArchiveSession(ctx context.Context, id string) (int, error) {
	c := r.data.entClient
	now := nowRFC3339()
	n, err := c.Session.Update().
		Where(entsession.IDEQ(id), entsession.DeletedAtEQ(""), entsession.StatusNEQ("running"), entsession.StatusNEQ("archived")).
		SetStatus("archived").
		SetArchivedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (r *sessionRepo) DeleteSession(ctx context.Context, id string) (int, error) {
	c := r.data.entClient
	now := nowRFC3339()
	n, err := c.Session.Update().
		Where(entsession.IDEQ(id), entsession.DeletedAtEQ(""), entsession.StatusNEQ("running")).
		SetDeletedAt(now).
		SetStatus("deleted").
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}
	_, _ = NewChannelPeerSessionRepo(r.data).DeleteBySessionID(ctx, id)
	return n, nil
}

func (r *sessionRepo) DeleteSessionsByAgentID(ctx context.Context, agentID string) error {
	c := r.data.entClient
	now := nowRFC3339()
	_, err := c.Session.Update().
		Where(entsession.AgentIDEQ(agentID), entsession.DeletedAtEQ("")).
		SetDeletedAt(now).
		SetStatus("deleted").
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func (r *sessionRepo) ListToolInvocationsBySession(ctx context.Context, sessionID string, limit int) ([]biz.ToolInvocationView, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	c := r.data.entClient
	rows, err := c.ToolInvocation.Query().
		Where(toolinvocationpkg.SessionIDEQ(sessionID)).
		Order(toolinvocationpkg.ByStartedAt(entsql.OrderDesc()), toolinvocationpkg.ByCreatedAt(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
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
			return nil, err
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
	c := r.data.entClient
	rows, err := c.SkillInvocation.Query().
		Where(skillinvocationpkg.SessionIDEQ(sessionID)).
		Order(skillinvocationpkg.ByStartedAt(entsql.OrderDesc()), skillinvocationpkg.ByCreatedAt(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	skillIDs := dedupeStrings(skillIDsFromSkillInvocations(rows))
	names := map[string]string{}
	if len(skillIDs) > 0 {
		skills, err := c.PlatformSkill.Query().Where(platformskill.IDIn(skillIDs...), platformskill.DeletedAtEQ("")).All(ctx)
		if err != nil {
			return nil, err
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
			return nil, err
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
		return kerrors.BadRequest("SESSION", "session id is required")
	}
	_, err := r.data.entClient.Session.Update().
		Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).
		SetRunnerSnapshotJSON(snapshotJSON).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	return err
}

func (r *sessionRepo) UpdateSessionContextFromLLMUsage(ctx context.Context, sessionID string, promptTokens, _ int, contextWindow int) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return kerrors.BadRequest("SESSION", "session id is required")
	}
	ratio := 0.0
	if contextWindow > 0 && promptTokens > 0 {
		ratio = llmcontext.ContextRatio(promptTokens, contextWindow)
	}
	now := nowRFC3339()
	_, err := r.data.RawDB().ExecContext(ctx,
		`UPDATE sessions
		 SET context_used_ratio = ?,
		     max_context_used_ratio = MAX(max_context_used_ratio, ?),
		     context_status = ?,
		     context_used_tokens = CASE WHEN ? > 0 THEN ? ELSE context_used_tokens END,
		     last_context_window_tokens = CASE WHEN ? > 0 THEN ? ELSE last_context_window_tokens END,
		     updated_at = ?
		 WHERE id = ? AND deleted_at = ''`,
		ratio, ratio, llmcontext.ContextStatusForRatio(ratio),
		promptTokens, promptTokens,
		contextWindow, contextWindow,
		now, sessionID,
	)
	return err
}

func (r *sessionRepo) UpdateSessionContextAfterCompression(ctx context.Context, sessionID string, estimatedPromptTokens int, contextWindow int) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return kerrors.BadRequest("SESSION", "session id is required")
	}
	if contextWindow <= 0 {
		contextWindow = 128000
	}
	tok := estimatedPromptTokens
	if tok < 0 {
		tok = 0
	}
	ratio := llmcontext.ContextRatio(tok, contextWindow)
	_, err := r.data.entClient.Session.Update().
		Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).
		SetContextUsedTokens(tok).
		SetContextUsedRatio(ratio).
		SetContextStatus(llmcontext.ContextStatusForRatio(ratio)).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	return err
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

func (r *sessionRepo) IncrementInvocationCounts(ctx context.Context, sessionID string, toolDelta, mcpDelta, skillDelta int) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || (toolDelta == 0 && mcpDelta == 0 && skillDelta == 0) {
		return nil
	}
	c := r.data.entClient
	upd := c.Session.UpdateOneID(sessionID).SetUpdatedAt(nowRFC3339())
	if toolDelta != 0 {
		upd = upd.AddToolCallCount(toolDelta)
	}
	if mcpDelta != 0 {
		upd = upd.AddMcpCallCount(mcpDelta)
	}
	if skillDelta != 0 {
		upd = upd.AddSkillCallCount(skillDelta)
	}
	_, err := upd.Save(ctx)
	return err
}

func (r *sessionRepo) BumpSessionRevision(ctx context.Context, sessionID string) (int64, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, kerrors.BadRequest("SESSION", "session id is required")
	}
	var rev int64
	err := entQueryRowScan(r.data.entClient, ctx,
		`UPDATE sessions SET session_revision = session_revision + 1, updated_at = ? WHERE id = ? AND deleted_at = '' RETURNING session_revision`,
		[]any{nowRFC3339(), sessionID},
		&rev,
	)
	if err != nil {
		return 0, err
	}
	return rev, nil
}

func (r *sessionRepo) GetSessionRevision(ctx context.Context, sessionID string) (int64, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, kerrors.BadRequest("SESSION", "session id is required")
	}
	row, err := r.data.entClient.Session.Query().
		Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		return 0, err
	}
	return row.SessionRevision, nil
}

func (r *sessionRepo) TryIncrementCompressVersion(ctx context.Context, sessionID string) (int64, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, kerrors.BadRequest("SESSION", "session id is required")
	}
	var old int64
	err := entQueryRowScan(r.data.entClient, ctx,
		`UPDATE sessions SET compress_version = compress_version + 1, updated_at = ? WHERE id = ? AND deleted_at = '' RETURNING compress_version - 1`,
		[]any{nowRFC3339(), sessionID}, &old)
	return old, err
}

func (r *sessionRepo) CompressSessionInTx(ctx context.Context, _ string, fn func(ctx context.Context) error) error {
	tx, err := r.data.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	if err := fn(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
