package lark

import (
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
