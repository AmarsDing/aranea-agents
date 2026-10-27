package data

import (
	"context"
	"crypto/sha1"
	"encoding/hex"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/decide"
)

// memory_eval_shadow.go — JEV Phase 0 / T8 shadow 审计链挂接（2026-10-28）。
//
// 两个生产决策点的现状裁决双记（零行为影响：只在既有判定之后补 Emit，
// 不改任何判定结果）：
//   - 读侧 route_mode：SearchMemories 的 VIR 路由（applyEvalQueryModes 之后），
//     记录五读侧意图的原始 margin + 词法分析前置模式（P29 分歧入料）；
//   - 写侧 correction_gate：supersedeContradictions 的 VIR correction 门，
//     记录 margin/阈值与词法 cue 共存态。
//
// forget_gate 的 VIR 产线挂接当前不存在（EvalIntentForget 仅有 biz 测试
// 消费，eval store 未接线）——本文件不加新判定路径，该缺口记台账。

// SetShadowRecorder wires the Phase-0 shadow audit recorder (T8). Nil
// disables; the store keeps byte-identical behavior either way. Implements
// decide.ShadowRecorderWired (defined on the consumer side).
func (s *evalMemoryStore) SetShadowRecorder(r *decide.Recorder) { s.shadow = r }

// evalRouteModeIntents 是读侧参与 route_mode 路由的 VIR 意图集（写侧
// correction/forget 不在其列）。顺序即 questionsets.json route_mode
// criteria 的读侧子集序。
var evalRouteModeIntents = []biz.EvalIntent{
	biz.EvalIntentChronological,
	biz.EvalIntentAggregate,
	biz.EvalIntentEnumerative,
	biz.EvalIntentRelational,
	biz.EvalIntentTrend,
}

// shadowRouteMode 双记读侧路由决策：五个读侧意图的原始 margin 全量入
// Scores（harvest 重建软分布用），Key 取达阈值的最优意图、全不达则
// no_recall（VIR 的盲区按问题集选项编码，P29 分歧量化即此）。lexical
// 是 VIR 合并前的词法判定快照——分析词法/向量分歧的关键输入。
func (s *evalMemoryStore) shadowRouteMode(ctx context.Context, userID, query string, embedding []float32, lexical biz.EvalQueryAnalysis) {
	if s.shadow == nil || !s.shadow.Enabled() || len(embedding) == 0 {
		return
	}
	r := s.intentRouter(ctx)
	if r == nil {
		return
	}
	specs := biz.EvalIntentAnchorSpecs()
	scores := make(map[string]float64, len(evalRouteModeIntents))
	best, bestMargin := "", 0.0
	for _, intent := range evalRouteModeIntents {
		pos, _, ok := r.Score(embedding, intent)
		if !ok {
			continue
		}
		scores[string(intent)] = float64(pos)
		if float64(pos) >= specs[intent].Threshold && float64(pos) > bestMargin {
			best, bestMargin = string(intent), float64(pos)
		}
	}
	if best == "" {
		// 全意图不达阈：VIR 侧无路由结论。以 no_recall 编码问题集选项
		// （注意这是近似——真实检索仍走关键词通道，harvest 金标时须按
		// 最终应答质量复核该编码，见台账）。
		best = "no_recall"
	}
	state := map[string]any{"user_message": query}
	state["lexical_modes"] = evalModeSet(lexical)
	ev := decide.ShadowEvent{
		CorrelationID: shadowCorrID("route_mode", userID, query),
		QuestionID:    "route_mode",
		State:         state,
		Incumbent: decide.Incumbent{
			Key:       best,
			Scores:    scores,
			Method:    "vir",
			Matched:   best != "no_recall",
		},
		UserID: userID,
	}
	s.shadow.Emit(ctx, ev)
}

// shadowCorrectionGate 双记写侧 correction 门决策（在 lexical||VIR 合成
// 判定之后、早退之前调用，保证自然流量全分布入样——P33 银标非自然分布
// 的修正手段）。routed 为 strip 后裸文本的嵌入（与 VIR 锚点同分布）。
func (s *evalMemoryStore) shadowCorrectionGate(ctx context.Context, userID, stmt string, routed []float32, lexicalCue bool) {
	if s.shadow == nil || !s.shadow.Enabled() || len(routed) == 0 {
		return
	}
	r := s.intentRouter(ctx)
	if r == nil {
		return
	}
	pos, _, ok := r.Score(routed, biz.EvalIntentCorrection)
	if !ok {
		return
	}
	thr := biz.EvalIntentAnchorSpecs()[biz.EvalIntentCorrection].Threshold
	key := "keep_as_is"
	if float64(pos) >= thr {
		key = "update_fact"
	}
	state := map[string]any{"user_message": stripEvalRolePrefix(stmt)}
	state["lexical_cue"] = lexicalCue
	ev := decide.ShadowEvent{
		CorrelationID: shadowCorrID("correction_gate", userID, stmt),
		QuestionID:    "correction_gate",
		State:         state,
		Incumbent: decide.Incumbent{
			Key:       key,
			Scores:    map[string]float64{"correction": float64(pos)},
			Threshold: thr,
			Method:    "vir",
			Matched:   float64(pos) >= thr,
		},
		UserID: userID,
	}
	s.shadow.Emit(ctx, ev)
}

// evalModeSet 快照词法分析置位的模式集（VIR 合并前）。
func evalModeSet(qa biz.EvalQueryAnalysis) []string {
	var out []string
	if qa.Chronological {
		out = append(out, "chronological")
	}
	if qa.Aggregate {
		out = append(out, "aggregate")
	}
	if qa.Enumerative {
		out = append(out, "enumerative")
	}
	if qa.Relational {
		out = append(out, "relational")
	}
	if qa.Trend {
		out = append(out, "trend")
	}
	return out
}

// shadowCorrID 生成确定性关联 id：同一 (question, user, text) 的重试/
// 重放塌缩为同一条关联（harvest 配对 incumbent/jev 两面的键）。
func shadowCorrID(question, userID, text string) string {
	h := sha1.New()
	h.Write([]byte(question))
	h.Write([]byte{0})
	h.Write([]byte(userID))
	h.Write([]byte{0})
	h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))[:16]
}
