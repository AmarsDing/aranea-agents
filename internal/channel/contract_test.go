package channel_test

import (
	"testing"

	"aranea-agents/internal/channel"
	"aranea-agents/internal/channel/lark"
)

// Compile-time check: Feishu outbound matches openclaw-style OutboundText split.
var _ channel.OutboundText = (*lark.FeishuTextSender)(nil)

func TestFeishuTextSenderID(t *testing.T) {
	s := &lark.FeishuTextSender{}
	if s.ID() != "feishu" {
		t.Fatalf("id: got %q", s.ID())
	}
}
