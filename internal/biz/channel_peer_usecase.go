package biz

import (
	"context"
	"strings"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ChannelPeerUsecase manages channel peer sessions and inbound receipt deduplication.
// Extracted from ChannelUsecase to reduce field count and clarify responsibility boundaries.
type ChannelPeerUsecase struct {
	peers           ChannelPeerSessionRepo
	inboundReceipts ChannelInboundReceiptRepo
	lg              loggateway.Logger
}

// NewChannelPeerUsecase creates a ChannelPeerUsecase.
func NewChannelPeerUsecase(
	peers ChannelPeerSessionRepo,
	inboundReceipts ChannelInboundReceiptRepo,
	lg loggateway.Logger,
) *ChannelPeerUsecase {
	return &ChannelPeerUsecase{peers: peers, inboundReceipts: inboundReceipts, lg: lg}
}

// DeletePeerBindingsByChannelID removes all peer bindings for a channel.
func (u *ChannelPeerUsecase) DeletePeerBindingsByChannelID(ctx context.Context, channelID string) (int, error) {
	if u == nil || u.peers == nil {
		return 0, nil
	}
	return u.peers.DeleteByChannelID(ctx, channelID)
}

// GetPeerSession returns the peer session for a channel+peerKey pair.
func (u *ChannelPeerUsecase) GetPeerSession(ctx context.Context, channelID, peerKey string) (ChannelPeerSession, error) {
	if u == nil || u.peers == nil {
		return ChannelPeerSession{}, shared.ErrNotFound
	}
	return u.peers.GetByChannelAndPeer(ctx, channelID, peerKey)
}

// CreatePeerSession creates a new peer session binding.
func (u *ChannelPeerUsecase) CreatePeerSession(ctx context.Context, row ChannelPeerSession) (ChannelPeerSession, error) {
	if u == nil || u.peers == nil {
		return ChannelPeerSession{}, apierror.Internal("CHANNEL", "peer session repository not configured")
	}
	return u.peers.Create(ctx, row)
}

// UpdatePeerSessionID rebinds an existing peer mapping to a new session.
func (u *ChannelPeerUsecase) UpdatePeerSessionID(ctx context.Context, channelID, peerKey, sessionID string) (ChannelPeerSession, error) {
	if u == nil || u.peers == nil {
		return ChannelPeerSession{}, apierror.Internal("CHANNEL", "peer session repository not configured")
	}
	return u.peers.UpdateSessionID(ctx, channelID, peerKey, sessionID)
}

// TryClaimInbound records idempotency before running an agent turn.
func (u *ChannelPeerUsecase) TryClaimInbound(ctx context.Context, channelID, platform, messageKey, peerID, text string) (bool, error) {
	if u == nil || u.inboundReceipts == nil {
		return true, nil
	}
	return TryClaimInbound(ctx, u.inboundReceipts, channelID, platform, messageKey, peerID, text)
}

// DeletePeerBindingsBySessionID removes peer bindings pointing at a deleted session.
func (u *ChannelPeerUsecase) DeletePeerBindingsBySessionID(ctx context.Context, sessionID string) (int, error) {
	if u == nil || u.peers == nil {
		return 0, nil
	}
	return u.peers.DeleteBySessionID(ctx, sessionID)
}

// ResolvePeerSession returns the existing peer session or creates a new one.
// This is a convenience method for the common get-or-create pattern in ingress.
func (u *ChannelPeerUsecase) ResolvePeerSession(ctx context.Context, channelID, peerKey string, sessionID string) (ChannelPeerSession, error) {
	if u == nil || u.peers == nil {
		return ChannelPeerSession{}, shared.ErrNotFound
	}
	peerKey = strings.TrimSpace(peerKey)
	if peerKey == "" {
		return ChannelPeerSession{}, apierror.BadRequest("CHANNEL", "peer_key is required")
	}
	existing, err := u.peers.GetByChannelAndPeer(ctx, channelID, peerKey)
	if err == nil {
		return existing, nil
	}
	return ChannelPeerSession{}, err
}
