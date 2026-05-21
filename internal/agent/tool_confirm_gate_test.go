package agent

import (
	"testing"

	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
)

func TestToolConfirmApprovedStructured(t *testing.T) {
	t.Parallel()
	if !toolConfirmApproved(serviceawaitreply.ReplyApprove) {
		t.Fatal("expected approve token to be approved")
	}
	if toolConfirmApproved(serviceawaitreply.ReplyDeny) {
		t.Fatal("expected deny token to be rejected")
	}
}

func TestToolConfirmApprovedTextFallback(t *testing.T) {
	t.Parallel()
	if !toolConfirmApproved("yes") {
		t.Fatal("expected text yes to be approved")
	}
	if toolConfirmApproved("no") {
		t.Fatal("expected text no to be rejected")
	}
}
