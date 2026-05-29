package data

import (
	"context"
	"database/sql"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformchannelpeersession"
	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type channelPeerSessionRepo struct {
	data *Data
}

var _ biz.ChannelPeerSessionRepo = (*channelPeerSessionRepo)(nil)

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

func (r *channelPeerSessionRepo) UpdateSessionID(ctx context.Context, channelID, peerKey, sessionID string) (biz.ChannelPeerSession, error) {
	channelID = strings.TrimSpace(channelID)
	sessionID = strings.TrimSpace(sessionID)
	if channelID == "" || sessionID == "" {
		return biz.ChannelPeerSession{}, kerrors.BadRequest("CHANNEL_PEER_SESSION", "missing channel_id or session_id")
	}
	n, err := r.data.entClient.PlatformChannelPeerSession.Update().
		Where(
			platformchannelpeersession.ChannelIDEQ(channelID),
			platformchannelpeersession.PeerKeyEQ(peerKey),
		).
		SetSessionID(sessionID).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	if err != nil {
		return biz.ChannelPeerSession{}, err
	}
	if n == 0 {
		return biz.ChannelPeerSession{}, sql.ErrNoRows
	}
	return r.GetByChannelAndPeer(ctx, channelID, peerKey)
}

func (r *channelPeerSessionRepo) Create(ctx context.Context, row biz.ChannelPeerSession) (biz.ChannelPeerSession, error) {
	if strings.TrimSpace(row.ID) == "" || strings.TrimSpace(row.ChannelID) == "" || strings.TrimSpace(row.SessionID) == "" {
		return biz.ChannelPeerSession{}, kerrors.BadRequest("CHANNEL_PEER_SESSION", "missing id, channel_id, or session_id")
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

func (r *channelPeerSessionRepo) DeleteBySessionID(ctx context.Context, sessionID string) (int, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, nil
	}
	n, err := r.data.entClient.PlatformChannelPeerSession.Delete().
		Where(platformchannelpeersession.SessionIDEQ(sessionID)).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	return n, nil
}
