package biz

import (
	"context"
)

// ChannelPeerSession binds an external channel peer to an internal chat session.
type ChannelPeerSession struct {
	ID        string
	ChannelID string
	PeerKey   string
	SessionID string
	CreatedAt string
	UpdatedAt string
}

// ChannelPeerSessionRepo persists channel_id + peer_key → session_id mappings.
type ChannelPeerSessionRepo interface {
	GetByChannelAndPeer(ctx context.Context, channelID, peerKey string) (ChannelPeerSession, error)
	Create(ctx context.Context, row ChannelPeerSession) (ChannelPeerSession, error)
	// DeleteByChannelID removes all peer bindings for a channel (e.g. after routing change).
	DeleteByChannelID(ctx context.Context, channelID string) (int, error)
}
