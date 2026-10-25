package session

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/llmcontext"
)

// D4-b durable 压缩预检（deferred 战役 2026-10-25，就绪不翻；F1 登记
// feature-flags-registry §1「D4 估算/预检就绪 flip」）。
//
// 落地座席：
//  1. seat①（service admitTurn）：mode≠off 时对本步判级落
//     session.compress.precheck 观测步骤；mode=on 且判级越软带时调用
//     AfterNativeTurn（正常触发语义：flight-guarded、非 forced、不绕
//     suppress），补 durable 轮绕过原生轮后异步压缩触发的缺口。
//  2. seat②（agent p8 装配对照）：仅 ratio 镜像对照落步（见
//     assembly_budget.go），authoritative 判级恒为本函数。

// PrecheckVerdict 对本会话上一轮权威读数（sess.ContextUsedTokens）做零查
// 询判级：完全复用 compressTriggerThresholds（含 adaptive buffer 内存
// 态、MaxTriggerTokens 封顶），与 runCompress 的触线口径逐点对齐；不上
// 库、无写副作用。
func (c *Compressor) PrecheckVerdict(sess biz.Session, ag biz.Agent) biz.DurablePrecheckReport {
	window := llmcontext.ResolveWindow(llmcontext.ResolveInput{})
	rep := biz.DurablePrecheckReport{
		Verdict:       biz.DurablePrecheckBelowSoft,
		UsedTokens:    sess.ContextUsedTokens,
		ContextWindow: window,
	}
	if c == nil {
		return rep
	}
	softTok, hardTok := c.compressTriggerThresholds(sess.ID, sess, ag, rep.UsedTokens, window)
	rep.SoftTokens, rep.HardTokens = softTok, hardTok
	switch {
	case hardTok > 0 && rep.UsedTokens >= hardTok:
		rep.Verdict = biz.DurablePrecheckHardBand
	case softTok > 0 && rep.UsedTokens >= softTok:
		rep.Verdict = biz.DurablePrecheckSoftBand
	}
	return rep
}
