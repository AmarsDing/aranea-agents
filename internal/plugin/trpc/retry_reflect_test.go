package plugintrpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func newRetryReflectForTest() *RetryAndReflectPlugin {
	return NewRetryAndReflectPlugin(biz.Plugin{Key: "retry_reflect"}, &noopStatsRecorder{}, nil, nil, loggateway.NewNoop())
}

func deterministicResultHasHint(t *testing.T, res *trpctool.AfterToolResult) bool {
	t.Helper()
	if res == nil || res.CustomResult == nil {
		return false
	}
	m, ok := res.CustomResult.(map[string]any)
	if !ok {
		return false
	}
	_, ok = m["reflection_hint"]
	return ok
}

// TestRetryReflect_DeterministicErrorSkipsReflection pins the P2 contract
// (2026-08-06 20:45 session): deterministic system errors — graph node not
// registered, unknown tool, permission denied — cannot be fixed by the LLM
// adjusting arguments, so the plugin must NOT emit a reflection hint (which
// previously caused 3 wasteful retries of "orchestration.skip not registered")
// and must NOT consume retry budget.
func TestRetryReflect_DeterministicErrorSkipsReflection(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"node not registered", errors.New(`graph build: node "orchestration.skip" is not registered`)},
		{"unknown tool", errors.New("unknown tool: web_fetch_v2")},
		{"tool not found", errors.New("tool not found: browser_open")},
		{"permission denied", errors.New("permission denied: tool requires scope admin")},
		{"forbidden", errors.New("403 forbidden by policy")},
		{"context canceled", context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newRetryReflectForTest()
			for i := 0; i < 3; i++ {
				res, err := p.afterTool(context.Background(), &trpctool.AfterToolArgs{
					ToolName: "plan_and_execute",
					Error:    tc.err,
				})
				if err != nil {
					t.Fatalf("afterTool returned error: %v", err)
				}
				if deterministicResultHasHint(t, res) {
					t.Fatalf("attempt %d: deterministic error must not emit reflection hint, got %+v", i+1, res.CustomResult)
				}
			}
			// Retry budget must be untouched: a subsequent transient error still
			// gets attempt 1, proving deterministic errors were never counted.
			res, err := p.afterTool(context.Background(), &trpctool.AfterToolArgs{
				ToolName: "plan_and_execute",
				Error:    errors.New("dial tcp: connection timeout"),
			})
			if err != nil {
				t.Fatalf("afterTool returned error: %v", err)
			}
			if !deterministicResultHasHint(t, res) {
				t.Fatalf("transient error after deterministic skips must still emit reflection hint")
			}
			m, _ := res.CustomResult.(map[string]any)
			if got := m["retry_attempt"]; got != 1 {
				t.Fatalf("retry_attempt=%v want 1 (deterministic errors must not consume budget)", got)
			}
		})
	}
}

// TestRetryReflect_ArgumentErrorStillReflects guards the inverse contract:
// argument-shape errors ARE LLM-fixable, so the reflection hint must be kept
// for them — they must not be classified as deterministic.
func TestRetryReflect_ArgumentErrorStillReflects(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"invalid argument", errors.New("invalid argument: budget must be > 0")},
		{"validation failed", errors.New("validation failed: missing required field steps")},
		{"malformed json", errors.New("malformed JSON in arguments")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newRetryReflectForTest()
			res, err := p.afterTool(context.Background(), &trpctool.AfterToolArgs{
				ToolName: "plan_and_execute",
				Error:    tc.err,
			})
			if err != nil {
				t.Fatalf("afterTool returned error: %v", err)
			}
			if !deterministicResultHasHint(t, res) {
				t.Fatalf("LLM-fixable argument error must keep reflection hint, got %+v", res)
			}
		})
	}
}

// TestRetryReflect_TransientErrorRetryBudgetUnchanged verifies the existing
// retry path is untouched: transient errors still reflect up to MaxRetries,
// then stop.
func TestRetryReflect_TransientErrorRetryBudgetUnchanged(t *testing.T) {
	p := newRetryReflectForTest()
	transient := errors.New("service temporarily unavailable")
	for i := 1; i <= 3; i++ {
		res, err := p.afterTool(context.Background(), &trpctool.AfterToolArgs{ToolName: "web_search", Error: transient})
		if err != nil {
			t.Fatalf("afterTool returned error: %v", err)
		}
		if !deterministicResultHasHint(t, res) {
			t.Fatalf("attempt %d: expected reflection hint", i)
		}
	}
	res, err := p.afterTool(context.Background(), &trpctool.AfterToolArgs{ToolName: "web_search", Error: transient})
	if err != nil {
		t.Fatalf("afterTool returned error: %v", err)
	}
	if deterministicResultHasHint(t, res) {
		t.Fatalf("attempt 4 exceeds MaxRetries=3, must not emit reflection hint")
	}
}

func TestIsDeterministicToolError(t *testing.T) {
	if isDeterministicToolError(nil) {
		t.Fatal("nil error must not be deterministic")
	}
	deterministic := []string{
		`node "orchestration.skip" is not registered`,
		"UNKNOWN TOOL: foo",
		"Tool Not Found",
		"Permission Denied",
		"HTTP 403 Forbidden",
		"request unauthorized: missing token",
		"operation not allowed for role viewer",
	}
	for _, msg := range deterministic {
		if !isDeterministicToolError(errors.New(msg)) {
			t.Errorf("%q must be deterministic", msg)
		}
	}
	retriable := []string{
		"timeout waiting for response",
		"connection refused",
		"invalid argument: budget must be > 0",
		"validation failed: missing steps",
		"rate limit exceeded",
		"internal server error",
	}
	for _, msg := range retriable {
		if isDeterministicToolError(errors.New(msg)) {
			t.Errorf("%q must NOT be deterministic (reflection can help)", msg)
		}
	}
}

// WP-2c: structured platform rate limits (subagent concurrency cap, quotas)
// are produced as apierror CodeRateLimit. Reflect-and-retry cannot fix them —
// no argument adjustment lifts a process-lifetime concurrency cap — and the
// raw error already carries actionable guidance, so the plugin must classify
// them deterministic (propagate untouched, no retry-budget burn). Plain
// third-party string errors like "rate limit exceeded" stay retriable.
func TestIsDeterministicToolError_StructuredRateLimit(t *testing.T) {
	err := apierror.RateLimit(apierror.DomainSubagent, "too many concurrent sub-agents (limit: 4)")
	if !isDeterministicToolError(err) {
		t.Fatal("structured CodeRateLimit error must be deterministic")
	}
	wrapped := fmt.Errorf("spawn subagent: %w", err)
	if !isDeterministicToolError(wrapped) {
		t.Fatal("wrapped CodeRateLimit error must be deterministic")
	}
}

// TestBuildReflectHint_Unchanged keeps the hint format stable.
func TestBuildReflectHint_Unchanged(t *testing.T) {
	hint := buildReflectHint("web_search", "timeout", 2, 3)
	if !strings.Contains(hint, "web_search") || !strings.Contains(hint, "2/3") {
		t.Fatalf("hint format drifted: %q", hint)
	}
}
