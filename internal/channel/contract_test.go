package channel_test

import (
	"testing"

	"aranea-agents/internal/channel"
	"aranea-agents/internal/channel/lark"
	"aranea-agents/internal/channel/slack"
	"aranea-agents/internal/channel/telegram"
)

// Compile-time check: outbound adapters match openclaw-style OutboundText split.
var (
	_ channel.OutboundText = (*lark.FeishuTextSender)(nil)
	_ channel.OutboundText = (*slack.TextSender)(nil)
	_ channel.OutboundText = (*telegram.TextSender)(nil)
)

func TestFeishuTextSenderID(t *testing.T) {
	s := &lark.FeishuTextSender{}
	if s.ID() != "feishu" {
		t.Fatalf("id: got %q", s.ID())
	}
}

func TestSlackTextSenderID(t *testing.T) {
	if (&slack.TextSender{}).ID() != "slack" {
		t.Fatal("expected slack id")
	}
}

func TestTelegramTextSenderID(t *testing.T) {
	if (&telegram.TextSender{}).ID() != "telegram" {
		t.Fatal("expected telegram id")
	}
}
