package telegram

import "testing"

func TestParseInbound(t *testing.T) {
	raw := []byte(`{"update_id":9,"message":{"message_id":2,"text":" hello ","chat":{"id":12345},"from":{"username":"alice"}}}`)
	msg, err := ParseInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "hello" || msg.ChatID != 12345 || msg.UpdateID != 9 {
		t.Fatalf("%#v", msg)
	}
}

func TestVerifySecretToken(t *testing.T) {
	if err := VerifySecretToken("tok", "tok"); err != nil {
		t.Fatal(err)
	}
	if err := VerifySecretToken("bad", "tok"); err == nil {
		t.Fatal("expected mismatch")
	}
}
