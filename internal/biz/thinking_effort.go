package biz

import "strings"

// ─── P2-5 思考强度路由（DeepSeek V4 effort 分档对齐）─────────────────────────
//
// 按任务复杂度选 thinking 档：简单=off/low、日常=high、复杂=max。
// 复杂度显式给出时覆盖 agent 静态 ReasoningLevel（与 P2-1 级联覆盖成员
// 模型路由插件同一约定：显式路由策略优先）；未给复杂度时回落静态档。

// ThinkingEffort 分档。与框架 validReasoningEfforts（low/medium/high/max）
// 对齐，另加 off = 显式关闭 thinking（映射 ThinkingDisabled）。
const (
	ThinkingEffortOff    = "off"
	ThinkingEffortLow    = "low"
	ThinkingEffortMedium = "medium"
	ThinkingEffortHigh   = "high"
	ThinkingEffortMax    = "max"
)

// ThinkingComplexity 是调用方对单次 LLM 任务的复杂度判定。
type ThinkingComplexity string

const (
	// ComplexityUnspecified 未判定——回落 agent 静态 ReasoningLevel。
	ComplexityUnspecified ThinkingComplexity = ""
	// ComplexitySimple 简单旁路任务（提取/改写/重排）→ off/low。
	ComplexitySimple ThinkingComplexity = "simple"
	// ComplexityRoutine 日常 agent 任务 → high。
	ComplexityRoutine ThinkingComplexity = "routine"
	// ComplexityComplex 复杂规划/评估任务 → max。
	ComplexityComplex ThinkingComplexity = "complex"
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

// ResolveThinkingEffort 把（agent 静态 ReasoningLevel, 任务复杂度）映射为
// 有效 thinking 档。返回 "" 表示不做任何覆盖（请求不携带 thinking 参数，
// 保留 provider 服务端默认）。
func ResolveThinkingEffort(staticLevel string, complexity ThinkingComplexity) string {
	static := NormalizeThinkingEffort(staticLevel)
	switch complexity {
	case ComplexitySimple:
		// 简单任务压掉长思考；高档静态配置保留 low（对齐 off/low 区间）。
		if static == ThinkingEffortHigh || static == ThinkingEffortMax {
			return ThinkingEffortLow
		}
		return ThinkingEffortOff
	case ComplexityRoutine:
		return ThinkingEffortHigh
	case ComplexityComplex:
		return ThinkingEffortMax
	default:
		return static
	}
}
