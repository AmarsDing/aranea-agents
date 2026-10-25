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
// 询判级：静态阈值臂（softTriggerTokens/hardTriggerTokens + MaxTriggerTokens
// 封顶），不上库、无写副作用——刻意不取 adaptive buffer 内存态：
// getAdaptiveBufferRatio 是写函数（播种/推进 ConsecutiveLowCount 且对
// AdaptiveBufferState 做无锁读写），只允许 runCompress 的飞行串行区访问
// （D4-f 评审 ws_0699f3d1 #1/#2/#7/#8）。adaptive 开启的 agent 下本判级为
// 静态近似（以策略 bufferRatio 代 adaptive CurrentRatio，方向保守可查），
// 权威判级恒为 runCompress 每轮的 compressTriggerThresholds。
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
	p := CompressPolicyFromAgent(ag)
	softTok, hardTok := capTriggerTokens(p, softTriggerTokens(ag, window), hardTriggerTokens(ag, window))
	rep.SoftTokens, rep.HardTokens = softTok, hardTok
	switch {
	case hardTok > 0 && rep.UsedTokens >= hardTok:
		rep.Verdict = biz.DurablePrecheckHardBand
	case softTok > 0 && rep.UsedTokens >= softTok:
		rep.Verdict = biz.DurablePrecheckSoftBand
	}
	return rep
}
