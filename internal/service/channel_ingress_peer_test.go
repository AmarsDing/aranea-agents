package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

func TestInboundPeerKey_threadSession(t *testing.T) {
	h := &ChannelIngress{}
	ch := biz.Channel{
		ConfigJSON: `{"type":"feishu","routing":{"dm_scope":"per-channel-peer"},"config":{"thread_sessions_per_user":true}}`,
	}
	ev := port.InboundEvent{
		PeerID: "ou_user",
		OutboundMeta: map[string]string{
			"chat_id":   "oc_group",
			"thread_id": "omt_thread",
		},
	}
	key, err := h.inboundPeerKey(ch, ev)
	if err != nil {
		t.Fatal(err)
	}
	if key != "oc_group:omt_thread" {
		t.Fatalf("peer key: got %q", key)
	}
}

func TestWasActiveBeforeTurn_interruptSkipsQueued(t *testing.T) {
	h := &ChannelIngress{}
	ch := biz.Channel{ConfigJSON: `{"config":{"busy_input_mode":"interrupt"}}`}
	if h.wasActiveBeforeTurn(context.Background(), ch, "sess", true) {
		t.Fatal("interrupted turn must not report wasActive")
	}
}
