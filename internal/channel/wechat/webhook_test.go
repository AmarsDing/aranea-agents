package wechat

import (
	"crypto/sha1"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/channel/port"
)

func signWeChat(token, timestamp, nonce string) string {
	parts := []string{token, timestamp, nonce}
	sort.Strings(parts)
	sum := sha1.Sum([]byte(strings.Join(parts, "")))
	return fmt.Sprintf("%x", sum)
}

func TestVerifyURL(t *testing.T) {
	token := "wechat-token"
	ts := fmt.Sprintf("%d", time.Now().Unix())
	nonce := "nonce1"
	echo := "echostr123"
	sig := signWeChat(token, ts, nonce)

	got, err := VerifyURL(token, ts, nonce, echo, sig)
	if err != nil || got != echo {
		t.Fatalf("VerifyURL: got=%q err=%v", got, err)
	}
	if _, err := VerifyURL(token, ts, nonce, echo, "bad"); err == nil {
		t.Fatal("expected bad signature error")
	}
}

func TestVerifyPOST(t *testing.T) {
	token := "wechat-token"
	ts := fmt.Sprintf("%d", time.Now().Unix())
	nonce := "nonce1"
	sig := signWeChat(token, ts, nonce)
	if err := VerifyPOST(token, ts, nonce, sig); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPOST(token, ts, nonce, "bad"); err == nil {
		t.Fatal("expected bad signature")
	}
}

func TestVerifyEmptyTokenRejects(t *testing.T) {
	if _, err := VerifyURL("", "1", "n", "echo", "anything"); err == nil {
		t.Fatal("empty token should reject with ErrCredentialsNotConfigured")
	} else if err != port.ErrCredentialsNotConfigured {
		t.Fatalf("expected ErrCredentialsNotConfigured, got: %v", err)
	}
	if err := VerifyPOST("", "1", "n", "anything"); err == nil {
		t.Fatal("empty token should reject with ErrCredentialsNotConfigured")
	} else if err != port.ErrCredentialsNotConfigured {
		t.Fatalf("expected ErrCredentialsNotConfigured, got: %v", err)
	}
}

func TestVerifyExpiredTimestamp(t *testing.T) {
	token := "wechat-token"
	// Timestamp 10 minutes in the past
	ts := fmt.Sprintf("%d", time.Now().Unix()-600)
	nonce := "nonce1"
	sig := signWeChat(token, ts, nonce)
	if err := VerifyPOST(token, ts, nonce, sig); err == nil {
		t.Fatal("expired timestamp should be rejected")
	}
}

func TestParseTextInbound(t *testing.T) {
	raw := []byte(`<xml><ToUserName><![CDATA[to]]></ToUserName><FromUserName><![CDATA[from]]></FromUserName><CreateTime>1</CreateTime><MsgType><![CDATA[text]]></MsgType><Content><![CDATA[ hello ]]></Content><MsgId>99</MsgId></xml>`)
	msg, err := ParseTextInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "hello" || msg.FromUser != "from" || msg.MsgID != 99 {
		t.Fatalf("%+v", msg)
	}
}
