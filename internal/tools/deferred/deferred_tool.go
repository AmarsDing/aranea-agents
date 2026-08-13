package deferred

import (
	"context"
	"fmt"

	"aranea-agents/pkg/loggateway"

	"aranea-agents/pkg/apierror"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// DeferredCallableTool 包装一个已完全装配的工具，提供按需激活门禁。
//
// eager-inner 模式（WP-4 修复版）：
//   - Declaration() 始终返回内部工具的完整声明（含 InputSchema），
//     不再像 factory 模式那样只有名称+描述。
//   - Call() 在未激活时返回结构化错误（提示先 tool_load），
//     激活后直接委托给内部工具。
//   - 内部工具在装配阶段已完全创建并装饰（超时/结果预算/确定性缓存），
//     不存在 lazy factory 的二次初始化问题。
type DeferredCallableTool struct {
	inner trpctool.Tool
	decl  *trpctool.Declaration
	lg    loggateway.Logger
}

// NewDeferredCallableTool 创建一个 eager-inner 的延迟工具包装器。
// inner 必须是已完全装配的工具（含完整 schema 和装饰器）。
func NewDeferredCallableTool(inner trpctool.Tool, lg loggateway.Logger) *DeferredCallableTool {
	decl := inner.Declaration()
	return &DeferredCallableTool{
		inner: inner,
		decl:  decl,
		lg:    lg,
	}
}

func (d *DeferredCallableTool) Declaration() *trpctool.Declaration {
	return d.decl
}

// InnerTool 返回内部工具，供 filter 穿透检查。
// 当 aliasTool 包装 DeferredCallableTool 时，filter 可通过 InnerTool
// 递归找到被 deferred 的底层工具。
func (d *DeferredCallableTool) InnerTool() trpctool.Tool {
	return d.inner
}

// ShouldDefer implements trpctool.DeferredTool.
// 返回 true 表示此工具在 LLM tools block 中应被 ToolFilter 隐藏，
// 直到通过 tool_load 激活。
func (d *DeferredCallableTool) ShouldDefer(_ context.Context) bool {
	return true
}

func (d *DeferredCallableTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	// 门禁：未激活时拒绝执行，引导模型先 tool_load
	if !isActivatedForSession(ctx, d.decl.Name) {
		d.lg.Warn("deferred tool called without activation",
			loggateway.StepID("tool.deferred.not_activated"),
			loggateway.Str("tool", d.decl.Name),
		)
		return nil, apierror.Forbidden(
			apierror.DomainTool,
			fmt.Sprintf("tool %q is not activated. Call tool_load(\"%s\") first to load and activate it.", d.decl.Name, d.decl.Name),
		)
	}

	// 已激活：委托给内部工具
	if callable, ok := d.inner.(trpctool.CallableTool); ok {
		return callable.Call(ctx, jsonArgs)
	}
	return nil, apierror.Internal(apierror.DomainTool, "deferred tool does not implement CallableTool")
}
