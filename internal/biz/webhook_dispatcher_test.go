package biz

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestWebhookDispatcher_TestDeliver_Success(t *testing.T) {
	t.Setenv("ARANEA_OUTBOUND_ALLOW_HOSTS", "127.0.0.1")
	var gotPayload WebhookPayload
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Custom")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewWebhookDispatcher(nil, nil, nil)
	res := d.TestDeliver(context.Background(), WebhookConfig{
		ID:      "wh-1",
		URL:     srv.URL,
		Headers: map[string]string{"X-Custom": "yes"},
	})
	if !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", res.StatusCode)
	}
	if gotPayload.EventType != WebhookEventTest {
		t.Fatalf("event_type=%q", gotPayload.EventType)
	}
	if gotHeader != "yes" {
		t.Fatalf("custom header=%q", gotHeader)
	}
}

func TestWebhookDispatcher_TestDeliver_HTTPError(t *testing.T) {
	t.Setenv("ARANEA_OUTBOUND_ALLOW_HOSTS", "127.0.0.1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := NewWebhookDispatcher(nil, nil, nil)
	res := d.TestDeliver(context.Background(), WebhookConfig{ID: "wh-1", URL: srv.URL})
	if res.Success {
		t.Fatal("expected failure on 500")
	}
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status=%d", res.StatusCode)
	}
	if !strings.Contains(res.Error, "500") {
		t.Fatalf("error=%q should mention status", res.Error)
	}
}

func TestWebhookDispatcher_TestDeliver_RejectsLocalhostWhenNotAllowed(t *testing.T) {
	t.Setenv("ARANEA_OUTBOUND_ALLOW_HOSTS", "")
	d := NewWebhookDispatcher(nil, nil, nil)
	res := d.TestDeliver(context.Background(), WebhookConfig{ID: "wh-1", URL: "http://127.0.0.1:9/hook"})
	if res.Success {
		t.Fatal("expected SSRF guard rejection")
	}
	if res.Error == "" {
		t.Fatal("expected error message")
	}
}

func TestWebhookDispatcher_TestDeliver_Unreachable(t *testing.T) {
	t.Setenv("ARANEA_OUTBOUND_ALLOW_HOSTS", "127.0.0.1")
	d := NewWebhookDispatcher(nil, nil, nil)
	res := d.TestDeliver(context.Background(), WebhookConfig{ID: "wh-1", URL: "http://127.0.0.1:1/hook"})
	if res.Success {
		t.Fatal("expected failure for unreachable target")
	}
	if res.Error == "" {
		t.Fatal("expected dial error message")
	}
}
