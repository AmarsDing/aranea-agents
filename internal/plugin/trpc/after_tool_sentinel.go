package plugintrpc

import (
	"context"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// afterToolSentinelPlugin 补偿上游出库后 tool.Callbacks 的行为差：
//
// 上游 tool/callbacks.go finalizeAfterToolResult 在整条插件链无任何回调
// 产出结果时，会把工具原始结果（args.Result）包成 CustomResult 返回；
// functioncall/state_graph 处理器将非 nil CustomResult 视为插件覆写并
// 提前返回，从而跳过 agent 级 AfterTool 链（循环守卫 afterHook、工具
// 调用记录器等全部失效——原 fork 的修复点，见 fork tool/callbacks.go
// finalizeAfterToolResult 注释）。
//
// 本插件注册一个恒返回非 nil 空结果的 AfterTool 哨兵，使链尾
// lastResult != nil，阻断捏造路径；它必须注册在全部业务插件之后
// （RunnerPluginsForAgent 末尾追加）。哨兵本身不改写任何结果。
type afterToolSentinelPlugin struct{}

// NewAfterToolSentinelPlugin 返回 AfterTool 哨兵插件。
func NewAfterToolSentinelPlugin() trpcplugin.Plugin {
	return afterToolSentinelPlugin{}
}

func (afterToolSentinelPlugin) Name() string { return "aranea_after_tool_sentinel" }

func (afterToolSentinelPlugin) Register(r *trpcplugin.Registry) {
	r.AfterTool(func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		return &trpctool.AfterToolResult{}, nil
	})
}
