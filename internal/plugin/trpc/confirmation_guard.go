package plugintrpc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// productConfirmGateActive 进程级标记：产品确认门禁（tool_confirmation
// BeforeTool hook）所在装配形态置位（server 模式 wire 注入非 nil ToolUC 时）。
//
// 79-runtime-governance（2026-08-27 三轮审查 T3 根修）：vendored 框架执行序为
// runner 插件回调先于 agent 链回调（functioncall.go runBeforeToolPluginCallbacks
// → runBeforeToolCallbacks；上游官方 plugin 文档时序图明载 plugin callbacks
// (global) → agent callbacks (local)，属设计语义非缺陷，上游无重排机制）。
// 因此 ToolConfirmHandled 标记在插件执行时必不存在——confirmation_guard 启用
// 时会整体遮蔽链上产品门禁：param gate deny 失 param_rule_deny 审计、ask 无
// 确认卡（插件硬拦无交互通道）、allow 与「跳过 catalog/plugin 确认」契约冲突。
//
// 让位的语义完备性：产品门禁 decide() 用同一 MatchConfirmationGuard + 同一 DB
// 配置行（PluginManager.ConfirmationGuardConfigForAgent）把插件的
// ConfirmTools/ConfirmPatterns 升级为交互确认（confirmReasonPolicyPlugin，
// 带 session/persisted grant 与确认卡）——插件硬拦的唯一存在理由是「产品门禁
// 未装配」的降级形态（CLI 无 DB 模式），装配后让位不损失任何拦截语义。
var productConfirmGateActive atomic.Bool

// SetProductConfirmGateActive 由装配层（cmd/admin/wire.go，ToolUC 非 nil 的
// server 形态）置位；CLI 模式不调用保持 false，插件保留遗产硬拦
// （fail-closed）。幂等，启动期调用。
func SetProductConfirmGateActive(active bool) { productConfirmGateActive.Store(active) }

// ProductConfirmGateActive 供插件让位判定、测试与诊断读取。
func ProductConfirmGateActive() bool { return productConfirmGateActive.Load() }

type ConfirmationGuardPlugin struct {
	base      basePlugin
	cfg       ConfirmationGuardConfig
	deferOnce sync.Once
}

var _ trpcplugin.Plugin = (*ConfirmationGuardPlugin)(nil)

func NewConfirmationGuardPlugin(p biz.Plugin, stats StatsRecorder, monitorBus contract.MonitorBus, lg loggateway.Logger) *ConfirmationGuardPlugin {
	var cfg ConfirmationGuardConfig
	cfg.DefaultAction = "reject"
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg, lg)
	return &ConfirmationGuardPlugin{base: newBasePlugin(p.Key, stats, monitorBus, lg), cfg: cfg}
}

func (c *ConfirmationGuardPlugin) Name() string { return c.base.Name() }

func (c *ConfirmationGuardPlugin) Register(r *trpcplugin.Registry) {
	r.BeforeTool(c.beforeTool)
}

// beforeTool blocks tool execution when confirmation is required.
//
// E2E-P1-10 unify: when the product confirmation gate already ran (approved,
// allow-without-channel, or explicitly handled), this plugin is a no-op.
// Hard-blocking here only applies when the product gate is NOT installed —
// so there is a single confirmation state machine in practice whenever
// tool_confirmation BeforeTool is wired.
//
// 2026-08-27 三轮审查 T3：框架执行序为插件先于链（上游设计语义），
// ToolConfirmHandled 在本函数执行时必不存在；产品门禁装配标记
// （ProductConfirmGateActive）才是真正的让位依据——两机制叠加后与执行序
// 无关：插件先→装配标记让位；链先→handled 标记让位。
func (c *ConfirmationGuardPlugin) beforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	if args == nil {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	// P1-10: skip when the product callback already handled confirmation.
	if ToolConfirmHandled(ctx) {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	// T3：产品确认门禁已装配的进程形态下本插件完全让位。插件命中的工具
	// 集合恒为产品门禁 decide() 插件分支（同一 MatchConfirmationGuard +
	// 同一 DB 配置行）处理集合的子集，且被升级为交互确认——此处硬拦只会
	// 遮蔽 param gate 的 deny 审计 / ask 确认卡 / allow 放行。
	if ProductConfirmGateActive() {
		c.deferOnce.Do(func() {
			c.base.logger.Info("plugin.confirmation_guard.defer_to_product_gate",
				"tool", args.ToolName,
				"reason", "product confirmation gate installed; plugin hard-block disabled (single confirmation state machine)")
		})
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	if MatchConfirmationGuard(c.cfg, args.ToolName, args.Arguments) {
		c.base.logger.Info("plugin.confirmation_guard.before_tool",
			"status", "blocked",
			"tool", args.ToolName,
			"default_action", c.cfg.DefaultAction,
		)
		c.base.recordEvent(ctx, "before_tool", "blocked",
			fmt.Sprintf("tool %s 需要确认（default_action=%s，未接入交互确认门）", args.ToolName, c.cfg.DefaultAction))
		msg := fmt.Sprintf("confirmation_guard: tool %q requires confirmation (enable product confirm gate for interactive approval)", args.ToolName)
		return &trpctool.BeforeToolResult{
			Context:      ctx,
			CustomResult: map[string]any{"error": msg, "blocked": true, "needs_confirm": true},
		}, nil
	}
	c.base.logger.Info("plugin.confirmation_guard.before_tool",
		"status", "success",
		"tool", args.ToolName,
		"needs_confirm", false,
	)
	c.base.record(ctx, "before_tool", "success")
	return &trpctool.BeforeToolResult{Context: ctx}, nil
}
