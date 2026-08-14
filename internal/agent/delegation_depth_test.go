package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// P1-4：委派深度上限必须可配（ARANEA_MAX_DELEGATE_DEPTH），默认 3，
// 非法值回落默认。与 subagent maxConcurrent 的 env 语义一致
// （进程启动时定值，进程生命周期内不变）。
func TestResolveMaxDelegateDepth(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv(envMaxDelegateDepth, "")
		if got := resolveMaxDelegateDepth(); got != defaultMaxDelegateDepth {
			t.Fatalf("resolveMaxDelegateDepth() = %d, want %d", got, defaultMaxDelegateDepth)
		}
	})
	t.Run("override", func(t *testing.T) {
		t.Setenv(envMaxDelegateDepth, "4")
		if got := resolveMaxDelegateDepth(); got != 4 {
			t.Fatalf("resolveMaxDelegateDepth() = %d, want 4", got)
		}
	})
	t.Run("invalid falls back", func(t *testing.T) {
		t.Setenv(envMaxDelegateDepth, "abc")
		if got := resolveMaxDelegateDepth(); got != defaultMaxDelegateDepth {
			t.Fatalf("resolveMaxDelegateDepth() = %d, want %d", got, defaultMaxDelegateDepth)
		}
	})
	t.Run("zero falls back", func(t *testing.T) {
		t.Setenv(envMaxDelegateDepth, "0")
		if got := resolveMaxDelegateDepth(); got != defaultMaxDelegateDepth {
			t.Fatalf("resolveMaxDelegateDepth() = %d, want %d", got, defaultMaxDelegateDepth)
		}
	})
}

// P1-4：agent-as-tool 派生在深度达到上限时必须 fail-loud 拒绝（不得静默
// 截断）。深度经 ctx 传递；depth==limit 即拒绝（下一次派生将是 limit+1 层）。
func TestBuildAgentAsTool_RejectsAtDepthLimit(t *testing.T) {
	ctx := withDelegationDepth(context.Background(), maxDelegateDepth())
	// nil matcher 即可——深度守卫必须先于任何 matcher/deps 使用。
	_, err := BuildAgentAsTool(ctx, nil, TRPCBuilderDeps{}, loggateway.NewNoop(), "task", nil)
	if err == nil {
		t.Fatal("expected depth-limit error, got nil")
	}
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("err code = %v, want BadRequest", err)
	}
	if !strings.Contains(err.Error(), "delegation depth") {
		t.Fatalf("error must name the depth limit: %v", err)
	}
}

// P1-4：transfer 链深度超限必须 fail-loud 拒绝。控制器按 run 计数，
// 默认上限内放行、上限+1 拒绝。
func TestTransferController_RejectsBeyondLimit(t *testing.T) {
	c := NewTransferController(loggateway.NewNoop())
	limit := maxDelegateDepth()
	for i := 1; i <= limit; i++ {
		timeout, err := c.OnTransfer(context.Background(), "a", "b")
		if err != nil {
			t.Fatalf("transfer #%d within limit rejected: %v", i, err)
		}
		if timeout != transferTargetTimeout {
			t.Fatalf("transfer #%d timeout = %v, want %v", i, timeout, transferTargetTimeout)
		}
	}
	_, err := c.OnTransfer(context.Background(), "a", "b")
	if err == nil {
		t.Fatalf("transfer #%d beyond limit %d must be rejected", limit+1, limit)
	}
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("err code = %v, want Forbidden", err)
	}
}

// 深度 ctx 存取的往返一致性（守卫正确性的前提）。
func TestDelegationDepthCtxRoundTrip(t *testing.T) {
	if got := delegationDepthFromCtx(context.Background()); got != 0 {
		t.Fatalf("unset depth = %d, want 0", got)
	}
	ctx := withDelegationDepth(context.Background(), 2)
	if got := delegationDepthFromCtx(ctx); got != 2 {
		t.Fatalf("depth = %d, want 2", got)
	}
}

// 防回归：transferTargetTimeout 必须为正（OnTransfer 返回值契约）。
func TestTransferTargetTimeoutPositive(t *testing.T) {
	if transferTargetTimeout <= 0 {
		t.Fatalf("transferTargetTimeout = %v, want > 0", transferTargetTimeout)
	}
	if transferTargetTimeout > 10*time.Minute {
		t.Fatalf("transferTargetTimeout = %v, unreasonably large", transferTargetTimeout)
	}
}
