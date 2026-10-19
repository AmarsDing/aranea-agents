package biz

import (
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// 压缩阈值跨机制相容性校验（P2-3，2026-10-19）。
//
// 背景：压缩相关比例旋钮由三个（实际四个）机制消费，口径各异——
//   - M1 框架 ContextCompaction（trpc-agent-go，vendored 只读）：L0SummaryThreshold
//     × provider catalog window（llmflow.go:2114，Aranea 未设 WithModelContextWindow）；tiktoken 计数。
//   - M2 产品终审闸门（context_compression_inject.go，BeforeModel p9）：HardTriggerRatio
//     × 256K 产品窗口（conversational 钳 64K）；chars 估算计数。
//   - M3 会话压缩器（session/compressor.go，inter-turn）：Soft/HardTriggerRatio
//     × effective_budget（256K − ToolsProfile 预留 − buffer），provider usage 计数。
//   - M4 turn 准入压力门（turn_admission.go:195）：同一个 L0SummaryThreshold 字段，
//     但分母为 provider 上报 prompt tokens / 256K。
//
// 同一字段多头消费 ⇒ 必须在写入侧守住相容性，否则两类静默失真：
//  1. soft >= hard：M3 压缩器迟滞带退化（compress_policy.go urgency 数学假设 soft<hard）。
//  2. l0_summary_threshold > hard_trigger_ratio：M4 准入门会在 M2 终审闸触发之前
//     就挡住 turn——「压缩永远追不上准入」反模式。此方向仅告警不拒绝
//    （存量行可能合法依赖该行为；拒绝会破坏向后兼容）。
//
// 数值 0 表示「未设置 → 走默认」（agent_defaults.go:51 / agent_types.go:19-22）。
func ValidateCompressionThresholds(s *AgentRuntimeSettings, lg loggateway.Logger) error {
	if s == nil {
		return nil
	}
	for _, f := range []struct {
		name  string
		value float64
	}{
		{"soft_trigger_ratio", s.SoftTriggerRatio},
		{"hard_trigger_ratio", s.HardTriggerRatio},
		{"l0_summary_threshold", s.L0SummaryThreshold},
	} {
		if f.value < 0 || f.value > 1 {
			return apierror.BadRequest("AGENT",
				"%s (%.3f) must be 0 (unset → default) or within (0,1]", f.name, f.value)
		}
	}
	if s.SoftTriggerRatio > 0 && s.HardTriggerRatio > 0 && s.SoftTriggerRatio >= s.HardTriggerRatio {
		return apierror.BadRequest("AGENT",
			"soft_trigger_ratio (%.3f) must be < hard_trigger_ratio (%.3f): compressor hysteresis band degenerates otherwise",
			s.SoftTriggerRatio, s.HardTriggerRatio)
	}
	// 跨语义倒置只告警。有效值口径：0 → 默认（与运行时解析一致）。
	if lg != nil {
		effL0 := s.L0SummaryThreshold
		if effL0 <= 0 {
			effL0 = 0.6 // agent_defaults.go:51 默认
		}
		effHard := s.HardTriggerRatio
		if effHard <= 0 {
			effHard = DefaultHardTriggerRatio
		}
		if effL0 > effHard {
			lg.Warn("compression thresholds cross-semantic inversion: turn-admission gate (l0_summary_threshold/256K) may block turns before the intra-turn hard gate (hard_trigger_ratio×256K) fires",
				loggateway.StepID("agent.compression_thresholds.inverted"),
				loggateway.Float64("l0_summary_threshold", effL0),
				loggateway.Float64("hard_trigger_ratio", effHard))
		}
	}
	return nil
}
