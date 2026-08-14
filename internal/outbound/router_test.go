package outbound

import (
	"context"
	"errors"
	"testing"

	ch "aranea-agents/internal/channel"
	"aranea-agents/pkg/loggateway"
)

type stubTextSender struct {
	id      string
	sendErr error
}

func (s *stubTextSender) ID() string                                    { return s.id }
func (s *stubTextSender) Run(_ context.Context) error                   { return nil }
func (s *stubTextSender) SendText(_ context.Context, _, _ string) error { return s.sendErr }

type stubMessageSender struct {
	id      string
	sendErr error
	lastMsg OutboundMessage
}

func (s *stubMessageSender) ID() string                  { return s.id }
func (s *stubMessageSender) Run(_ context.Context) error { return nil }
func (s *stubMessageSender) SendMessage(_ context.Context, _ string, msg OutboundMessage) error {
	s.lastMsg = msg
	return s.sendErr
}

type stubOutboundText struct {
	id      string
	sendErr error
}

func (s *stubOutboundText) ID() string                                    { return s.id }
func (s *stubOutboundText) SendText(_ context.Context, _, _ string) error { return s.sendErr }

var _ ch.OutboundText = (*stubOutboundText)(nil)

func TestNewRouter(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	if r == nil {
		t.Fatal("expected non-nil router")
	}
}

func TestRouter_RegisterTextSender(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	r.RegisterTextSender(&stubTextSender{id: "telegram"})
	ch := r.Channels()
	if len(ch) != 1 || ch[0] != "telegram" {
		t.Fatalf("expected [telegram], got %v", ch)
	}
}

func TestRouter_RegisterTextSender_NilRouter(t *testing.T) {
	var r *Router
	r.RegisterTextSender(&stubTextSender{id: "x"})
}

func TestRouter_RegisterTextSender_NilSender(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	r.RegisterTextSender(nil)
	if len(r.Channels()) != 0 {
		t.Fatal("should not register nil sender")
	}
}

func TestRouter_RegisterTextSender_EmptyID(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	r.RegisterTextSender(&stubTextSender{id: ""})
	if len(r.Channels()) != 0 {
		t.Fatal("should not register sender with empty ID")
	}
}

func TestRouter_RegisterMessageSender(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	r.RegisterMessageSender(&stubMessageSender{id: "slack"})
	ch := r.Channels()
	if len(ch) != 1 || ch[0] != "slack" {
		t.Fatalf("expected [slack], got %v", ch)
	}
}

func TestRouter_RegisterMessageSender_NilRouter(t *testing.T) {
	var r *Router
	r.RegisterMessageSender(&stubMessageSender{id: "x"})
}

func TestRouter_RegisterMessageSender_NilSender(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	r.RegisterMessageSender(nil)
	if len(r.Channels()) != 0 {
		t.Fatal("should not register nil sender")
	}
}

func TestRouter_RegisterMessageSender_EmptyID(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	r.RegisterMessageSender(&stubMessageSender{id: ""})
	if len(r.Channels()) != 0 {
		t.Fatal("should not register sender with empty ID")
	}
}

func TestRouter_RegisterOutboundText(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	r.RegisterOutboundText(&stubOutboundText{id: "feishu"})
	ch := r.Channels()
	if len(ch) != 1 || ch[0] != "feishu" {
		t.Fatalf("expected [feishu], got %v", ch)
	}
}

func TestRouter_RegisterOutboundText_Nil(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	r.RegisterOutboundText(nil)
	if len(r.Channels()) != 0 {
		t.Fatal("should not register nil outbound text")
	}
}

func TestRouter_Channels_NilRouter(t *testing.T) {
	var r *Router
	if ch := r.Channels(); ch != nil {
		t.Fatalf("expected nil, got %v", ch)
	}
}

func TestRouter_Channels_Sorted(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	r.RegisterTextSender(&stubTextSender{id: "zebra"})
	r.RegisterTextSender(&stubTextSender{id: "alpha"})
	ch := r.Channels()
	if ch[0] != "alpha" || ch[1] != "zebra" {
		t.Fatalf("expected sorted, got %v", ch)
	}
}

func TestRouter_Channels_Dedup(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	r.RegisterTextSender(&stubTextSender{id: "telegram"})
	r.RegisterMessageSender(&stubMessageSender{id: "telegram"})
	ch := r.Channels()
	if len(ch) != 1 {
		t.Fatalf("expected 1 (dedup), got %d", len(ch))
	}
}

func TestRouter_SendText(t *testing.T) {
	sender := &stubTextSender{id: "telegram"}
	r := NewRouter(loggateway.NewNoop())
	r.RegisterTextSender(sender)
	err := r.SendText(context.Background(), DeliveryTarget{Channel: "telegram", Target: "chat1"}, "hello")
	if err != nil {
		t.Fatal(err)
	}
}

func TestRouter_SendMessage_NilRouter(t *testing.T) {
	var r *Router
	err := r.SendMessage(context.Background(), DeliveryTarget{Channel: "x", Target: "y"}, OutboundMessage{Text: "hi"})
	if err == nil {
		t.Fatal("expected error for nil router")
	}
}

func TestRouter_SendMessage_EmptyChannel(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	err := r.SendMessage(context.Background(), DeliveryTarget{Channel: "", Target: "y"}, OutboundMessage{Text: "hi"})
	if err == nil {
		t.Fatal("expected error for empty channel")
	}
}

func TestRouter_SendMessage_UnsupportedChannel(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	err := r.SendMessage(context.Background(), DeliveryTarget{Channel: "unknown", Target: "y"}, OutboundMessage{Text: "hi"})
	if err == nil {
		t.Fatal("expected unsupported channel error")
	}
}

func TestRouter_SendMessage_MessageSenderPreferred(t *testing.T) {
	ms := &stubMessageSender{id: "slack"}
	ts := &stubTextSender{id: "slack"}
	r := NewRouter(loggateway.NewNoop())
	r.RegisterMessageSender(ms)
	r.RegisterTextSender(ts)
	msg := OutboundMessage{Text: "hello", Files: []OutboundFile{{Path: "/tmp/f.txt"}}}
	err := r.SendMessage(context.Background(), DeliveryTarget{Channel: "slack", Target: "ch1"}, msg)
	if err != nil {
		t.Fatal(err)
	}
	if ms.lastMsg.Text != "hello" {
		t.Fatal("message sender should have been used")
	}
}

func TestRouter_SendMessage_FilesOnTextOnlySender(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	r.RegisterTextSender(&stubTextSender{id: "telegram"})
	msg := OutboundMessage{Text: "hi", Files: []OutboundFile{{Path: "/tmp/f.txt"}}}
	err := r.SendMessage(context.Background(), DeliveryTarget{Channel: "telegram", Target: "ch1"}, msg)
	if err == nil {
		t.Fatal("expected file delivery error")
	}
}

func TestRouter_SendMessage_TextSenderFallback(t *testing.T) {
	ts := &stubTextSender{id: "telegram"}
	r := NewRouter(loggateway.NewNoop())
	r.RegisterTextSender(ts)
	err := r.SendMessage(context.Background(), DeliveryTarget{Channel: "telegram", Target: "ch1"}, OutboundMessage{Text: "hi"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRouter_SendMessage_SenderError(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	r.RegisterTextSender(&stubTextSender{id: "telegram", sendErr: errors.New("network")})
	err := r.SendText(context.Background(), DeliveryTarget{Channel: "telegram", Target: "ch1"}, "hi")
	if err == nil {
		t.Fatal("expected send error")
	}
}

func TestCollectPaths(t *testing.T) {
	paths := collectPaths("a.txt", []string{"b.txt", "c.txt"})
	if len(paths) != 3 {
		t.Fatalf("expected 3, got %d", len(paths))
	}
}

func TestCollectPaths_Dedup(t *testing.T) {
	paths := collectPaths("a.txt", []string{"a.txt", "b.txt"})
	if len(paths) != 2 {
		t.Fatalf("expected 2 (dedup), got %d", len(paths))
	}
}

func TestCollectPaths_Empty(t *testing.T) {
	paths := collectPaths("", nil)
	if len(paths) != 0 {
		t.Fatalf("expected 0, got %d", len(paths))
	}
}

func TestCollectPaths_TrimSpace(t *testing.T) {
	paths := collectPaths("  a.txt  ", []string{"  b.txt  "})
	if len(paths) != 2 {
		t.Fatalf("expected 2, got %d", len(paths))
	}
	if paths[0] != "a.txt" {
		t.Fatalf("expected trimmed 'a.txt', got %q", paths[0])
	}
	if paths[1] != "b.txt" {
		t.Fatalf("expected trimmed 'b.txt', got %q", paths[1])
	}
}

func TestWrapOutboundText(t *testing.T) {
	inner := &stubOutboundText{id: "test"}
	sender := WrapOutboundText(inner)
	if sender == nil {
		t.Fatal("expected non-nil sender")
	}
	if sender.ID() != "test" {
		t.Fatalf("expected test, got %q", sender.ID())
	}
}

func TestWrapOutboundText_Nil(t *testing.T) {
	sender := WrapOutboundText(nil)
	if sender != nil {
		t.Fatal("WrapOutboundText(nil) returns non-nil adapter wrapping nil inner")
	}
}
