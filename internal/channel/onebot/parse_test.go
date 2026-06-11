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
	raw := []byte(`{"message_type":"group","group_id":"g1","user_id":"u1","message_id":"m1","raw_message":" hello "}`)
	msg, err := ParseInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "hello" || msg.PeerID != "g1" || msg.GroupID != "g1" {
		t.Fatalf("%+v", msg)
	}
}
