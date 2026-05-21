package wecom

import (
	"testing"
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
	ts := "1409659589"
	nonce := "xxxxxx"
	sig := SignFor(token, ts, nonce)
	if err := VerifySignature(token, ts, nonce, sig); err != nil {
		t.Fatal(err)
	}
	if err := VerifySignature(token, ts, nonce, "bad"); err == nil {
		t.Fatal("expected bad signature error")
	}
}
