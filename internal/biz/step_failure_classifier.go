package biz

import "strings"

// step_failure_classifier.go —— L4 失败分类器规则版（P2-3，2026-09-03）。
//
// 背景：F2 把所有 team 执行失败（team_failed）一律视为可重试，但能力缺失
// （agent/API key/模型不存在、权限不足）与语义错误（输入校验、内容安全拦截）
// 重试不会改变结果，只会推迟 cascade 对账信号并浪费一轮 5s 退避 + 完整团队
// 重跑。本分类器按错误消息子串规则把失败分四类，调用方（plan_executor 的
// failStepWith）据此决定是否消耗自动重试预算。
//
// D1-F3（87-orchestration-chain-hardening，2026-10-21）：新增 cancel 类——
// 用户取消信号（"context canceled"/"user_cancel"/"team_cancel" 等）沿
// status=cancelled 回调的 errMsg 原文到达，重试只会把用户已取消的团队再
// 跑一遍。注意 "deadline exceeded" 维持 transient（TeamMaxLifetime 4h 墙钟
// 属 infra 超时而非用户取消，F2 恢复语义不变）。
//
// 规则版边界：仅匹配消息文本，不解析结构化错误码（team 链路目前只透传
// ErrorMsg 字符串）；数字状态码按裸子串匹配（"429"/"503" 等），3 位数字在
// 普通错误文本中误伤率极低，可接受。未知消息默认 transient（保持 F2 既有
// 重试语义，fail-open）。

// StepFailureClass 是 step 失败的 L4 分类。
type StepFailureClass string

const (
	// StepFailureTransient 瞬时故障：网络抖动、LLM 首字节超时、限流、
	// 上游 5xx——值得消耗重试预算。
	StepFailureTransient StepFailureClass = "transient"
	// StepFailureCapability 能力缺失：agent/密钥/模型/权限不存在或不可用——
	// 重试不会改变结果，直接 cascade 快速失败。
	StepFailureCapability StepFailureClass = "capability"
	// StepFailureSemantic 语义错误：输入校验失败、内容安全/护栏拦截——
	// 同样不可重试。
	StepFailureSemantic StepFailureClass = "semantic"
	// StepFailureCancel 用户取消（D1-F3）：team 被用户主动取消
	// （status=cancelled 回调透传的 context canceled / user_cancel /
	// team_cancel 等）。自动重跑违背用户意图——不重试、不消耗重试预算，
	// 下游经 cascadeSkip 收敛（可被 ResumePlanBoard 复活）。
	StepFailureCancel StepFailureClass = "cancel"
)

// 匹配优先级：cancel → capability → semantic → transient → 默认 transient。
// 全部小写，匹配前对消息 lowercase。
var stepFailureCancelPatterns = []string{
	"context canceled", "context cancelled",
	"user_cancel", "user_cancelled",
	"team_cancel", "team_cancelled",
}

var stepFailureCapabilityPatterns = []string{
	"agent keys not found", "agent not found", "no such agent",
	"api key", "apikey", "unauthorized", "401", "403", "forbidden",
	"permission denied", "model not found", "no such model",
	"invalid model", "unsupported model", "insufficient_quota",
	"quota exceeded", "tool not found", "not configured",
}

var stepFailureSemanticPatterns = []string{
	"invalid input", "validation failed", "content filter", "content_filter",
	"guardrail", "safety", "policy violation", "malformed",
}

var stepFailureTransientPatterns = []string{
	"timeout", "deadline exceeded", "connection reset", "connection refused",
	"rate limit", "429", "502", "503", "504", "overloaded",
	"temporarily unavailable", "eof", "stream error", "首字节",
}

// ClassifyStepFailure 对 team 执行失败消息做规则分类。空消息/未知消息
// 返回 transient（fail-open，保持 F2 语义）。
// 匹配优先级（D1-F3）：cancel → capability → semantic → transient → 默认。
func ClassifyStepFailure(msg string) StepFailureClass {
	m := strings.ToLower(msg)
	for _, p := range stepFailureCancelPatterns {
		if strings.Contains(m, p) {
			return StepFailureCancel
		}
	}
	for _, p := range stepFailureCapabilityPatterns {
		if strings.Contains(m, p) {
			return StepFailureCapability
		}
	}
	for _, p := range stepFailureSemanticPatterns {
		if strings.Contains(m, p) {
			return StepFailureSemantic
		}
	}
	for _, p := range stepFailureTransientPatterns {
		if strings.Contains(m, p) {
			return StepFailureTransient
		}
	}
	return StepFailureTransient
}
