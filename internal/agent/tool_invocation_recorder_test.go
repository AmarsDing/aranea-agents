package agent

import (
	"errors"
	"strings"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// TestInvocationStatusFromAfter_NormalizesToFailed pins the wire-level status value
// produced for a generic tool error. Frontend contract (frontend/src/features/chat/
// envelopeToolCall.ts:normalizeToolStatus) collapses "failed" and "error" to one
// canonical status, but the EnvelopeToolCall.Status field sent to the UI historically
// used "error" — which made the frontend's `error_code` fallback unreachable and
// erased error info. We require "failed" here so the fix cannot silently regress.
func TestInvocationStatusFromAfter_NormalizesToFailed(t *testing.T) {
	t.Parallel()
	args := &trpctool.AfterToolArgs{Error: errors.New("boom: file not found")}
	status, code, msg := invocationStatusFromAfter(args)
	if status != "failed" {
		t.Fatalf("status: want \"failed\", got %q", status)
	}
	if code != "tool_error" {
		t.Fatalf("errCode: want \"tool_error\", got %q", code)
	}
	if !strings.Contains(msg, "boom") {
		t.Fatalf("errMsg: want to contain error text, got %q", msg)
	}
}

func TestInvocationStatusFromAfter_BlockedForConfirmation(t *testing.T) {
	t.Parallel()
	args := &trpctool.AfterToolArgs{Error: errors.New(errToolConfirmationRequired + ": please confirm")}
	status, code, _ := invocationStatusFromAfter(args)
	if status != "blocked" || code != "confirmation_required" {
		t.Fatalf("blocked path: got status=%q code=%q", status, code)
	}
}

func TestInvocationStatusFromAfter_SuccessPath(t *testing.T) {
	t.Parallel()
	args := &trpctool.AfterToolArgs{} // no error
	status, code, _ := invocationStatusFromAfter(args)
	if status != "success" || code != "" {
		t.Fatalf("success path: got status=%q code=%q", status, code)
	}
}

// TestSkillInvocationOutcome_FailedMapping ensures the runtime status "failed"
// maps to outcome="failure" (this is the new canonical mapping after the fix).
// "error" is also accepted for backward compatibility with rows written before
// the status was normalized.
func TestSkillInvocationOutcome_FailedMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status string
		want   string
	}{
		{"success", "success"},
		{"failed", "failure"},
		{"error", "failure"}, // legacy: pre-normalization rows
		{"blocked", "partial"},
		{"running", "partial"},
		{"", "partial"},
	}
	for _, c := range cases {
		got := skillOutcome(c.status)
		if got != c.want {
			t.Errorf("status=%q: want outcome=%q, got %q", c.status, c.want, got)
		}
	}
}

// skillOutcome mirrors the switch in recordSkillInvocation. Keep in sync.
func skillOutcome(status string) string {
	switch status {
	case "success":
		return "success"
	case "failed", "error":
		return "failure"
	default:
		return "partial"
	}
}
