package biz

import "strings"

// ─── P2-5 思考强度路由（DeepSeek V4 effort 分档对齐）─────────────────────────
//
// 按任务复杂度选 thinking 档：简单=off/low、日常=high、复杂=max。
// 复杂度复用既有 ComplexityLevel（QuickAssess 产出），显式给出时覆盖 agent
// 静态 ReasoningLevel（与 P2-1 级联覆盖成员模型路由插件同一约定：显式路由
// 策略优先）；未给复杂度时回落静态档。

// ThinkingEffort 分档。与框架 validReasoningEfforts（low/medium/high/max）
// 对齐，另加 off = 显式关闭 thinking（映射 ThinkingEnabled=false）。
const (
	ThinkingEffortOff    = "off"
	ThinkingEffortLow    = "low"
	ThinkingEffortMedium = "medium"
	ThinkingEffortHigh   = "high"
	ThinkingEffortMax    = "max"
)

// NormalizeThinkingEffort 归一化档位字符串（trim + lowercase + 校验）。
// 非法/空输入返回 ""（= 不覆盖，保留默认行为）。
func NormalizeThinkingEffort(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case ThinkingEffortOff:
		return ThinkingEffortOff
	case ThinkingEffortLow:
		return ThinkingEffortLow
	case ThinkingEffortMedium:
		return ThinkingEffortMedium
	case ThinkingEffortHigh:
		return ThinkingEffortHigh
	case ThinkingEffortMax:
		return ThinkingEffortMax
	}
	return ""
}

// ReasoningModeCustom 是 agent 静态推理策略的「自定义」模式（reasoning_mode）。
// 仅自定义模式下 ReasoningLevel 才注入请求；provider_default = 跟随厂商，
// 不注入任何 thinking 参数（保留 provider 服务端默认）。
const ReasoningModeCustom = "custom"

// StaticThinkingEffort 把 agent 静态推理策略（mode, level）折算为有效 thinking
// 档。返回 "" = 不注入（跟随厂商 / 未配置 / 非法档位）。
//
// 注意：存量 agent 默认（provider_default + off）经此折算为 ""——零行为变化；
// 只有用户显式选择「自定义 + 档位」才会落地 thinking 参数。
func StaticThinkingEffort(mode, level string) string {
	if strings.ToLower(strings.TrimSpace(mode)) != ReasoningModeCustom {
		return ""
	}
	return NormalizeThinkingEffort(level)
}

// ResolveThinkingEffort 把（agent 静态 ReasoningLevel, 任务复杂度）映射为
// 有效 thinking 档。返回 "" 表示不做任何覆盖（请求不携带 thinking 参数，
// 保留 provider 服务端默认）。
//
// level 为空（未判定）时回落归一化后的静态档。
func ResolveThinkingEffort(staticLevel string, level ComplexityLevel) string {
	static := NormalizeThinkingEffort(staticLevel)
	switch level {
	case ComplexitySimple:
		// 简单任务压掉长思考；高档静态配置保留 low（对齐 off/low 区间）。
		if static == ThinkingEffortHigh || static == ThinkingEffortMax {
			return ThinkingEffortLow
		}
		return ThinkingEffortOff
	case ComplexityModerate:
		return ThinkingEffortHigh
	case ComplexityComplex:
		return ThinkingEffortMax
	default:
		return static
	}
}
