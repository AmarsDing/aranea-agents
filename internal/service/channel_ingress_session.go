package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	arametrics "aranea-agents/internal/metrics"

	"github.com/google/uuid"
)

func platformFromTitlePrefix(titlePrefix string) string {
	return strings.TrimSpace(strings.Split(strings.TrimSpace(titlePrefix), ":")[0])
}

func (h *ChannelIngress) createChannelSession(
	ctx context.Context,
	chRow biz.Channel,
	titlePrefix string,
	platform string,
	peerKey string,
	peerID string,
	ownerType string,
	agentID string,
	teamID string,
) (string, error) {
	title := titlePrefix + ":" + strings.TrimSpace(chRow.Key)
	if pk := strings.TrimSpace(peerKey); pk != "" {
		title += ":" + pk
	}
	created, err := h.sessions.Create(ctx, biz.Session{
		OwnerType:    ownerType,
		AgentID:      agentID,
		TeamID:       teamID,
		Title:        title,
		MetadataJSON: biz.BuildChannelSessionMetadataJSON(chRow, platform, peerID, peerKey),
	})
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

func (h *ChannelIngress) ensureChannelSession(
	ctx context.Context,
	chRow biz.Channel,
	titlePrefix string,
	platform string,
	peerKey string,
	peerID string,
	routing biz.ChannelRouting,
) (string, error) {
	ownerType, agentID, teamID, err := biz.ResolveChannelTarget(ctx, h.agents, h.teams, routing, peerID)
	if err != nil {
		return "", err
	}
	platform = strings.TrimSpace(platform)

	bind, err := h.peers.GetByChannelAndPeer(ctx, chRow.ID, peerKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if err == nil {
		if sessionID := strings.TrimSpace(bind.SessionID); sessionID != "" {
			if _, verr := h.sessions.Get(ctx, sessionID); verr == nil {
				return sessionID, nil
			} else if !errors.Is(verr, sql.ErrNoRows) {
				return "", verr
			}
			sessionID, cerr := h.createChannelSession(ctx, chRow, titlePrefix, platform, peerKey, peerID, ownerType, agentID, teamID)
			if cerr != nil {
				return "", cerr
			}
			if _, uerr := h.peers.UpdateSessionID(ctx, chRow.ID, peerKey, sessionID); uerr != nil {
				return "", uerr
			}
			return sessionID, nil
		}
	}

	sessionID, cerr := h.createChannelSession(ctx, chRow, titlePrefix, platform, peerKey, peerID, ownerType, agentID, teamID)
	if cerr != nil {
		return "", cerr
	}
	if _, cerr = h.peers.Create(ctx, biz.ChannelPeerSession{
		ID:        uuid.NewString(),
		ChannelID: chRow.ID,
		PeerKey:   peerKey,
		SessionID: sessionID,
	}); cerr != nil {
		return "", cerr
	}
	return sessionID, nil
}

func (h *ChannelIngress) prepareChannelChatRequest(
	ctx context.Context,
	chRow biz.Channel,
	titlePrefix string,
	peerKey string,
	peerID string,
	content string,
) (*chatv1.SendChatMessageRequest, error) {
	routing, err := biz.ParseChannelRouting(chRow.ConfigJSON)
	if err != nil {
		return nil, err
	}
	sessionID, err := h.ensureChannelSession(ctx, chRow, titlePrefix, platformFromTitlePrefix(titlePrefix), peerKey, peerID, routing)
	if err != nil {
		return nil, err
	}
	req := &chatv1.SendChatMessageRequest{SessionId: sessionID, Content: content}
	ownerType, _, teamID, err := biz.ResolveChannelTarget(ctx, h.agents, h.teams, routing, peerID)
	if err != nil {
		return nil, err
	}
	if ownerType == "team" && teamID != "" {
		tid := teamID
		req.TeamId = &tid
	}
	return req, nil
}

func channelChatRequestToTurnInput(req *chatv1.SendChatMessageRequest) biz.TurnInput {
	if req == nil {
		return biz.TurnInput{}
	}
	input := biz.TurnInput{
		SessionID: strings.TrimSpace(req.GetSessionId()),
		Content:   strings.TrimSpace(req.GetContent()),
	}
	if tid := strings.TrimSpace(req.GetTeamId()); tid != "" {
		input.TeamID = tid
	}
	return input
}

func recordStreamUpdate(platform, phase string, err error) {
	result := "ok"
	if err != nil {
		result = "error"
	}
	arametrics.ChannelStreamUpdateTotal.WithLabelValues(strings.ToLower(strings.TrimSpace(platform)), phase, result).Inc()
}

func (h *ChannelIngress) resolveInboundSessionID(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string) string {
	if sid := strings.TrimSpace(ev.OutboundMeta["session_id"]); sid != "" {
		return sid
	}
	peerKey, err := h.inboundPeerKey(chRow, ev)
	if err != nil {
		return ""
	}
	req, err := h.prepareChannelChatRequest(ctx, chRow, platform, peerKey, ev.PeerID, ev.Text)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(req.GetSessionId())
}

func (h *ChannelIngress) shouldSkipTurnErrorReply(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string, execErr error) bool {
	if h == nil || h.chat == nil || execErr == nil {
		return false
	}
	if turnErrorIsCanceled(execErr) {
		return true
	}
	if TurnErrorCodeFromErr(execErr) != TurnErrTurnTimeout && TurnErrorCodeFromErr(execErr) != TurnErrFirstByteTimeout && !turnErrorIsTimeout(execErr) {
		return false
	}
	sessionID := h.resolveInboundSessionID(ctx, chRow, ev, platform)
	if sessionID == "" {
		return false
	}
	phase := h.chat.ActiveSessionRunPhase(ctx, sessionID)
	switch phase {
	case biz.SessionRunPhaseDurable, biz.SessionRunPhaseEscalating:
		return true
	default:
	}
	if h.chat.HasActiveRun(sessionID) {
		return true
	}
	return false
}
