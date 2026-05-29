package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/lark"
)

func TestResolvePeerSessionID_prefersOperatorOpenID(t *testing.T) {
	const (
		channelID = "ch-feishu"
		agentID   = "agent-1"
		openID    = "ou_user"
		chatID    = "oc_dm"
		sessionID = "sess-bound"
	)
	peerRepo := &ingressPeerSessionRepo{
		byKey: map[string]biz.ChannelPeerSession{
			peerMapKey(channelID, openID): {
				ChannelID: channelID,
				PeerKey:   openID,
				SessionID: sessionID,
			},
		},
	}
	sessRepo := &ingressSessionRepo{
		sessions: map[string]biz.Session{
			sessionID: {ID: sessionID, AgentID: agentID, OwnerType: "agent"},
		},
	}
	agents := ingressAgentRepo{id: agentID}
	h := &ChannelIngress{
		channels: biz.NewChannelUsecase(nil, peerRepo, nil, agents, nil, nil),
		sessions: biz.NewSessionUsecase(sessRepo, biz.NewSessionAgentLookup(agents), nil, nil, nil),
	}
	ch := biz.Channel{
		ID:         channelID,
		ConfigJSON: `{"type":"feishu","routing":{"default_agent_id":"` + agentID + `"}}`,
	}
	action := lark.CardActionPayload{
		OperatorOpenID: openID,
		OpenChatID:     chatID,
		SessionID:      sessionID,
	}
	got, ok := h.resolvePeerSessionID(context.Background(), ch, action)
	if !ok || got != sessionID {
		t.Fatalf("resolvePeerSessionID = (%q, %v), want (%q, true)", got, ok, sessionID)
	}
}

func TestResolvePeerSessionID_deniedWhenOnlyChatIDBindMissing(t *testing.T) {
	const (
		channelID = "ch-feishu"
		agentID   = "agent-1"
		openID    = "ou_user"
		chatID    = "oc_dm"
		sessionID = "sess-bound"
	)
	peerRepo := &ingressPeerSessionRepo{
		byKey: map[string]biz.ChannelPeerSession{
			peerMapKey(channelID, openID): {
				ChannelID: channelID,
				PeerKey:   openID,
				SessionID: sessionID,
			},
		},
	}
	sessRepo := &ingressSessionRepo{
		sessions: map[string]biz.Session{
			sessionID: {ID: sessionID, AgentID: agentID, OwnerType: "agent"},
		},
	}
	agents := ingressAgentRepo{id: agentID}
	h := &ChannelIngress{
		channels: biz.NewChannelUsecase(nil, peerRepo, nil, agents, nil, nil),
		sessions: biz.NewSessionUsecase(sessRepo, biz.NewSessionAgentLookup(agents), nil, nil, nil),
	}
	ch := biz.Channel{
		ID:         channelID,
		ConfigJSON: `{"type":"feishu","routing":{"default_agent_id":"` + agentID + `"}}`,
	}
	// Card only carried chat_id (wrong order before fix) — should still resolve via operator.
	action := lark.CardActionPayload{
		OperatorOpenID: openID,
		OpenChatID:     chatID,
	}
	got, ok := h.resolveCardActionSessionID(context.Background(), ch, action)
	if !ok || got != sessionID {
		t.Fatalf("resolveCardActionSessionID = (%q, %v), want (%q, true)", got, ok, sessionID)
	}
}
