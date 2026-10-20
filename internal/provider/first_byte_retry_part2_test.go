package provider

import (
	"aranea-agents/pkg/loggateway"
	"context"
	"sync/atomic"
	"testing"
	"time"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestLivenessGuard_LongHealthyStreamNotKilled(t *testing.T) {
	inner := &slowHealthyModel{}
	wrapped := WrapModelWithLivenessGuard(inner, "openai", "gpt-4o", loggateway.NewNoop())

	ctx := WithStallBudget(context.Background(), 100*time.Millisecond, 3)
	out, err := wrapped.GenerateContent(ctx, &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	count := 0
	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-out:
			if !ok {
				goto drained
			}
			count++
		case <-timeout:
			t.Fatal("healthy stream killed or hung")
		}
	}
drained:
	if count != 15 {
		t.Fatalf("forwarded %d events, want 15 (no reconnect interference)", count)
	}
	if n := atomic.LoadInt32(&inner.calls); n != 1 {
		t.Fatalf("inner calls = %d, want 1 (healthy stream never reconnected)", n)
	}
}

func TestStallPolicyFromConfigJSON(t *testing.T) {
	if d, n := StallPolicyFromConfigJSON(""); d != 0 || n != 0 {
		t.Fatalf("empty config = (%v, %d), want (0, 0)", d, n)
	}
	if d, n := StallPolicyFromConfigJSON(`{}`); d != 0 || n != 0 {
		t.Fatalf("empty object = (%v, %d), want (0, 0)", d, n)
	}
	if d, n := StallPolicyFromConfigJSON(`{not json`); d != 0 || n != 0 {
		t.Fatalf("invalid json = (%v, %d), want (0, 0)", d, n)
	}
	d, n := StallPolicyFromConfigJSON(`{"stall_timeout_sec":120,"stall_max_attempts":8}`)
	if d != 120*time.Second || n != 8 {
		t.Fatalf("full config = (%v, %d), want (120s, 8)", d, n)
	}

	d, n = StallPolicyFromConfigJSON(`{"stall_timeout_sec":45}`)
	if d != 45*time.Second || n != 0 {
		t.Fatalf("partial config = (%v, %d), want (45s, 0)", d, n)
	}
}

func TestWithStallBudgetDefaults(t *testing.T) {
	p := stallPolicyFromContext(context.Background())
	if p.timeout != DefaultStallTimeout || p.maxReconnects != DefaultStallMaxReconnects {
		t.Fatalf("absent ctx = (%v, %d), want defaults (%v, %d)", p.timeout, p.maxReconnects, DefaultStallTimeout, DefaultStallMaxReconnects)
	}

	ctx := WithStallBudget(context.Background(), 5*time.Second, 2)
	p = stallPolicyFromContext(ctx)
	if p.timeout != 5*time.Second || p.maxReconnects != 2 {
		t.Fatalf("injected = (%v, %d), want (5s, 2)", p.timeout, p.maxReconnects)
	}

	ctx = WithStallBudget(context.Background(), 0, -1)
	p = stallPolicyFromContext(ctx)
	if p.timeout != DefaultStallTimeout || p.maxReconnects != DefaultStallMaxReconnects {
		t.Fatalf("non-positive fields = (%v, %d), want defaults", p.timeout, p.maxReconnects)
	}
}
