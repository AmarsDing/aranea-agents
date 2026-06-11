package mattermost

import (
	"errors"
	"testing"

	"aranea-agents/internal/channel/port"
)

func TestParseInbound_TextMessage(t *testing.T) {
	raw := []byte(`{
		"token": "abc123",
		"team_id": "team1",
		"channel_id": "chan1",
		"user_id": "user1",
		"post_id": "post1",
		"text": " hello ",
		"trigger_word": ""
	}`)
	msg, err := ParseInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "hello" {
		t.Fatalf("text: got %q", msg.Text)
	}
	if msg.UserID != "user1" {
		t.Fatalf("user_id: got %q", msg.UserID)
	}
	if msg.ChannelID != "chan1" {
		t.Fatalf("channel_id: got %q", msg.ChannelID)
	}
	if msg.PostID != "post1" {
		t.Fatalf("post_id: got %q", msg.PostID)
	}
}

func TestParseInbound_EmptyText(t *testing.T) {
	raw := []byte(`{"token":"abc","channel_id":"c1","user_id":"u1","text":"  "}`)
	_, err := ParseInbound(raw)
	if err == nil {
		t.Fatal("expected error for empty text")
	}
}

func TestVerifyToken(t *testing.T) {
	if err := VerifyToken("secret", "secret"); err != nil {
		t.Fatal(err)
	}
	if err := VerifyToken("secret", "wrong"); err == nil {
		t.Fatal("expected error for wrong token")
	}
	if err := VerifyToken("", "any"); !errors.Is(err, port.ErrCredentialsNotConfigured) {
		t.Fatalf("expected ErrCredentialsNotConfigured, got %v", err)
	}
}

func TestParseWSMessage_PostedEvent(t *testing.T) {
	raw := []byte(`{
		"event": "posted",
		"data": {
			"post": "{\"message\":\"hello world\",\"channel_id\":\"chan1\",\"user_id\":\"user1\",\"id\":\"post1\"}",
			"sender_type": "user"
		}
	}`)
	ev, ok := parseWSMessage(raw, "bot123")
	if !ok {
		t.Fatal("expected ok")
	}
	if ev.Text != "hello world" {
		t.Fatalf("text: got %q", ev.Text)
	}
	if ev.OutboundMeta["recipient"] != "chan1" {
		t.Fatalf("recipient: got %q", ev.OutboundMeta["recipient"])
	}
	if ev.IdempotencyKey != "mattermost:post1" {
		t.Fatalf("idempotency_key: got %q", ev.IdempotencyKey)
	}
}

func TestParseWSMessage_BotIgnored(t *testing.T) {
	raw := []byte(`{
		"event": "posted",
		"data": {
			"post": "{\"message\":\"bot msg\",\"channel_id\":\"c1\",\"user_id\":\"bot123\",\"id\":\"p1\"}",
			"sender_type": "bot"
		}
	}`)
	_, ok := parseWSMessage(raw, "bot123")
	if ok {
		t.Fatal("bot messages should be ignored")
	}
}

func TestParseWSMessage_SelfIgnored(t *testing.T) {
	raw := []byte(`{
		"event": "posted",
		"data": {
			"post": "{\"message\":\"self msg\",\"channel_id\":\"c1\",\"user_id\":\"bot123\",\"id\":\"p1\"}",
			"sender_type": "user"
		}
	}`)
	_, ok := parseWSMessage(raw, "bot123")
	if ok {
		t.Fatal("self messages should be ignored")
	}
}

func TestParseWSMessage_NonPostedIgnored(t *testing.T) {
	raw := []byte(`{"event": "channel_viewed", "data": {}}`)
	_, ok := parseWSMessage(raw, "bot123")
	if ok {
		t.Fatal("non-posted events should be ignored")
	}
}

func TestBuildWSURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://mattermost.example.com", "wss://mattermost.example.com/api/v4/websocket"},
		{"http://localhost:8065", "ws://localhost:8065/api/v4/websocket"},
		{"https://mm.example.com/", "wss://mm.example.com/api/v4/websocket"},
	}
	for _, tt := range tests {
		got := buildWSURL(tt.input)
		if got != tt.expected {
			t.Errorf("buildWSURL(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
