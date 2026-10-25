package biz

import "testing"

// E2E-C3: the HTTP async-submit path surfaces background turn failures ONLY
// via the send_failed system notice (the ACK already returned). Before the
// upgrade the notice rode the Informational lane — no outbox row, no replay —
// so a dropped live frame meant the failure vanished silently after ACK.
// send_failed must be Critical-delivery (WBPF + durable outbox + replay).

func TestIsCriticalDeliveryEvent_SendFailedNotice(t *testing.T) {
	ev := NewSystemNoticeEvent("sess-1", "send_failed", "turn execution failed", map[string]any{
		"error_type": "send_failed",
	})
	if !IsCriticalDeliveryEvent(ev) {
		t.Fatal("IsCriticalDeliveryEvent(system.notice/send_failed) = false, want true (E2E-C3)")
	}
}

func TestIsCriticalDeliveryEvent_OtherNoticesStayInformational(t *testing.T) {
	for _, noticeType := range []string{"metrics_updated", "context_usage", "contract_mismatch", "llm_retry"} {
		ev := NewSystemNoticeEvent("sess-1", noticeType, "", nil)
		if IsCriticalDeliveryEvent(ev) {
			t.Fatalf("IsCriticalDeliveryEvent(system.notice/%s) = true, want false (lane discipline)", noticeType)
		}
	}
}
