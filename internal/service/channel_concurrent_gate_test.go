package service

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

func TestChannelConcurrentGate_limitsGroup(t *testing.T) {
	g := newChannelConcurrentGate()
	ch := biz.Channel{ID: "ch-1"}
	ev := port.InboundEvent{OutboundMeta: map[string]string{"chat_type": "group"}}
	lt := biz.ChannelLongTaskConfig{SessionMaxConcurrentGroup: 2}

	release1, ok := tryAcquireForTest(g, ch, ev, lt)
	if !ok {
		t.Fatal("first acquire")
	}
	release2, ok := tryAcquireForTest(g, ch, ev, lt)
	if !ok {
		t.Fatal("second acquire")
	}
	_, ok = tryAcquireForTest(g, ch, ev, lt)
	if ok {
		t.Fatal("third acquire should fail")
	}
	release1()
	release2()
	_, ok = tryAcquireForTest(g, ch, ev, lt)
	if !ok {
		t.Fatal("should acquire after release")
	}
}

func tryAcquireForTest(g *channelConcurrentGate, ch biz.Channel, ev port.InboundEvent, lt biz.ChannelLongTaskConfig) (func(), bool) {
	h := &ChannelIngress{concurrentGate: g}
	return h.tryAcquireChannelConcurrent(ch, ev, lt)
}

func TestInboundEventIsGroup(t *testing.T) {
	if !inboundEventIsGroup(port.InboundEvent{OutboundMeta: map[string]string{"chat_type": "group"}}) {
		t.Fatal("group chat")
	}
	if inboundEventIsGroup(port.InboundEvent{OutboundMeta: map[string]string{"chat_type": "p2p"}}) {
		t.Fatal("p2p")
	}
}

func TestChannelBusyInputFollowup(t *testing.T) {
	cfg := `{"config":{"busy_input_mode":"followup"}}`
	if !biz.ChannelBusyInputFollowup(cfg) {
		t.Fatal("followup mode")
	}
	if !biz.ChannelBusyInputQueue(cfg) {
		t.Fatal("followup should allow queue/steer")
	}
}

func TestMaxConcurrentInboundDefaults(t *testing.T) {
	lt := biz.ParseChannelLongTaskConfig(`{}`)
	if lt.MaxConcurrentInbound(false) != 1 {
		t.Fatalf("dm default=%d", lt.MaxConcurrentInbound(false))
	}
	if lt.MaxConcurrentInbound(true) != 3 {
		t.Fatalf("group default=%d", lt.MaxConcurrentInbound(true))
	}
}
