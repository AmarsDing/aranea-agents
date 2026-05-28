package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	entsessionturn "aranea-agents/internal/data/ent/sessionturn"

	kerrors "github.com/go-kratos/kratos/v2/errors"
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
		CreatedAt:           e.CreatedAt,
		UpdatedAt:           e.UpdatedAt,
	}
}

func (r *sessionRepo) CreateSessionTurn(ctx context.Context, turn biz.SessionTurn) (biz.SessionTurn, error) {
	c := r.data.entClient
	saved, err := c.SessionTurn.Create().
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
		SetCreatedAt(turn.CreatedAt).
		SetUpdatedAt(turn.UpdatedAt).
		Save(ctx)
	if err != nil {
		return biz.SessionTurn{}, err
	}
	return entSessionTurnToBiz(saved), nil
}

func (r *sessionRepo) UpdateSessionTurn(ctx context.Context, id string, fields biz.SessionTurnUpdateFields) (biz.SessionTurn, error) {
	c := r.data.entClient
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
		return biz.SessionTurn{}, err
	}
	return entSessionTurnToBiz(saved), nil
}

func (r *sessionRepo) ListSessionTurns(ctx context.Context, sessionID string, limit, offset int) (biz.SessionTurnListResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return biz.SessionTurnListResult{}, kerrors.BadRequest("SESSION_TURN", "session_id is required")
	}
	c := r.data.entClient
	where := entsessionturn.SessionIDEQ(sessionID)
	total, err := c.SessionTurn.Query().Where(where).Count(ctx)
	if err != nil {
		return biz.SessionTurnListResult{}, err
	}
	rows, err := c.SessionTurn.Query().
		Where(where).
		Order(ent.Asc(entsessionturn.FieldTurnNumber)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return biz.SessionTurnListResult{}, err
	}
	items := make([]biz.SessionTurn, 0, len(rows))
	for _, r := range rows {
		items = append(items, entSessionTurnToBiz(r))
	}
	return biz.SessionTurnListResult{Items: items, Total: total}, nil
}

func (r *sessionRepo) GetSessionTurn(ctx context.Context, id string) (biz.SessionTurn, error) {
	c := r.data.entClient
	row, err := c.SessionTurn.Get(ctx, id)
	if err != nil {
		return biz.SessionTurn{}, err
	}
	return entSessionTurnToBiz(row), nil
}
