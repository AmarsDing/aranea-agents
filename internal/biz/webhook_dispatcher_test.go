package biz

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestWebhookSubscribes(t *testing.T) {
	json := `["run.completed","run.failed"]`
	if !WebhookSubscribes(json, WebhookEventRunCompleted) {
		t.Fatal("expected subscription to run.completed")
	}
	if WebhookSubscribes(json, WebhookEventRunCancelled) {
		t.Fatal("expected no subscription to run.cancelled")
	}
	if !WebhookSubscribes("[]", WebhookEventRunFailed) {
		t.Fatal("empty filter should accept all events")
	}
}

func TestRunStatusToWebhookEvent(t *testing.T) {
	if got := RunStatusToWebhookEvent("completed"); got != WebhookEventRunCompleted {
		t.Fatalf("got %q", got)
	}
	if got := RunStatusToWebhookEvent("running"); got != "" {
		t.Fatalf("expected empty for non-terminal status, got %q", got)
	}
}

func TestWebhookHMACSignature(t *testing.T) {
	body := []byte(`{"event_type":"run.completed"}`)
	secret := "test-secret"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	if len(sig) != 64 {
		t.Fatalf("expected 64-char hex signature, got len %d", len(sig))
	}
}
