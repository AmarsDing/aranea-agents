package service

import (
	"testing"

	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
)

func TestResolveConfirmReply_LegacyApproved(t *testing.T) {
	t.Parallel()
	token, approved, err := resolveConfirmReply(true, "")
	if err != nil || !approved || token != "approved" {
		t.Fatalf("= (%q,%v,%v), want (approved,true,nil)", token, approved, err)
	}
}

func TestResolveConfirmReply_LegacyRejected(t *testing.T) {
	t.Parallel()
	token, approved, err := resolveConfirmReply(false, "")
	if err != nil || approved || token != "rejected" {
		t.Fatalf("= (%q,%v,%v), want (rejected,false,nil)", token, approved, err)
	}
}

func TestResolveConfirmReply_StructuredTokens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		reply    string
		approved bool
	}{
		{serviceawaitreply.ReplyApprove, true},
		{serviceawaitreply.ReplyDeny, false},
		{serviceawaitreply.ReplyApproveSession, true},
		{serviceawaitreply.ReplyApproveAlways, true},
	}
	for _, tc := range cases {
		token, approved, err := resolveConfirmReply(false, tc.reply)
		if err != nil {
			t.Fatalf("reply %q: err = %v", tc.reply, err)
		}
		if token != tc.reply || approved != tc.approved {
			t.Fatalf("reply %q: = (%q,%v), want (%q,%v)", tc.reply, token, approved, tc.reply, tc.approved)
		}
	}
}

func TestResolveConfirmReply_StructuredOverridesApprovedFlag(t *testing.T) {
	t.Parallel()
	// reply=deny with approved=true must resolve to deny (structured wins).
	token, approved, err := resolveConfirmReply(true, serviceawaitreply.ReplyDeny)
	if err != nil || approved || token != serviceawaitreply.ReplyDeny {
		t.Fatalf("= (%q,%v,%v), want (%q,false,nil)", token, approved, err, serviceawaitreply.ReplyDeny)
	}
}

func TestResolveConfirmReply_InvalidTokenRejected(t *testing.T) {
	t.Parallel()
	if _, _, err := resolveConfirmReply(true, "__aranea:tool_confirm:bogus"); err == nil {
		t.Fatal("expected error for unknown structured token")
	}
}
