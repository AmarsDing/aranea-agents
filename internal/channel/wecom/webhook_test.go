package wecom

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"aranea-agents/internal/channel/port"
)

func TestParseInbound(t *testing.T) {
	raw := []byte(`{"msgtype":"text","text":{"content":" hi "},"from":{"userid":"u1"},"chatid":"c1","response_url":"https://example.com/r"}`)
	msg, err := ParseInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "hi" {
		t.Fatalf("text=%q", msg.Text)
	}
	if msg.ResponseURL == "" {
		t.Fatal("expected response_url")
	}
}

func TestVerifySignature(t *testing.T) {
	token := "tok"
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := "xxxxxx"
	sig := SignFor(token, ts, nonce)
	if err := VerifySignature(token, ts, nonce, sig); err != nil {
		t.Fatal(err)
	}
	if err := VerifySignature(token, ts, nonce, "bad"); err == nil {
		t.Fatal("expected bad signature error")
	}
	if err := VerifySignature("", ts, nonce, "sig"); !errors.Is(err, port.ErrCredentialsNotConfigured) {
		t.Fatalf("expected ErrCredentialsNotConfigured, got %v", err)
	}
}
