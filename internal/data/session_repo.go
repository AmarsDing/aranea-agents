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
	skillinvocationpkg "aranea-agents/internal/data/ent/skillinvocation"
	toolinvocationpkg "aranea-agents/internal/data/ent/toolinvocation"

	entsql "entgo.io/ent/dialect/sql"
	kerrors "github.com/go-kratos/kratos/v2/errors"
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
		RunnerSnapshotJSON:         e.RunnerSnapshotJSON,
		StateJSON:                  e.StateJSON,
		MetadataJSON:               e.MetadataJSON,
	}
}

func contextStatusForRatio(ratio float64) string {
	switch {
	case ratio >= 0.95:
		return "exceeded"
	case ratio >= 0.8:
		return "critical"
	case ratio >= 0.6:
		return "warning"
	default:
		return "normal"
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
		in.ContextStatus = contextStatusForRatio(in.ContextUsedRatio)
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

func (r *sessionRepo) ArchiveSession(ctx context.Context, id string) error {
	c := r.data.entClient
	now := nowRFC3339()
	_, err := c.Session.Update().
		Where(entsession.IDEQ(id), entsession.DeletedAtEQ("")).
		SetStatus("archived").
		SetArchivedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func (r *sessionRepo) DeleteSession(ctx context.Context, id string) error {
	c := r.data.entClient
	now := nowRFC3339()
	_, err := c.Session.Update().
		Where(entsession.IDEQ(id), entsession.DeletedAtEQ("")).
		SetDeletedAt(now).
		SetStatus("deleted").
		SetUpdatedAt(now).
		Save(ctx)
	return err
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

func (r *sessionRepo) ListMessagesBySession(ctx context.Context, sessionID string) ([]biz.ChatMessage, error) {
	c := r.data.entClient
	rows, err := c.Message.Query().
		Where(message.SessionIDEQ(sessionID)).
		Order(message.ByTurnIndex(entsql.OrderAsc()), message.ByCreatedAt(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.ChatMessage, 0, len(rows))
	for _, m := range rows {
		out = append(out, biz.ChatMessage{
			ID:               m.ID,
			SessionID:        m.SessionID,
			ParentMessageID:  m.ParentMessageID,
			TurnIndex:        m.TurnIndex,
			Role:             m.Role,
			ContentMarkdown:  m.ContentMarkdown,
			ModelName:        m.ModelName,
			TokenIn:          m.TokenIn,
			TokenOut:         m.TokenOut,
			LatencyMS:        m.LatencyMs,
			Status:           m.Status,
			AttachmentsCount: m.AttachmentsCount,
			OptionsJSON:      m.OptionsJSON,
			ErrorMessage:     m.ErrorMessage,
			CreatedAt:        m.CreatedAt,
		})
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
	row, err := tx.Message.Query().
		Where(message.SessionIDEQ(sessionID)).
		Order(message.ByTurnIndex(entsql.OrderDesc())).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	return row.TurnIndex, nil
}

func (r *sessionRepo) insertMessageTx(ctx context.Context, tx *ent.Tx, m biz.ChatMessage) error {
	return tx.Message.Create().
		SetID(m.ID).
		SetSessionID(m.SessionID).
		SetParentMessageID(m.ParentMessageID).
		SetTurnIndex(m.TurnIndex).
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
	cur, err := r.GetSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	ratio := cur.ContextUsedRatio
	if contextWindow > 0 && promptTokens > 0 {
		ratio = float64(promptTokens) / float64(contextWindow)
		if ratio > 1 {
			ratio = 1
		}
	}
	maxR := cur.MaxContextUsedRatio
	if ratio > maxR {
		maxR = ratio
	}
	upd := r.data.entClient.Session.Update().
		Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).
		SetContextUsedRatio(ratio).
		SetMaxContextUsedRatio(maxR).
		SetContextStatus(contextStatusForRatio(ratio)).
		SetUpdatedAt(nowRFC3339())
	if contextWindow > 0 {
		upd = upd.SetLastContextWindowTokens(contextWindow)
	}
	if promptTokens > 0 {
		upd = upd.SetContextUsedTokens(promptTokens)
	}
	_, err = upd.Save(ctx)
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
	ratio := float64(tok) / float64(contextWindow)
	if ratio > 1 {
		ratio = 1
	}
	_, err := r.data.entClient.Session.Update().
		Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).
		SetContextUsedTokens(tok).
		SetContextUsedRatio(ratio).
		SetContextStatus(contextStatusForRatio(ratio)).
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
	user.TurnIndex = maxTurn + 1
	assistant.TurnIndex = maxTurn + 2
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
		upd = upd.AddInputTokens(tin).AddOutputTokens(tout).AddTotalTokens(tin + tout).AddContextUsedTokens(tin + tout)
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
	default:
		return []entsession.OrderOption{
			entsession.ByLastMessageAt(entsql.OrderDesc()),
			entsession.ByUpdatedAt(entsql.OrderDesc()),
		}
	}
}

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
		maxTurn, merr := r.maxMessageTurnTx(ctx, tx, sessionID)
		if merr != nil {
			return rollback(merr)
		}
		msg.TurnIndex = maxTurn + 1
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
	if _, err = tx.Message.UpdateOneID(msg.ID).
		SetContentMarkdown(msg.ContentMarkdown).
		SetOptionsJSON(msg.OptionsJSON).
		SetStatus(msg.Status).
		SetLatencyMs(msg.LatencyMS).
		SetErrorMessage(msg.ErrorMessage).
		Save(ctx); err != nil {
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
	maxTurn, err := r.maxMessageTurnTx(ctx, tx, sessionID)
	if err != nil {
		return rollback(err)
	}
	msg.TurnIndex = maxTurn + 1
	if err = r.insertMessageTx(ctx, tx, msg); err != nil {
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
		upd = upd.AddInputTokens(tin).AddOutputTokens(tout).AddTotalTokens(tin + tout).AddContextUsedTokens(tin + tout)
	}
	if _, err = upd.Save(ctx); err != nil {
		return rollback(err)
	}
	if err = tx.Commit(); err != nil {
		return rollback(err)
	}
	return nil
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
