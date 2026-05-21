package biz

import "testing"

func TestTerminalRunStatuses(t *testing.T) {
	m := terminalRunStatuses()
	for _, s := range []string{"completed", "failed", "cancelled", "canceled"} {
		if _, ok := m[s]; !ok {
			t.Fatalf("missing %q", s)
		}
	}
	if _, ok := m["running"]; ok {
		t.Fatal("running should not be terminal")
	}
}

func TestRunStatusToWebhookEventFromConsumer(t *testing.T) {
	if got := RunStatusToWebhookEvent("completed"); got != WebhookEventRunCompleted {
		t.Fatalf("got %q", got)
	}
}
