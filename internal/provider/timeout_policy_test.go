package provider

import (
	"context"
	"testing"
	"time"
)

func TestNewTimeoutPolicy_Defaults(t *testing.T) {
	p := NewTimeoutPolicy()

	cases := []struct {
		name     string
		taskType TaskType
		want     time.Duration
	}{
		{"simple returns 30min", TaskTypeSimple, 30 * time.Minute},
		{"moderate returns 60min", TaskTypeModerate, 60 * time.Minute},
		{"complex returns 120min", TaskTypeComplex, 120 * time.Minute},
		{"graph_node returns 60min", TaskTypeGraphNode, 60 * time.Minute},
		{"code_gen returns 90min", TaskTypeCodeGen, 90 * time.Minute},
		{"unknown returns 90min fallback", TaskTypeUnknown, 90 * time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.TimeoutFor(tc.taskType); got != tc.want {
				t.Errorf("TimeoutFor(%q) = %v, want %v", tc.taskType, got, tc.want)
			}
		})
	}
}

func TestTimeoutPolicy_UnknownTaskTypeReturnsFallback(t *testing.T) {
	p := NewTimeoutPolicy()
	// 未注册的 TaskType 应回退到 unknown 兜底值（90min）。
	got := p.TimeoutFor(TaskType("nonexistent"))
	if got != 90*time.Minute {
		t.Errorf("TimeoutFor(nonexistent) = %v, want 90min (unknown fallback)", got)
	}
}

func TestTimeoutPolicy_MaxTimeoutTruncation(t *testing.T) {
	// 配置一个超过 maxTimeout（120min）的值，应被截断。
	p := NewTimeoutPolicy().WithTimeout(TaskTypeComplex, 180*time.Minute)
	got := p.TimeoutFor(TaskTypeComplex)
	if got != 120*time.Minute {
		t.Errorf("TimeoutFor(complex) with 180min config = %v, want 120min (max truncation)", got)
	}

	// 验证 maxTimeout 本身不被截断。
	p2 := NewTimeoutPolicy().WithTimeout(TaskTypeComplex, 120*time.Minute)
	got2 := p2.TimeoutFor(TaskTypeComplex)
	if got2 != 120*time.Minute {
		t.Errorf("TimeoutFor(complex) with 120min config = %v, want 120min (at max, no truncation)", got2)
	}
}

func TestTimeoutPolicy_WithTimeout_OverridesDefault(t *testing.T) {
	p := NewTimeoutPolicy().WithTimeout(TaskTypeSimple, 15*time.Minute)
	got := p.TimeoutFor(TaskTypeSimple)
	if got != 15*time.Minute {
		t.Errorf("TimeoutFor(simple) after WithTimeout = %v, want 15min", got)
	}

	// 验证其他 TaskType 不受影响。
	gotModerate := p.TimeoutFor(TaskTypeModerate)
	if gotModerate != 60*time.Minute {
		t.Errorf("TimeoutFor(moderate) = %v, want 60min (unchanged)", gotModerate)
	}
}

func TestTimeoutPolicy_WithTimeout_Chaining(t *testing.T) {
	p := NewTimeoutPolicy().
		WithTimeout(TaskTypeSimple, 10*time.Minute).
		WithTimeout(TaskTypeComplex, 90*time.Minute).
		WithTimeout(TaskTypeCodeGen, 45*time.Minute)

	if got := p.TimeoutFor(TaskTypeSimple); got != 10*time.Minute {
		t.Errorf("TimeoutFor(simple) = %v, want 10min", got)
	}
	if got := p.TimeoutFor(TaskTypeComplex); got != 90*time.Minute {
		t.Errorf("TimeoutFor(complex) = %v, want 90min", got)
	}
	if got := p.TimeoutFor(TaskTypeCodeGen); got != 45*time.Minute {
		t.Errorf("TimeoutFor(code_gen) = %v, want 45min", got)
	}
	// 未覆盖的 moderate 保持默认。
	if got := p.TimeoutFor(TaskTypeModerate); got != 60*time.Minute {
		t.Errorf("TimeoutFor(moderate) = %v, want 60min (default)", got)
	}
}

func TestTimeoutPolicy_NilReceiver(t *testing.T) {
	// nil receiver 应返回 defaultFallbackTimeout（防御性编程，红线 #26）。
	var p *TimeoutPolicy
	got := p.TimeoutFor(TaskTypeSimple)
	if got != defaultFallbackTimeout {
		t.Errorf("nil TimeoutPolicy.TimeoutFor() = %v, want %v", got, defaultFallbackTimeout)
	}
}

func TestTimeoutPolicy_MaxTimeoutAccessor(t *testing.T) {
	p := NewTimeoutPolicy()
	if got := p.MaxTimeout(); got != 120*time.Minute {
		t.Errorf("MaxTimeout() = %v, want 120min", got)
	}
}

func TestTimeoutPolicy_DefaultTimeoutAccessor(t *testing.T) {
	p := NewTimeoutPolicy()
	if got := p.DefaultTimeout(); got != 30*time.Minute {
		t.Errorf("DefaultTimeout() = %v, want 30min", got)
	}
}

func TestTimeoutPolicy_WithTimeout_BelowMaxNotTruncated(t *testing.T) {
	// 配置低于 maxTimeout 的值不应被截断。
	p := NewTimeoutPolicy().WithTimeout(TaskTypeSimple, 5*time.Minute)
	got := p.TimeoutFor(TaskTypeSimple)
	if got != 5*time.Minute {
		t.Errorf("TimeoutFor(simple) with 5min config = %v, want 5min (below max, no truncation)", got)
	}
}

// --- Context-based TaskType propagation tests ---

func TestWithTaskType_AndTaskTypeFromCtx(t *testing.T) {
	ctx := context.Background()
	ctx = WithTaskType(ctx, TaskTypeCodeGen)

	got := TaskTypeFromCtx(ctx)
	if got != TaskTypeCodeGen {
		t.Errorf("TaskTypeFromCtx() = %q, want %q", got, TaskTypeCodeGen)
	}
}

func TestTaskTypeFromCtx_DefaultsToUnknown(t *testing.T) {
	ctx := context.Background()
	got := TaskTypeFromCtx(ctx)
	if got != TaskTypeUnknown {
		t.Errorf("TaskTypeFromCtx() with no value = %q, want %q", got, TaskTypeUnknown)
	}
}

func TestTaskTypeFromCtx_NilContext(t *testing.T) {
	got := TaskTypeFromCtx(nil)
	if got != TaskTypeUnknown {
		t.Errorf("TaskTypeFromCtx(nil) = %q, want %q", got, TaskTypeUnknown)
	}
}

func TestWithTaskType_NilContext(t *testing.T) {
	// WithTaskType(nil, ...) 不应 panic，应使用 context.Background()。
	ctx := WithTaskType(nil, TaskTypeComplex)
	if got := TaskTypeFromCtx(ctx); got != TaskTypeComplex {
		t.Errorf("TaskTypeFromCtx after WithTaskType(nil,...) = %q, want %q", got, TaskTypeComplex)
	}
}

func TestApplyTimeoutFromCtx_WithTaskType(t *testing.T) {
	p := NewTimeoutPolicy()
	ctx := WithTaskType(context.Background(), TaskTypeCodeGen)

	newCtx, cancel, timeout := p.ApplyTimeoutFromCtx(ctx)
	if cancel == nil {
		t.Fatal("cancel must not be nil when TaskType is set")
	}
	defer cancel()

	if timeout != 90*time.Minute {
		t.Errorf("timeout = %v, want 90min (code_gen default)", timeout)
	}
	if newCtx == nil {
		t.Fatal("newCtx must not be nil")
	}
	// 验证 context 有 deadline。
	if _, ok := newCtx.Deadline(); !ok {
		t.Error("newCtx should have a deadline")
	}
}

func TestApplyTimeoutFromCtx_WithoutTaskType(t *testing.T) {
	p := NewTimeoutPolicy()
	ctx := context.Background()

	newCtx, cancel, timeout := p.ApplyTimeoutFromCtx(ctx)
	if cancel != nil {
		t.Fatal("cancel should be nil when no TaskType is set")
	}
	if timeout != 0 {
		t.Errorf("timeout = %v, want 0 (no TaskType)", timeout)
	}
	if newCtx != ctx {
		t.Error("newCtx should be the same as input ctx when no TaskType")
	}
}

func TestApplyTimeoutFromCtx_NilPolicy(t *testing.T) {
	var p *TimeoutPolicy
	ctx := WithTaskType(context.Background(), TaskTypeComplex)

	newCtx, cancel, timeout := p.ApplyTimeoutFromCtx(ctx)
	if cancel != nil {
		t.Fatal("cancel should be nil for nil policy")
	}
	if timeout != 0 {
		t.Errorf("timeout = %v, want 0 (nil policy)", timeout)
	}
	if newCtx != ctx {
		t.Error("newCtx should be the same as input ctx for nil policy")
	}
}

func TestApplyTimeoutFromCtx_RespectsMaxTimeout(t *testing.T) {
	// 配置超过 maxTimeout 的值，ApplyTimeoutFromCtx 应使用截断后的值。
	p := NewTimeoutPolicy().WithTimeout(TaskTypeComplex, 180*time.Minute)
	ctx := WithTaskType(context.Background(), TaskTypeComplex)

	_, cancel, timeout := p.ApplyTimeoutFromCtx(ctx)
	if cancel == nil {
		t.Fatal("cancel must not be nil")
	}
	defer cancel()

	if timeout != 120*time.Minute {
		t.Errorf("timeout = %v, want 120min (max truncation)", timeout)
	}
}
