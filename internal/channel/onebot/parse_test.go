package onebot

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"testing"

	"aranea-agents/internal/channel/port"
)

func signOneBot(token string, body []byte) string {
	mac := hmac.New(sha1.New, []byte(token))
	_, _ = mac.Write(body)
	return "sha1=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	token := "recv-token"
	body := []byte(`{"message_type":"private","user_id":"u1","raw_message":"hi"}`)
	sig := signOneBot(token, body)
	if err := VerifySignature(token, body, sig); err != nil {
		t.Fatal(err)
	}
	if err := VerifySignature(token, body, "sha1=deadbeef"); err == nil {
		t.Fatal("expected bad signature")
	}
}

func TestVerifySignatureEmptyTokenRejects(t *testing.T) {
	if err := VerifySignature("", []byte("{}"), ""); err == nil {
		t.Fatal("empty token should reject with ErrCredentialsNotConfigured")
	} else if err != port.ErrCredentialsNotConfigured {
		t.Fatalf("expected ErrCredentialsNotConfigured, got: %v", err)
	}
}

func TestParseInbound(t *testing.T) {
	raw := []byte(`{"message_type":"group","group_id":"8821","user_id":"10001","message_id":"42","raw_message":" hello "}`)
	msg, err := ParseInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "hello" || msg.PeerID != "8821" || msg.GroupID != "8821" {
		t.Fatalf("%+v", msg)
	}
}

// TestParseInboundNumericIDs pins G1: OneBot 11 (NapCat/LLOneBot default)
// sends message_id/user_id/group_id as JSON numbers — string-typed fields
// failed to unmarshal and the whole inbound push was silently dropped.
func TestParseInboundNumericIDs(t *testing.T) {
	raw := []byte(`{"message_type":"group","group_id":8821,"user_id":10001,"message_id":42,"sender":{"user_id":10001,"nickname":"n"},"raw_message":"hello"}`)
	msg, err := ParseInbound(raw)
	if err != nil {
		t.Fatalf("numeric OneBot ids must parse: %v", err)
	}
	if msg.MessageID != "42" || msg.UserID != "10001" || msg.GroupID != "8821" || msg.PeerID != "8821" {
		t.Fatalf("numeric ids not stringified correctly: %+v", msg)
	}
}

// TestParseInboundMixedIDs pins the private-chat fallback path with a
// numeric user_id carried only on sender.
func TestParseInboundMixedIDs(t *testing.T) {
	raw := []byte(`{"message_type":"private","user_id":null,"sender":{"user_id":10002},"message":[{"type":"text","data":{"text":"hi"}}]}`)
	msg, err := ParseInbound(raw)
	if err != nil {
		t.Fatalf("mixed OneBot ids must parse: %v", err)
	}
	if msg.UserID != "10002" || msg.Text != "hi" {
		t.Fatalf("sender fallback broken: %+v", msg)
	}
}
