package serviceawaitreply

import "testing"

func TestParseToolConfirmReply(t *testing.T) {
	t.Parallel()
	tests := []struct {
		reply      string
		approved   bool
		structured bool
	}{
		{ReplyApprove, true, true},
		{ReplyDeny, false, true},
		{" yes ", false, false},
		{"approve", false, false},
	}
	for _, tc := range tests {
		approved, structured := ParseToolConfirmReply(tc.reply)
		if approved != tc.approved || structured != tc.structured {
			t.Fatalf("ParseToolConfirmReply(%q) = (%v,%v), want (%v,%v)", tc.reply, approved, structured, tc.approved, tc.structured)
		}
	}
}

func TestParseToolConfirmOutcome(t *testing.T) {
	t.Parallel()
	tests := []struct {
		reply      string
		outcome    ToolConfirmOutcome
		structured bool
	}{
		{ReplyApprove, ToolConfirmOutcomeApprove, true},
		{ReplyDeny, ToolConfirmOutcomeDeny, true},
		{ReplyApproveSession, ToolConfirmOutcomeApproveSession, true},
		{ReplyApproveAlways, ToolConfirmOutcomeApproveAlways, true},
		{" " + ReplyApproveAlways + " ", ToolConfirmOutcomeApproveAlways, true},
		{"yes", ToolConfirmOutcomeDeny, false},
		{"", ToolConfirmOutcomeDeny, false},
	}
	for _, tc := range tests {
		outcome, structured := ParseToolConfirmOutcome(tc.reply)
		if outcome != tc.outcome || structured != tc.structured {
			t.Fatalf("ParseToolConfirmOutcome(%q) = (%v,%v), want (%v,%v)", tc.reply, outcome, structured, tc.outcome, tc.structured)
		}
	}
}

// Approved reports whether the outcome allows the tool to run.
func TestToolConfirmOutcomeApproved(t *testing.T) {
	t.Parallel()
	tests := []struct {
		outcome  ToolConfirmOutcome
		approved bool
	}{
		{ToolConfirmOutcomeApprove, true},
		{ToolConfirmOutcomeApproveSession, true},
		{ToolConfirmOutcomeApproveAlways, true},
		{ToolConfirmOutcomeDeny, false},
	}
	for _, tc := range tests {
		if got := tc.outcome.Approved(); got != tc.approved {
			t.Fatalf("outcome %v.Approved() = %v, want %v", tc.outcome, got, tc.approved)
		}
	}
}

func TestToolConfirmRequestFromContext(t *testing.T) {
	t.Parallel()
	ctx := WithToolConfirmRequest(t.Context(), ToolConfirmRequest{ToolKey: "bash", ToolCallID: "call-1"})
	req, ok := ToolConfirmRequestFromContext(ctx)
	if !ok || req.ToolKey != "bash" || req.ToolCallID != "call-1" {
		t.Fatalf("unexpected request: %+v ok=%v", req, ok)
	}
}
