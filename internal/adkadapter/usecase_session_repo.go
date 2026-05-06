package adkadapter

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
)

// UsecaseSessionRepo adapts *biz.SessionUsecase to [SessionRepositorySubset].
type UsecaseSessionRepo struct {
	UC *biz.SessionUsecase
}

func (r UsecaseSessionRepo) GetSessionByID(ctx context.Context, id string) (biz.Session, error) {
	if r.UC == nil {
		return biz.Session{}, fmt.Errorf("adkadapter: nil session usecase")
	}
	return r.UC.Get(ctx, id)
}

func (r UsecaseSessionRepo) UpdateAdkSnapshotJSON(ctx context.Context, sessionID string, snapshotJSON string) error {
	if r.UC == nil {
		return nil
	}
	return r.UC.UpdateAdkSnapshotJSON(ctx, sessionID, snapshotJSON)
}

func (r UsecaseSessionRepo) ListMessagesBySession(ctx context.Context, sessionID string) ([]biz.ChatMessage, error) {
	if r.UC == nil {
		return nil, nil
	}
	return r.UC.ListMessages(ctx, sessionID)
}
