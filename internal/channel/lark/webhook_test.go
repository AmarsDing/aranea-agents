package lark

import (
	"bytes"
	"net/http"
	"testing"
)

func TestEventSignature(t *testing.T) {
	ts := "123"
	nonce := "abc"
	key := "testkey"
	body := []byte(`{"hello":"world"}`)
	sig := EventSignature(ts, nonce, key, body)
	if !VerifyEventSignature(ts, nonce, key, body, sig) {
		t.Fatal("signature roundtrip")
	}
	if VerifyEventSignature(ts, nonce, key, body, "wrong") {
		t.Fatal("expected mismatch")
	}
}

func TestParseURLVerification(t *testing.T) {
	raw := []byte(`{"type":"url_verification","token":"t","challenge":"ch1"}`)
	res, err := ParseWebhookPost(raw, "t")
	if err != nil || !res.IsURLVerification || res.Challenge != "ch1" {
		t.Fatalf("got %+v err=%v", res, err)
	}
	if _, err := ParseWebhookPost(raw, "other"); err == nil {
		t.Fatal("expect token mismatch")
	}
}

func TestVerifyHTTPRequest(t *testing.T) {
	body := []byte(`{"type":"event_callback"}`)
	ts := "1710000000"
	nonce := "n1"
	key := "encrypt-key"
	sig := EventSignature(ts, nonce, key, body)
	req, err := http.NewRequest(http.MethodPost, "http://example/webhook", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Lark-Request-Timestamp", ts)
	req.Header.Set("X-Lark-Request-Nonce", nonce)
	req.Header.Set("X-Lark-Signature", sig)
	if err := VerifyHTTPRequest(req, key, body); err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Lark-Signature", "bad")
	if err := VerifyHTTPRequest(req, key, body); err == nil {
		t.Fatal("expected signature mismatch")
	}
	if err := VerifyHTTPRequest(req, "", body); err != nil {
		t.Fatal("empty encrypt key should skip verify")
	}
}
