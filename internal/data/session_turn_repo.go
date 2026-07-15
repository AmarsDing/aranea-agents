package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	entsessionturn "aranea-agents/internal/data/ent/sessionturn"
	"aranea-agents/pkg/apierror"
)

func entSessionTurnToBiz(e *ent.SessionTurn) biz.SessionTurn {
	if e == nil {
		return biz.SessionTurn{}
	}
	return biz.SessionTurn{
		ID:                  e.ID,
		SessionID:           e.SessionID,
		RunID:               e.RunID,
		TurnNumber:          e.TurnNumber,
		UserMessageID:       e.UserMessageID,
		AssistantMessageID:  e.AssistantMessageID,
		OwnerType:           e.OwnerType,
		AgentID:             e.AgentID,
		TeamID:              e.TeamID,
		Status:              e.Status,
		StartedAt:           e.StartedAt,
		EndedAt:             e.EndedAt,
		DurationMs:          e.DurationMs,
		FirstTokenMs:        e.FirstTokenMs,
		ModelCallCount:      e.ModelCallCount,
		ToolCallCount:       e.ToolCallCount,
		SkillCallCount:      e.SkillCallCount,
		MCPCallCount:        e.McpCallCount,
		InputTokens:         e.InputTokens,
		OutputTokens:        e.OutputTokens,
		TotalTokens:         e.TotalTokens,
		TotalCostMicroUSD:   e.TotalCostMicroUsd,
		FinalProvider:       e.FinalProvider,
		FinalModel:          e.FinalModel,
		FinalContentPreview: e.FinalContentPreview,
		ErrorCode:           e.ErrorCode,
		ErrorMessage:        e.ErrorMessage,
		MetadataJSON:        e.MetadataJSON,
		IdempotencyKey:      e.IdempotencyKey,
		CreatedAt:           e.CreatedAt,
		UpdatedAt:           e.UpdatedAt,
	}
}

func (r *sessionRepo) CreateSessionTurn(ctx context.Context, turn biz.SessionTurn) (biz.SessionTurn, error) {
	var saved biz.SessionTurn
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		c := EntClientFromCtx(txCtx, r.data.entClient)
		if turn.TurnNumber <= 0 {
			maxTurnNumber, err := nextSessionTurnNumber(txCtx, c, r.data.Dialect(), turn.SessionID)
			if err != nil {
				return err
			}
			turn.TurnNumber = maxTurnNumber
		}
		idemKey := strings.TrimSpace(turn.IdempotencyKey)
		if idemKey == "" {
			// C-13: empty client keys become id-scoped sentinels so the unique
			// index never collides across independent turns.
			idemKey = "__id__:" + turn.ID
			turn.IdempotencyKey = idemKey
		}
		b := c.SessionTurn.Create().
			SetID(turn.ID).
			SetSessionID(turn.SessionID).
			SetRunID(turn.RunID).
			SetTurnNumber(turn.TurnNumber).
			SetUserMessageID(turn.UserMessageID).
			SetAssistantMessageID(turn.AssistantMessageID).
			SetOwnerType(turn.OwnerType).
			SetAgentID(turn.AgentID).
			SetTeamID(turn.TeamID).
			SetStatus(turn.Status).
			SetStartedAt(turn.StartedAt).
			SetEndedAt(turn.EndedAt).
			SetDurationMs(turn.DurationMs).
			SetFirstTokenMs(turn.FirstTokenMs).
			SetModelCallCount(turn.ModelCallCount).
			SetToolCallCount(turn.ToolCallCount).
			SetSkillCallCount(turn.SkillCallCount).
			SetMcpCallCount(turn.MCPCallCount).
			SetInputTokens(turn.InputTokens).
			SetOutputTokens(turn.OutputTokens).
			SetTotalTokens(turn.TotalTokens).
			SetTotalCostMicroUsd(turn.TotalCostMicroUSD).
			SetFinalProvider(turn.FinalProvider).
			SetFinalModel(turn.FinalModel).
			SetFinalContentPreview(turn.FinalContentPreview).
			SetErrorCode(turn.ErrorCode).
			SetErrorMessage(turn.ErrorMessage).
			SetMetadataJSON(turn.MetadataJSON).
			SetIdempotencyKey(idemKey).
			SetCreatedAt(turn.CreatedAt).
			SetUpdatedAt(turn.UpdatedAt).
			OnConflictColumns(entsessionturn.FieldSessionID, entsessionturn.FieldIdempotencyKey).
			Ignore()

		id, err := b.ID(txCtx)
		if err != nil {
			// Conflict path: load the canonical row by unique key.
			existing, findErr := c.SessionTurn.Query().
				Where(
					entsessionturn.SessionID(turn.SessionID),
					entsessionturn.IdempotencyKey(idemKey),
				).
				Only(txCtx)
			if findErr != nil {
				return err
			}
			saved = entSessionTurnToBiz(existing)
			return nil
		}
		row, findErr := c.SessionTurn.Get(txCtx, id)
		if findErr != nil {
			return findErr
		}
		saved = entSessionTurnToBiz(row)
		return nil
	})
	if err != nil {
		return biz.SessionTurn{}, entErrToBizErr(err, "SESSION_TURN")
	}
	return saved, nil
}

func nextSessionTurnNumber(ctx context.Context, c *ent.Client, d Dialect, sessionID string) (int, error) {
	// Postgres 不允许在含聚合函数的查询上直接使用 FOR UPDATE（错误 0A000），
	// 改用子查询先锁定该 session 的行，再在外层做聚合，达到相同的串行化效果。
	query := `SELECT COALESCE(MAX(turn_number), 0) FROM session_turns WHERE session_id = ?`
	if d.IsPostgres() {
		query = `SELECT COALESCE(MAX(turn_number), 0) FROM (SELECT turn_number FROM session_turns WHERE session_id = ? FOR UPDATE) AS locked_rows`
	}
	query = d.RenumberPlaceholders(query)
	var maxTurnNumber int
	if err := QueryRowScan(ctx, c, query, []any{sessionID}, &maxTurnNumber); err != nil {
		return 0, err
	}
	return maxTurnNumber + 1, nil
}

func (r *sessionRepo) UpdateSessionTurn(ctx context.Context, id string, fields biz.SessionTurnUpdateFields) (biz.SessionTurn, error) {
	c := r.data.RW().Write(ctx)
	upd := c.SessionTurn.UpdateOneID(id)
	if fields.Status != nil {
		upd = upd.SetStatus(*fields.Status)
	}
	if fields.EndedAt != nil {
		upd = upd.SetEndedAt(*fields.EndedAt)
	}
	if fields.UserMessageID != nil {
		upd = upd.SetUserMessageID(*fields.UserMessageID)
	}
	if fields.AssistantMessageID != nil {
		upd = upd.SetAssistantMessageID(*fields.AssistantMessageID)
	}
	if fields.OwnerType != nil {
		upd = upd.SetOwnerType(*fields.OwnerType)
	}
	if fields.AgentID != nil {
		upd = upd.SetAgentID(*fields.AgentID)
	}
	if fields.TeamID != nil {
		upd = upd.SetTeamID(*fields.TeamID)
	}
	if fields.DurationMs != nil {
		upd = upd.SetDurationMs(*fields.DurationMs)
	}
	if fields.FirstTokenMs != nil {
		upd = upd.SetFirstTokenMs(*fields.FirstTokenMs)
	}
	if fields.ModelCallCount != nil {
		upd = upd.SetModelCallCount(*fields.ModelCallCount)
	}
	if fields.ToolCallCount != nil {
		upd = upd.SetToolCallCount(*fields.ToolCallCount)
	}
	if fields.SkillCallCount != nil {
		upd = upd.SetSkillCallCount(*fields.SkillCallCount)
	}
	if fields.MCPCallCount != nil {
		upd = upd.SetMcpCallCount(*fields.MCPCallCount)
	}
	if fields.InputTokens != nil {
		upd = upd.SetInputTokens(*fields.InputTokens)
	}
	if fields.OutputTokens != nil {
		upd = upd.SetOutputTokens(*fields.OutputTokens)
	}
	if fields.TotalTokens != nil {
		upd = upd.SetTotalTokens(*fields.TotalTokens)
	}
	if fields.TotalCostMicroUSD != nil {
		upd = upd.SetTotalCostMicroUsd(*fields.TotalCostMicroUSD)
	}
	if fields.FinalProvider != nil {
		upd = upd.SetFinalProvider(*fields.FinalProvider)
	}
	if fields.FinalModel != nil {
		upd = upd.SetFinalModel(*fields.FinalModel)
	}
	if fields.FinalContentPreview != nil {
		upd = upd.SetFinalContentPreview(*fields.FinalContentPreview)
	}
	if fields.ErrorCode != nil {
		upd = upd.SetErrorCode(*fields.ErrorCode)
	}
	if fields.ErrorMessage != nil {
		upd = upd.SetErrorMessage(*fields.ErrorMessage)
	}
	if fields.MetadataJSON != nil {
		upd = upd.SetMetadataJSON(*fields.MetadataJSON)
	}
	upd = upd.SetUpdatedAt(nowRFC3339())
	saved, err := upd.Save(ctx)
	if err != nil {
		return biz.SessionTurn{}, entErrToBizErr(err, "SESSION_TURN")
	}
	return entSessionTurnToBiz(saved), nil
}

func (r *sessionRepo) ListSessionTurns(ctx context.Context, sessionID string, limit, offset int) (biz.SessionTurnListResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return biz.SessionTurnListResult{}, apierror.BadRequest("SESSION_TURN", "session_id is required")
	}
	c := r.data.RW().Read(ctx)
	where := entsessionturn.SessionIDEQ(sessionID)
	total, err := c.SessionTurn.Query().Where(where).Count(ctx)
	if err != nil {
		return biz.SessionTurnListResult{}, entErrToBizErr(err, "SESSION_TURN")
	}
	rows, err := c.SessionTurn.Query().
		Where(where).
		Order(ent.Asc(entsessionturn.FieldTurnNumber)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return biz.SessionTurnListResult{}, entErrToBizErr(err, "SESSION_TURN")
	}
	items := make([]biz.SessionTurn, 0, len(rows))
	for _, r := range rows {
		items = append(items, entSessionTurnToBiz(r))
	}
	return biz.SessionTurnListResult{Items: items, Total: total}, nil
}

func (r *sessionRepo) GetSessionTurn(ctx context.Context, id string) (biz.SessionTurn, error) {
	c := r.data.RW().Read(ctx)
	row, err := c.SessionTurn.Get(ctx, id)
	if err != nil {
		return biz.SessionTurn{}, entErrToBizErr(err, "SESSION_TURN")
	}
	return entSessionTurnToBiz(row), nil
}
