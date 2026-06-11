package service

import (
	"testing"

	"aranea-agents/internal/biz"
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
		got := biz.InboundEventIsGroup(tc.ev.OutboundMeta)
		if got != tc.want {
			t.Errorf("InboundEventIsGroup(%+v) = %v, want %v", tc.ev.OutboundMeta, got, tc.want)
		}
	}
}
