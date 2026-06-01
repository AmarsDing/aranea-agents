package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/toolresultblob"
	"aranea-agents/internal/data/ent/toolresultreplacement"
	"aranea-agents/pkg/loggateway"
)

type ToolResultBlobRepo struct {
	data *Data
}

var _ biz.ToolResultBlobReader = (*ToolResultBlobRepo)(nil)
var _ biz.ToolResultBlobWriter = (*ToolResultBlobRepo)(nil)

func NewToolResultBlobRepo(data *Data) *ToolResultBlobRepo {
	return &ToolResultBlobRepo{data: data}
}

func (r *ToolResultBlobRepo) SaveBlob(ctx context.Context, blob *biz.ToolResultBlob) error {
	client := r.data.Ent()
	_, err := client.ToolResultBlob.Create().
		SetID(blob.ID).
		SetSessionID(blob.SessionID).
		SetTurnNumber(blob.TurnNumber).
		SetToolName(blob.ToolName).
		SetToolArgsSummary(blob.ToolArgsSummary).
		SetFullContent(blob.FullContent).
		SetContentSizeChars(blob.ContentSizeChars).
		SetCreatedAt(blob.CreatedAt).
		Save(ctx)
	if err != nil {
		r.data.lg.Warn("tool result blob save failed", loggateway.StepID("data.tool.blob_save"), loggateway.Err(err))
	}
	return err
}

func (r *ToolResultBlobRepo) GetBlob(ctx context.Context, id string) (*biz.ToolResultBlob, error) {
	client := r.data.Ent()
	row, err := client.ToolResultBlob.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return entToolResultBlobToBiz(row), nil
}

func (r *ToolResultBlobRepo) ListBlobsBySession(ctx context.Context, sessionID string, fromTurn int) ([]*biz.ToolResultBlob, error) {
	client := r.data.Ent()
	rows, err := client.ToolResultBlob.Query().
		Where(
			toolresultblob.SessionIDEQ(sessionID),
			toolresultblob.TurnNumberGTE(fromTurn),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*biz.ToolResultBlob, len(rows))
	for i, row := range rows {
		result[i] = entToolResultBlobToBiz(row)
	}
	return result, nil
}

type ToolResultReplacementRepo struct {
	data *Data
}

var _ biz.ToolResultReplacementReader = (*ToolResultReplacementRepo)(nil)
var _ biz.ToolResultReplacementWriter = (*ToolResultReplacementRepo)(nil)

func NewToolResultReplacementRepo(data *Data) *ToolResultReplacementRepo {
	return &ToolResultReplacementRepo{data: data}
}

func (r *ToolResultReplacementRepo) SaveReplacement(ctx context.Context, rep *biz.ToolResultReplacement) error {
	client := r.data.Ent()
	_, err := client.ToolResultReplacement.Create().
		SetID(rep.ID).
		SetSessionID(rep.SessionID).
		SetMessageID(rep.MessageID).
		SetResultBlobID(rep.ResultBlobID).
		SetPreviewText(rep.PreviewText).
		SetReplacedAt(rep.ReplacedAt).
		Save(ctx)
	if err != nil {
		r.data.lg.Warn("tool result replacement save failed", loggateway.StepID("data.tool.replacement_save"), loggateway.Err(err))
	}
	return err
}

func (r *ToolResultReplacementRepo) GetReplacementByMessage(ctx context.Context, sessionID, messageID string) (*biz.ToolResultReplacement, error) {
	client := r.data.Ent()
	row, err := client.ToolResultReplacement.Query().
		Where(
			toolresultreplacement.SessionIDEQ(sessionID),
			toolresultreplacement.MessageIDEQ(messageID),
		).
		Only(ctx)
	if err != nil {
		return nil, err
	}
	return entToolResultReplacementToBiz(row), nil
}

func entToolResultBlobToBiz(row *ent.ToolResultBlob) *biz.ToolResultBlob {
	return &biz.ToolResultBlob{
		ID:               row.ID,
		SessionID:        row.SessionID,
		TurnNumber:       row.TurnNumber,
		ToolName:         row.ToolName,
		ToolArgsSummary:  row.ToolArgsSummary,
		FullContent:      row.FullContent,
		ContentSizeChars: row.ContentSizeChars,
		CreatedAt:        row.CreatedAt,
	}
}

func entToolResultReplacementToBiz(row *ent.ToolResultReplacement) *biz.ToolResultReplacement {
	return &biz.ToolResultReplacement{
		ID:           row.ID,
		SessionID:    row.SessionID,
		MessageID:    row.MessageID,
		ResultBlobID: row.ResultBlobID,
		PreviewText:  row.PreviewText,
		ReplacedAt:   row.ReplacedAt,
	}
}
