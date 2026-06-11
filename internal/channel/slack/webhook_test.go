package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"testing"
	"time"

	"aranea-agents/internal/channel/port"
)

func TestParseInboundURLVerification(t *testing.T) {
	ch, msg, err := ParseInbound([]byte(`{"type":"url_verification","challenge":"abc"}`))
	if err != nil || msg != nil || ch != "abc" {
		t.Fatalf("ch=%q msg=%v err=%v", ch, msg, err)
	}
}

func TestParseInboundMessage(t *testing.T) {
	raw := []byte(`{"type":"event_callback","event":{"type":"message","text":" hi ","user":"U1","channel":"C1","ts":"1.0"}}`)
	_, msg, err := ParseInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "hi" || msg.ChannelID != "C1" {
		t.Fatalf("%#v", msg)
	}
}

func TestVerifyRequest(t *testing.T) {
	secret := "signing-secret"
	body := []byte(`{"type":"event_callback"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	base := "v0:" + ts + ":" + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(base))
	sig := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if err := VerifyRequest(ts, sig, secret, body); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRequest(ts, "v0=bad", secret, body); err == nil {
		t.Fatal("expected bad signature")
	}
	stale := strconv.FormatInt(time.Now().Unix()-600, 10)
	if err := VerifyRequest(stale, sig, secret, body); err == nil {
		t.Fatal("expected stale timestamp")
	}
	if err := VerifyRequest(ts, sig, "", body); !errors.Is(err, port.ErrCredentialsNotConfigured) {
		t.Fatalf("expected ErrCredentialsNotConfigured, got %v", err)
	}
}
