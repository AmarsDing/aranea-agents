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

func TestToolConfirmRequestFromContext(t *testing.T) {
	t.Parallel()
	ctx := WithToolConfirmRequest(t.Context(), ToolConfirmRequest{ToolKey: "bash", ToolCallID: "call-1"})
	req, ok := ToolConfirmRequestFromContext(ctx)
	if !ok || req.ToolKey != "bash" || req.ToolCallID != "call-1" {
		t.Fatalf("unexpected request: %+v ok=%v", req, ok)
	}
}
