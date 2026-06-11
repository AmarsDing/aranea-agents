package dingtalk

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"aranea-agents/internal/channel/port"
)

func TestParseInbound(t *testing.T) {
	raw := []byte(`{"msgtype":"text","text":{"content":" hello "},"senderStaffId":"u1","conversationId":"c1"}`)
	msg, err := ParseInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "hello" {
		t.Fatalf("text=%q", msg.Text)
	}
}

func TestVerifySign(t *testing.T) {
	secret := "SEC123"
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	sign := signFor(secret, ts)
	if err := VerifySign(ts, sign, secret); err != nil {
		t.Fatal(err)
	}
	if err := VerifySign(ts, "bad-sign", secret); err == nil {
		t.Fatal("expected bad signature")
	}
	if err := VerifySign(ts, "sign", ""); !errors.Is(err, port.ErrCredentialsNotConfigured) {
		t.Fatalf("expected ErrCredentialsNotConfigured, got %v", err)
	}
}
