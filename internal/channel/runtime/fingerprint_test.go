package runtime

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestRuntimeFingerprintIgnoresUpdatedAt(t *testing.T) {
	ch1 := biz.Channel{
		ID:         "ch-1",
		Enabled:    true,
		ConfigJSON: `{"type":"feishu","receive_mode":"websocket"}`,
		UpdatedAt:  "2026-05-22T17:28:00Z",
	}
	ch2 := ch1
	ch2.UpdatedAt = "2026-05-22T17:30:00Z"
	mode := "websocket"
	rev := "cred-rev"
	if runtimeFingerprint(ch1, mode, rev) != runtimeFingerprint(ch2, mode, rev) {
		t.Fatal("UpdatedAt must not affect runtime fingerprint")
	}
}

func TestRuntimeFingerprintChangesOnConfig(t *testing.T) {
	base := biz.Channel{ID: "ch-1", Enabled: true, ConfigJSON: `{"type":"feishu","receive_mode":"websocket"}`}
	other := base
	other.ConfigJSON = `{"type":"feishu","receive_mode":"webhook"}`
	if runtimeFingerprint(base, "websocket", "") == runtimeFingerprint(other, "webhook", "") {
		t.Fatal("config/receive_mode change must change fingerprint")
	}
}
