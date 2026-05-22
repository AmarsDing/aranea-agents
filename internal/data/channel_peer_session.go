package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformchannelpeersession"
)

type channelPeerSessionRepo struct {
	data *Data
}

// NewChannelPeerSessionRepo implements biz.ChannelPeerSessionRepo.
func NewChannelPeerSessionRepo(d *Data) biz.ChannelPeerSessionRepo {
	return &channelPeerSessionRepo{data: d}
}

func entPeerToBiz(e *ent.PlatformChannelPeerSession) biz.ChannelPeerSession {
	if e == nil {
		return biz.ChannelPeerSession{}
	}
	return biz.ChannelPeerSession{
		ID:        e.ID,
		ChannelID: e.ChannelID,
		PeerKey:   e.PeerKey,
		SessionID: e.SessionID,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}

func (r *channelPeerSessionRepo) GetByChannelAndPeer(ctx context.Context, channelID, peerKey string) (biz.ChannelPeerSession, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return biz.ChannelPeerSession{}, sql.ErrNoRows
	}
	e, err := r.data.entClient.PlatformChannelPeerSession.Query().
		Where(
			platformchannelpeersession.ChannelIDEQ(channelID),
			platformchannelpeersession.PeerKeyEQ(peerKey),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.ChannelPeerSession{}, sql.ErrNoRows
		}
		return biz.ChannelPeerSession{}, err
	}
	return entPeerToBiz(e), nil
}

func (r *channelPeerSessionRepo) Create(ctx context.Context, row biz.ChannelPeerSession) (biz.ChannelPeerSession, error) {
	if strings.TrimSpace(row.ID) == "" || strings.TrimSpace(row.ChannelID) == "" || strings.TrimSpace(row.SessionID) == "" {
		return biz.ChannelPeerSession{}, fmt.Errorf("channel peer session: missing id, channel_id, or session_id")
	}
	now := nowRFC3339()
	if row.CreatedAt == "" {
		row.CreatedAt = now
	}
	row.UpdatedAt = now
	e, err := r.data.entClient.PlatformChannelPeerSession.Create().
		SetID(row.ID).
		SetChannelID(row.ChannelID).
		SetPeerKey(row.PeerKey).
		SetSessionID(row.SessionID).
		SetCreatedAt(row.CreatedAt).
		SetUpdatedAt(row.UpdatedAt).
		Save(ctx)
	if err != nil {
		return biz.ChannelPeerSession{}, err
	}
	return entPeerToBiz(e), nil
}

func (r *channelPeerSessionRepo) DeleteByChannelID(ctx context.Context, channelID string) (int, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return 0, nil
	}
	n, err := r.data.entClient.PlatformChannelPeerSession.Delete().
		Where(platformchannelpeersession.ChannelIDEQ(channelID)).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	return n, nil
}
