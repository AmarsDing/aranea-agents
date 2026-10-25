// Package conf provides runtime configuration helpers.
package conf

import (
	"os"
	"strings"
)

// D4 prompts 估算 / durable 压缩预检 就绪 flip（deferred 战役 2026-10-25；
// F1 先登记后写码——docs/development/feature-flags-registry.md §1「D4
// 估算/预检就绪 flip」，成果设计 docs/development/87 三件套）。就绪不翻：
// 缺省保持 shadow / observe，行为零变化。
//
// conf 包保持零 loggateway 依赖——访问器纯读取：坏值回落缺省并返回
// valid=false，由 wire 侧翻译闭包用注入的 Logger 落一次性 Warn。

// PromptEstMode reports ARANEA_PROMPT_EST_MODE ∈ {legacy, shadow, segmented}：
//   - legacy：prompt/message 级估算走旧实现（混合臂 + CJK 兜底）。
//   - shadow（缺省/空值回落）：消费 legacy 值，另每 prompt 级调用落一条
//     prompt.est_shadow 对照日志（legacy_est/segmented_est/delta）；message
//     级不落影子（防逐消息循环伪造整片调用毒化对比脚本，P0-3① 冻结契约）。
//   - segmented：真开关——prompt 级走 SegmentedCalibrator.Estimate，
//     message 级逐消息 rune 类分段 ceil。翻牌成对：prompt 级与 message 级
//     同开关一起切（Σ per-message ceil 恒 ≥ 整片 ceil，保守方向，记档 87）。
//
// 返回 (mode, valid)：空值回落 shadow 且 valid=true；未知值回落 shadow、
// valid=false（wire 侧据此落 Warn）。
func PromptEstMode() (mode string, valid bool) {
	switch v := strings.ToLower(strings.TrimSpace(os.Getenv("ARANEA_PROMPT_EST_MODE"))); v {
	case "":
		return "shadow", true
	case "legacy", "shadow", "segmented":
		return v, true
	default:
		return "shadow", false
	}
}

// DurablePrecheckMode reports ARANEA_DURABLE_PRECHECK ∈ {off, observe, on}：
//   - off：双落点全静默。
//   - observe（缺省/空值回落）：双落点只观测不动 compressor——① admitTurn
//     用上轮 sess.ContextUsedTokens 权威值落 session.compress.precheck
//     步骤（used/soft/hard/verdict，零额外查询）；② p8 装配对照落步
//     assembly.compress.precheck（join 键 runID）。
//   - on：正常触发语义——verdict 越过 soft 时调
//     Compressor.AfterNativeTurn（flight-guarded、非 forced、不绕
//     suppress，与原生轮后触发完全一致），补 durable 轮缺压缩预检的缺口。
//
// 返回 (mode, valid)：口径同 PromptEstMode。
func DurablePrecheckMode() (mode string, valid bool) {
	switch v := strings.ToLower(strings.TrimSpace(os.Getenv("ARANEA_DURABLE_PRECHECK"))); v {
	case "":
		return "observe", true
	case "off", "observe", "on":
		return v, true
	default:
		return "observe", false
	}
}
