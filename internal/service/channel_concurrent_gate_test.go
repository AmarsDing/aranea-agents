package service

import (
	"testing"

	"aranea-agents/internal/channel/port"
)

func TestInboundEventIsGroup(t *testing.T) {
	cases := []struct {
		ev   port.InboundEvent
		want bool
	}{
		{port.InboundEvent{OutboundMeta: map[string]string{"chat_type": "group"}}, true},
		{port.InboundEvent{OutboundMeta: map[string]string{"chat_type": "supergroup"}}, true},
		{port.InboundEvent{OutboundMeta: map[string]string{"chat_type": "GROUP"}}, true},
		{port.InboundEvent{OutboundMeta: map[string]string{"chat_type": "  group  "}}, true},
		{port.InboundEvent{OutboundMeta: map[string]string{"chat_type": "private"}}, false},
		{port.InboundEvent{OutboundMeta: map[string]string{"group_id": "g123"}}, true},
		{port.InboundEvent{OutboundMeta: map[string]string{"group_id": "  "}}, false},
		{port.InboundEvent{OutboundMeta: nil}, false},
		{port.InboundEvent{}, false},
	}
	for _, tc := range cases {
		got := inboundEventIsGroup(tc.ev)
		if got != tc.want {
			t.Errorf("inboundEventIsGroup(%+v) = %v, want %v", tc.ev, got, tc.want)
		}
	}
}

func TestChannelConcurrentGate_TryAcquireRelease(t *testing.T) {
	g := newChannelConcurrentGate()
	defer g.Close()

	if !g.TryAcquire("ch1", "peer1", false, 2) {
		t.Fatal("first acquire should succeed")
	}
	if !g.TryAcquire("ch1", "peer1", false, 2) {
		t.Fatal("second acquire should succeed (limit=2)")
	}
	if g.TryAcquire("ch1", "peer1", false, 2) {
		t.Fatal("third acquire should fail (limit=2)")
	}

	g.Release("ch1", "peer1", false)
	if !g.TryAcquire("ch1", "peer1", false, 2) {
		t.Fatal("acquire after release should succeed")
	}
}

func TestChannelConcurrentGate_NilReceiver(t *testing.T) {
	var g *channelConcurrentGate
	if !g.TryAcquire("ch1", "peer1", false, 1) {
		t.Fatal("nil gate should always allow")
	}
	g.Release("ch1", "peer1", false)
}

func TestChannelConcurrentGate_ZeroLimit(t *testing.T) {
	g := newChannelConcurrentGate()
	defer g.Close()
	if !g.TryAcquire("ch1", "peer1", false, 0) {
		t.Fatal("zero limit should always allow")
	}
	if !g.TryAcquire("ch1", "peer1", false, -1) {
		t.Fatal("negative limit should always allow")
	}
}

func TestChannelConcurrentGate_DifferentPeers(t *testing.T) {
	g := newChannelConcurrentGate()
	defer g.Close()
	if !g.TryAcquire("ch1", "peer1", false, 1) {
		t.Fatal("peer1 should acquire")
	}
	if !g.TryAcquire("ch1", "peer2", false, 1) {
		t.Fatal("peer2 should acquire independently")
	}
}

func TestChannelConcurrentGate_ReleaseNonExistent(t *testing.T) {
	g := newChannelConcurrentGate()
	defer g.Close()
	g.Release("nonexistent", "peer", false)
}
