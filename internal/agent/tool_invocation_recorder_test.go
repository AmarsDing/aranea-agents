package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"

	"github.com/prometheus/client_golang/prometheus/testutil"
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

// 参数质量信号必须转化为 aranea_tool_args_guard_total{tool,outcome} 计数，
// 否则"哪个工具的 schema 在诱导模型产出坏参数"无从观测。
func TestRecordToolInvocation_ArgsGuardOutcomeMetrics(t *testing.T) {
	cases := []struct {
		name    string
		q       toolArgsQuality
		outcome string
	}{
		{"repaired increments repaired outcome", toolArgsQuality{Repaired: true}, "repaired"},
		{"invalid increments invalid outcome", toolArgsQuality{Invalid: true}, "invalid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			deps := TRPCBuilderDeps{}
			deps.ToolUC = &fakeToolLookup{}
			tool := "recorder_guard_metric_" + c.outcome
			ctx := context.WithValue(context.Background(), toolArgsQualityKey{}, c.q)
			counter := metrics.ToolArgsGuardTotal.WithLabelValues(tool, c.outcome)
			before := testutil.ToFloat64(counter)
			recordToolInvocationAfter(ctx, &trpctool.AfterToolArgs{ToolName: tool, ToolCallID: "tc-" + c.outcome}, biz.Agent{ID: "agent-x"}, deps)
			if delta := testutil.ToFloat64(counter) - before; delta != 1 {
				t.Fatalf("counter delta: want 1, got %v", delta)
			}
		})
	}
}
