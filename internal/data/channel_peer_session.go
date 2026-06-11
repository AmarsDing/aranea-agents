package data

import (
	"context"
	"database/sql"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformchannelpeersession"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
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
	e, err := r.data.RW().Read(ctx).PlatformChannelPeerSession.Query().
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
		return biz.ChannelPeerSession{}, apierror.BadRequest("CHANNEL_PEER_SESSION", "missing channel_id or session_id")
	}
	var result biz.ChannelPeerSession
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		n, err := r.data.RW().Write(txCtx).PlatformChannelPeerSession.Update().
			Where(
				platformchannelpeersession.ChannelIDEQ(channelID),
				platformchannelpeersession.PeerKeyEQ(peerKey),
			).
			SetSessionID(sessionID).
			SetUpdatedAt(nowRFC3339()).
			Save(txCtx)
		if err != nil {
			return err
		}
		if n == 0 {
			return sql.ErrNoRows
		}
		e, err := r.data.RW().Read(txCtx).PlatformChannelPeerSession.Query().
			Where(
				platformchannelpeersession.ChannelIDEQ(channelID),
				platformchannelpeersession.PeerKeyEQ(peerKey),
			).
			Only(txCtx)
		if err != nil {
			return err
		}
		result = entPeerToBiz(e)
		return nil
	})
	if err != nil {
		return biz.ChannelPeerSession{}, err
	}
	return result, nil
}

func (r *channelPeerSessionRepo) Create(ctx context.Context, row biz.ChannelPeerSession) (biz.ChannelPeerSession, error) {
	if strings.TrimSpace(row.ID) == "" || strings.TrimSpace(row.ChannelID) == "" || strings.TrimSpace(row.SessionID) == "" {
		return biz.ChannelPeerSession{}, apierror.BadRequest("CHANNEL_PEER_SESSION", "missing id, channel_id, or session_id")
	}
	now := nowRFC3339()
	if row.CreatedAt == "" {
		row.CreatedAt = now
	}
	row.UpdatedAt = now
	e, err := r.data.RW().Write(ctx).PlatformChannelPeerSession.Create().
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
	n, err := r.data.RW().Write(ctx).PlatformChannelPeerSession.Delete().
		Where(platformchannelpeersession.ChannelIDEQ(channelID)).
		Exec(ctx)
	if err != nil {
		r.data.lg.Warn("delete channel peer sessions by channel failed", loggateway.StepID("data.channel_peer_session.delete_by_channel"), loggateway.Err(err))
		return 0, err
	}
	return n, nil
}

func (r *channelPeerSessionRepo) DeleteBySessionID(ctx context.Context, sessionID string) (int, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, nil
	}
	n, err := r.data.RW().Write(ctx).PlatformChannelPeerSession.Delete().
		Where(platformchannelpeersession.SessionIDEQ(sessionID)).
		Exec(ctx)
	if err != nil {
		r.data.lg.Warn("delete channel peer sessions by session failed", loggateway.StepID("data.channel_peer_session.delete_by_session"), loggateway.Err(err))
		return 0, err
	}
	return n, nil
}
