package line

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"aranea-agents/internal/channel/port"
)

func TestParseInbound_TextMessage(t *testing.T) {
	raw := []byte(`{
		"events": [{
			"type": "message",
			"replyToken": "rt123",
			"source": {"type": "user", "userId": "Uabc"},
			"message": {"id": "msg1", "type": "text", "text": " hello "}
		}]
	}`)
	msgs, err := ParseInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	m := msgs[0]
	if m.Text != "hello" {
		t.Fatalf("text: got %q", m.Text)
	}
	if m.UserID != "Uabc" {
		t.Fatalf("user_id: got %q", m.UserID)
	}
	if m.ReplyToken != "rt123" {
		t.Fatalf("reply_token: got %q", m.ReplyToken)
	}
	if m.MessageID != "msg1" {
		t.Fatalf("message_id: got %q", m.MessageID)
	}
}

func TestParseInbound_GroupMessage(t *testing.T) {
	raw := []byte(`{
		"events": [{
			"type": "message",
			"replyToken": "rt456",
			"source": {"type": "group", "userId": "Uabc", "groupId": "Gxyz"},
			"message": {"id": "msg2", "type": "text", "text": "hi"}
		}]
	}`)
	msgs, err := ParseInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].GroupID != "Gxyz" {
		t.Fatalf("group_id: got %q", msgs[0].GroupID)
	}
}

func TestParseInbound_NonTextIgnored(t *testing.T) {
	raw := []byte(`{
		"events": [{
			"type": "message",
			"replyToken": "rt789",
			"source": {"type": "user", "userId": "Uabc"},
			"message": {"id": "msg3", "type": "image", "originalContentUrl": "https://example.com/img.jpg"}
		}]
	}`)
	_, err := ParseInbound(raw)
	if err == nil {
		t.Fatal("expected error for non-text message")
	}
}

func TestParseInbound_NonMessageIgnored(t *testing.T) {
	raw := []byte(`{
		"events": [{
			"type": "follow",
			"source": {"type": "user", "userId": "Uabc"}
		}]
	}`)
	_, err := ParseInbound(raw)
	if err == nil {
		t.Fatal("expected error for non-message event")
	}
}

func TestVerifySignature(t *testing.T) {
	secret := "test_secret"
	body := []byte("hello world")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if err := VerifySignature(secret, body, sig); err != nil {
		t.Fatal(err)
	}
	if err := VerifySignature(secret, body, "bad_sig"); err == nil {
		t.Fatal("expected error for bad signature")
	}
}

func TestVerifySignatureEmptySecretRejects(t *testing.T) {
	if err := VerifySignature("", []byte("body"), "sig"); err == nil {
		t.Fatal("empty secret should reject with ErrCredentialsNotConfigured")
	} else if err != port.ErrCredentialsNotConfigured {
		t.Fatalf("expected ErrCredentialsNotConfigured, got: %v", err)
	}
}
