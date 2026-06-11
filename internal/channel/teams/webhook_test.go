package teams

import (
	"errors"
	"net/http"
	"testing"

	"aranea-agents/internal/channel/port"
)

func TestParseInbound_MessageActivity(t *testing.T) {
	raw := []byte(`{
		"type": "message",
		"id": "act123",
		"timestamp": "2026-01-01T00:00:00Z",
		"serviceUrl": "https://smba.trafficmanager.net/amer/",
		"channelId": "msteams",
		"from": {"id": "user1", "name": "Test User"},
		"conversation": {"id": "conv1", "conversationType": "personal"},
		"recipient": {"id": "bot1", "name": "Test Bot"},
		"text": " hello world "
	}`)
	msg, err := ParseInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "hello world" {
		t.Fatalf("text: got %q", msg.Text)
	}
	if msg.FromID != "user1" {
		t.Fatalf("from_id: got %q", msg.FromID)
	}
	if msg.ConversationID != "conv1" {
		t.Fatalf("conversation_id: got %q", msg.ConversationID)
	}
	if msg.ServiceURL != "https://smba.trafficmanager.net/amer/" {
		t.Fatalf("service_url: got %q", msg.ServiceURL)
	}
	if msg.ActivityID != "act123" {
		t.Fatalf("activity_id: got %q", msg.ActivityID)
	}
	if msg.RecipientID != "bot1" {
		t.Fatalf("recipient_id: got %q", msg.RecipientID)
	}
}

func TestParseInbound_NonMessageIgnored(t *testing.T) {
	raw := []byte(`{
		"type": "conversationUpdate",
		"id": "act456",
		"channelId": "msteams"
	}`)
	_, err := ParseInbound(raw)
	if err == nil {
		t.Fatal("expected error for non-message activity")
	}
}

func TestParseInbound_EmptyText(t *testing.T) {
	raw := []byte(`{
		"type": "message",
		"id": "act789",
		"channelId": "msteams",
		"from": {"id": "u1"},
		"conversation": {"id": "c1"},
		"text": "   "
	}`)
	_, err := ParseInbound(raw)
	if err == nil {
		t.Fatal("expected error for empty text")
	}
}

func TestTextSenderID(t *testing.T) {
	s := &TextSender{}
	if s.ID() != "teams" {
		t.Fatalf("id: got %q", s.ID())
	}
}

func TestVerifyRequest_EmptyCredentials(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer some-token")
	err := VerifyRequest("", "", h, nil)
	if !errors.Is(err, port.ErrCredentialsNotConfigured) {
		t.Fatalf("expected ErrCredentialsNotConfigured, got %v", err)
	}
}
