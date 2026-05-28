package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/message"
	"aranea-agents/internal/data/ent/platformskill"
	entsession "aranea-agents/internal/data/ent/session"
	entsessionturn "aranea-agents/internal/data/ent/sessionturn"
	skillinvocationpkg "aranea-agents/internal/data/ent/skillinvocation"
	toolinvocationpkg "aranea-agents/internal/data/ent/toolinvocation"
	"aranea-agents/internal/llmcontext"

	entsql "entgo.io/ent/dialect/sql"
	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

type sessionRepo struct {
	data *Data
}

// NewSessionRepo implements biz.SessionRepository.
func NewSessionRepo(d *Data) biz.SessionRepository {
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

func entMessageToBiz(m *ent.Message) biz.ChatMessage {
	if m == nil {
		return biz.ChatMessage{}
	}
	return biz.ChatMessage{
		ID: m.ID, SessionID: m.SessionID, ParentMessageID: m.ParentMessageID,
		TurnID: m.TurnID, TurnNumber: m.TurnNumber, SeqInTurn: m.SeqInTurn, Role: m.Role, ContentMarkdown: m.ContentMarkdown,
		ModelName: m.ModelName, TokenIn: m.TokenIn, TokenOut: m.TokenOut,
		LatencyMS: m.LatencyMs, Status: m.Status, AttachmentsCount: m.AttachmentsCount,
		OptionsJSON: m.OptionsJSON, ErrorMessage: m.ErrorMessage, CreatedAt: m.CreatedAt,
	}
}

func (r *sessionRepo) CountMessagesBySession(ctx context.Context, sessionID string) (int, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, kerrors.BadRequest("SESSION", "session id is required")
	}
	return r.data.entClient.Message.Query().Where(message.SessionIDEQ(sessionID)).Count(ctx)
}

func (r *sessionRepo) ListMessagesBySession(ctx context.Context, sessionID string, limit, offset int) ([]biz.ChatMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, kerrors.BadRequest("SESSION", "session id is required")
	}
	if limit <= 0 {
		limit = biz.MessageListDefaultLimit
	}
	if limit > biz.MessageListMaxLimit {
		limit = biz.MessageListMaxLimit
	}
	rows, err := r.data.entClient.Message.Query().
		Where(message.SessionIDEQ(sessionID)).
		Order(message.ByTurnID(entsql.OrderAsc()), message.BySeqInTurn(entsql.OrderAsc()), message.ByCreatedAt(entsql.OrderAsc())).
		Limit(limit).Offset(clampOffset(offset)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.ChatMessage, 0, len(rows))
	for _, m := range rows {
		out = append(out, entMessageToBiz(m))
	}
	return out, nil
}

func (r *sessionRepo) ListMessagesAfterTurn(ctx context.Context, sessionID string, afterTurn int) ([]biz.ChatMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, kerrors.BadRequest("SESSION", "session id is required")
	}
	turnIDs, err := r.data.entClient.SessionTurn.Query().
		Where(entsessionturn.SessionIDEQ(sessionID), entsessionturn.TurnNumberGT(afterTurn)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(turnIDs))
	for _, t := range turnIDs {
		ids = append(ids, t.ID)
	}
	if len(ids) == 0 {
		return []biz.ChatMessage{}, nil
	}
	q := r.data.entClient.Message.Query().Where(message.SessionIDEQ(sessionID), message.TurnIDIn(ids...))
	rows, err := q.Order(message.ByTurnID(entsql.OrderAsc()), message.BySeqInTurn(entsql.OrderAsc()), message.ByCreatedAt(entsql.OrderAsc())).
		Limit(biz.CompressMessageMaxRows).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.ChatMessage, 0, len(rows))
	for _, m := range rows {
		out = append(out, entMessageToBiz(m))
	}
	return out, nil
}

func (r *sessionRepo) ListMessagesByStatus(ctx context.Context, sessionID, status string, limit int) ([]biz.ChatMessage, error) {
	sessionID, status = strings.TrimSpace(sessionID), strings.TrimSpace(status)
	if sessionID == "" || status == "" {
		return nil, kerrors.BadRequest("SESSION", "session id and status are required")
	}
	if limit <= 0 || limit > biz.ActivityCancelScanLimit {
		limit = biz.ActivityCancelScanLimit
	}
	rows, err := r.data.entClient.Message.Query().
		Where(message.SessionIDEQ(sessionID), message.StatusEQ(status)).
		Order(message.ByTurnID(entsql.OrderDesc()), message.BySeqInTurn(entsql.OrderDesc()), message.ByCreatedAt(entsql.OrderDesc())).
		Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.ChatMessage, 0, len(rows))
	for _, m := range rows {
		out = append(out, entMessageToBiz(m))
	}
	return out, nil
}

func (r *sessionRepo) ListMessagesRecent(ctx context.Context, sessionID string, limit int) ([]biz.ChatMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, kerrors.BadRequest("SESSION", "session id is required")
	}
	if limit <= 0 || limit > biz.TimelineMessageMaxFetch {
		limit = biz.TimelineMessageMaxFetch
	}
	rows, err := r.data.entClient.Message.Query().
		Where(message.SessionIDEQ(sessionID)).
		Order(message.ByTurnID(entsql.OrderDesc()), message.BySeqInTurn(entsql.OrderDesc()), message.ByCreatedAt(entsql.OrderDesc())).
		Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.ChatMessage, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		out = append(out, entMessageToBiz(rows[i]))
	}
	return out, nil
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

func (r *sessionRepo) maxMessageTurnTx(ctx context.Context, tx *ent.Tx, sessionID string) (int, error) {
	row, err := tx.SessionTurn.Query().
		Where(entsessionturn.SessionIDEQ(sessionID)).
		Order(entsessionturn.ByTurnNumber(entsql.OrderDesc())).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	return row.TurnNumber, nil
}

func (r *sessionRepo) assignTurnForNewMessage(ctx context.Context, tx *ent.Tx, sessionID, role string) (turnID string, turnNumber int, seqInTurn int, err error) {
	latestTurn, qErr := tx.SessionTurn.Query().
		Where(entsessionturn.SessionIDEQ(sessionID)).
		Order(entsessionturn.ByTurnNumber(entsql.OrderDesc())).
		First(ctx)
	if qErr != nil && !ent.IsNotFound(qErr) {
		return "", 0, 0, qErr
	}
	if latestTurn != nil {
		shouldReuse := false
		switch role {
		case "user":
			shouldReuse = latestTurn.Status == "awaiting_user"
		default:
			shouldReuse = latestTurn.Status != "completed" && latestTurn.Status != "failed" && latestTurn.Status != "cancelled"
		}
		if shouldReuse {
			maxSeq, seqErr := tx.Message.Query().
				Where(message.SessionIDEQ(sessionID), message.TurnIDEQ(latestTurn.ID)).
				Order(message.BySeqInTurn(entsql.OrderDesc())).
				First(ctx)
			if seqErr != nil && !ent.IsNotFound(seqErr) {
				return "", 0, 0, seqErr
			}
			nextSeq := 1
			if maxSeq != nil {
				nextSeq = maxSeq.SeqInTurn + 1
			}
			return latestTurn.ID, latestTurn.TurnNumber, nextSeq, nil
		}
	}
	maxTurn, mErr := r.maxMessageTurnTx(ctx, tx, sessionID)
	if mErr != nil {
		return "", 0, 0, mErr
	}
	newTurnID := uuid.NewString()
	newTurnNumber := maxTurn + 1
	now := nowRFC3339()
	if _, cErr := tx.SessionTurn.Create().
		SetID(newTurnID).
		SetSessionID(sessionID).
		SetTurnNumber(newTurnNumber).
		SetStatus("running").
		SetStartedAt(now).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); cErr != nil {
		return "", 0, 0, cErr
	}
	return newTurnID, newTurnNumber, 1, nil
}

func (r *sessionRepo) insertMessageTx(ctx context.Context, tx *ent.Tx, m biz.ChatMessage) error {
	return tx.Message.Create().
		SetID(m.ID).
		SetSessionID(m.SessionID).
		SetParentMessageID(m.ParentMessageID).
		SetTurnID(m.TurnID).
		SetTurnNumber(m.TurnNumber).
		SetSeqInTurn(m.SeqInTurn).
		SetRole(m.Role).
		SetContentMarkdown(m.ContentMarkdown).
		SetModelName(m.ModelName).
		SetTokenIn(m.TokenIn).
		SetTokenOut(m.TokenOut).
		SetLatencyMs(m.LatencyMS).
		SetStatus(m.Status).
		SetAttachmentsCount(m.AttachmentsCount).
		SetOptionsJSON(m.OptionsJSON).
		SetErrorMessage(m.ErrorMessage).
		SetCreatedAt(m.CreatedAt).
		Exec(ctx)
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

func (r *sessionRepo) GetSessionState(ctx context.Context, sessionID string) (map[string]string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, kerrors.BadRequest("SESSION", "session id is required")
	}
	row, err := r.data.entClient.Session.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	state := map[string]string{}
	if row.StateJSON != "" {
		_ = json.Unmarshal([]byte(row.StateJSON), &state)
	}
	return state, nil
}

func (r *sessionRepo) SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return kerrors.BadRequest("SESSION", "session id is required")
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = r.data.entClient.Session.Update().
		Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).
		SetStateJSON(string(raw)).
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
	_, err := r.data.rawDB.ExecContext(ctx,
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

func (r *sessionRepo) AppendChatTurn(ctx context.Context, sessionID string, user, assistant biz.ChatMessage) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return kerrors.BadRequest("SESSION", "session id is required")
	}
	tx, err := r.data.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	rollback := func(e error) error {
		_ = tx.Rollback()
		return e
	}
	if _, err = tx.Session.Query().Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).Only(ctx); err != nil {
		return rollback(err)
	}
	maxTurn, err := r.maxMessageTurnTx(ctx, tx, sessionID)
	if err != nil {
		return rollback(err)
	}
	turnID := uuid.NewString()
	turnNumber := maxTurn + 1
	now := nowRFC3339()
	if _, err = tx.SessionTurn.Create().
		SetID(turnID).
		SetSessionID(sessionID).
		SetTurnNumber(turnNumber).
		SetUserMessageID(user.ID).
		SetAssistantMessageID(assistant.ID).
		SetStatus("completed").
		SetStartedAt(now).
		SetEndedAt(now).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return rollback(err)
	}
	user.TurnID = turnID
	user.TurnNumber = turnNumber
	user.SeqInTurn = 1
	assistant.TurnID = turnID
	assistant.TurnNumber = turnNumber
	assistant.SeqInTurn = 2
	if err = r.insertMessageTx(ctx, tx, user); err != nil {
		return rollback(err)
	}
	if err = r.insertMessageTx(ctx, tx, assistant); err != nil {
		return rollback(err)
	}
	upd := tx.Session.UpdateOneID(sessionID).
		AddMessageCount(2).
		SetLastMessageAt(assistant.CreatedAt).
		SetUpdatedAt(nowRFC3339()).
		AddModelCallCount(1)
	if tin, tout := assistant.TokenIn, assistant.TokenOut; tin > 0 || tout > 0 {
		upd = upd.AddInputTokens(tin).AddOutputTokens(tout).AddTotalTokens(tin + tout)
	}
	if _, err = upd.Save(ctx); err != nil {
		return rollback(err)
	}
	if err = tx.Commit(); err != nil {
		return rollback(err)
	}
	return nil
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

// TODO(finding-5): UpsertChatActivityMessage still uses manual Tx instead of ExecInTx + txClient.
// sessionRepo has no txClient helper yet; refactor when available.
func (r *sessionRepo) UpsertChatActivityMessage(ctx context.Context, sessionID string, msg biz.ChatMessage) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return kerrors.BadRequest("SESSION", "session id is required")
	}
	msg.ID = strings.TrimSpace(msg.ID)
	if msg.ID == "" {
		return kerrors.BadRequest("SESSION", "message id is required")
	}
	tx, err := r.data.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	rollback := func(e error) error {
		_ = tx.Rollback()
		return e
	}
	if _, err = tx.Session.Query().Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).Only(ctx); err != nil {
		return rollback(err)
	}
	existing, err := tx.Message.Query().Where(message.IDEQ(msg.ID), message.SessionIDEQ(sessionID)).Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			return rollback(err)
		}
		msg.SessionID = sessionID
		turnID, turnNum, seqInTurn, merr := r.assignTurnForNewMessage(ctx, tx, sessionID, msg.Role)
		if merr != nil {
			return rollback(merr)
		}
		msg.TurnID = turnID
		msg.TurnNumber = turnNum
		msg.SeqInTurn = seqInTurn
		if strings.TrimSpace(msg.CreatedAt) == "" {
			msg.CreatedAt = nowRFC3339()
		}
		if err = r.insertMessageTx(ctx, tx, msg); err != nil {
			return rollback(err)
		}
		if _, err = tx.Session.UpdateOneID(sessionID).
			AddMessageCount(1).
			SetLastMessageAt(msg.CreatedAt).
			SetUpdatedAt(nowRFC3339()).
			Save(ctx); err != nil {
			return rollback(err)
		}
		return tx.Commit()
	}
	lastAt := msg.CreatedAt
	if strings.TrimSpace(lastAt) == "" {
		lastAt = existing.CreatedAt
	}
	update := tx.Message.UpdateOneID(msg.ID).
		SetContentMarkdown(msg.ContentMarkdown).
		SetOptionsJSON(msg.OptionsJSON).
		SetStatus(msg.Status).
		SetLatencyMs(msg.LatencyMS).
		SetErrorMessage(msg.ErrorMessage)
	if msg.TokenIn > 0 {
		update = update.SetTokenIn(msg.TokenIn)
	}
	if msg.TokenOut > 0 {
		update = update.SetTokenOut(msg.TokenOut)
	}
	if msg.ModelName != "" {
		update = update.SetModelName(msg.ModelName)
	}
	if _, err = update.Save(ctx); err != nil {
		return rollback(err)
	}
	if _, err = tx.Session.UpdateOneID(sessionID).
		SetLastMessageAt(lastAt).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx); err != nil {
		return rollback(err)
	}
	return tx.Commit()
}

func (r *sessionRepo) AppendChatMessage(ctx context.Context, sessionID string, msg biz.ChatMessage, bumpModelCall bool) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return kerrors.BadRequest("SESSION", "session id is required")
	}
	tx, err := r.data.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	rollback := func(e error) error {
		_ = tx.Rollback()
		return e
	}
	if _, err = tx.Session.Query().Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).Only(ctx); err != nil {
		return rollback(err)
	}
	turnID, turnNum, seqInTurn, err := r.assignTurnForNewMessage(ctx, tx, sessionID, msg.Role)
	if err != nil {
		return rollback(err)
	}
	msg.TurnID = turnID
	msg.TurnNumber = turnNum
	msg.SeqInTurn = seqInTurn
	if err = r.insertMessageTx(ctx, tx, msg); err != nil {
		if ent.IsConstraintError(err) {
			_ = tx.Rollback()
			return biz.ErrMessageDuplicate
		}
		return rollback(err)
	}
	upd := tx.Session.UpdateOneID(sessionID).
		AddMessageCount(1).
		SetLastMessageAt(msg.CreatedAt).
		SetUpdatedAt(nowRFC3339())
	if bumpModelCall {
		upd = upd.AddModelCallCount(1)
	}
	tin, tout := msg.TokenIn, msg.TokenOut
	if bumpModelCall && (tin > 0 || tout > 0) {
		upd = upd.AddInputTokens(tin).AddOutputTokens(tout).AddTotalTokens(tin + tout)
	}
	if _, err = upd.Save(ctx); err != nil {
		return rollback(err)
	}
	if err = tx.Commit(); err != nil {
		return rollback(err)
	}
	return nil
}

func (r *sessionRepo) UpdateChatMessageStatus(ctx context.Context, sessionID, messageID, status, errorMessage string) error {
	sessionID = strings.TrimSpace(sessionID)
	messageID = strings.TrimSpace(messageID)
	status = strings.TrimSpace(status)
	if sessionID == "" || messageID == "" {
		return kerrors.BadRequest("SESSION", "session_id and message_id are required")
	}
	if status == "" {
		return kerrors.BadRequest("SESSION", "status is required")
	}
	_, err := r.data.entClient.Message.Update().
		Where(message.IDEQ(messageID), message.SessionIDEQ(sessionID)).
		SetStatus(status).
		SetErrorMessage(strings.TrimSpace(errorMessage)).
		Save(ctx)
	return err
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

func (r *sessionRepo) ListMessagesAfterRevision(ctx context.Context, sessionID string, afterRevision int64) ([]biz.ChatMessage, error) {
	if afterRevision <= 0 {
		return r.ListMessagesBySession(ctx, sessionID, biz.MessageListMaxLimit, 0)
	}
	return r.ListMessagesAfterTurn(ctx, sessionID, int(afterRevision))
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
