package agent

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// 使命驱动匹配（B.10.21.5）：L0 领域配方复用 + L1 使命匹配。
//
// L0 domain_recipe：OrchestrationCache 中以 "domain:<path>" 记录的成功配方
//   （DQ ≥ DomainRecipeMinDQ + lead agent 仍存活）直接复用，AgentKeys[1:]
//   作为 DAG 配方成员（替代 selectAdditionalMembers 随机补员）。
// L1 mission：同域候选收敛（DomainPathRelated）后按
//   score = similarity(task, mission) × 0.4 + perfSuccessRate × 0.6 排序，
//   bestScore > 0.3 命中。embedder 非 nil 用 cosine，否则 TF-IDF（不变量 3）。
// ---------------------------------------------------------------------------

const (
	// missionSimWeight / missionPerfWeight fuse mission similarity with
	// track-record success rate in L1 scoring（履历背书优先于文本相似）。
	missionSimWeight  = 0.4
	missionPerfWeight = 0.6
	// missionMatchMinScore is the minimum fused score for an L1 hit.
	missionMatchMinScore = 0.3
	// defaultSuccessRate applies when no performance record exists
	// （与 L2 exactMatch 无履历默认语义一致）。
	defaultSuccessRate = 0.5
)

// tryDomainRecipe is the L0 domain-recipe reuse matcher.
func (impl *agentAllocatorImpl) tryDomainRecipe(domainPath string, capabilities []biz.AgentCapability, traceID string) (biz.AgentCapability, []string, float64, bool) {
	if impl.orchCache == nil {
		return biz.AgentCapability{}, nil, 0, false
	}
	entry, ok := impl.orchCache.BestRecipeForDomain(domainPath)
	if !ok || entry == nil || len(entry.AgentKeys) == 0 {
		return biz.AgentCapability{}, nil, 0, false
	}
	// lead agent 必须仍存在于存活能力列表（已删除的 agent 不复用）。
	lead, found := findCapabilityByKey(capabilities, entry.AgentKeys[0])
	if !found {
		impl.lg.Warn("L0 领域配方 lead agent 已删除，跳过复用",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Str("domain_path", domainPath),
			loggateway.AgentKey(entry.AgentKeys[0]),
		)
		return biz.AgentCapability{}, nil, 0, false
	}
	// 配方成员：AgentKeys[1:] 直接复用，剔除已删除者（缓存可能滞后）。
	members := make([]string, 0, len(entry.AgentKeys)-1)
	for _, key := range entry.AgentKeys[1:] {
		if _, ok := findCapabilityByKey(capabilities, key); ok {
			members = append(members, key)
		}
	}
	impl.lg.Info("L0 领域配方命中",
		loggateway.StepID(biz.SpiritStepAllocatorMatch),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("domain_path", domainPath),
		loggateway.AgentKey(lead.AgentKey),
		loggateway.Float64("dq_score", entry.DQScore),
		loggateway.Int("member_count", len(members)),
	)
	return lead, members, entry.DQScore, true
}

// tryMissionMatch is the L1 mission-driven matcher. Returns the matched
// capability, fused score, same-domain candidate count (for MatchReason
// explainability, US-MM-03), and whether the score cleared the threshold.
func (impl *agentAllocatorImpl) tryMissionMatch(ctx context.Context, taskText, domainPath string, capabilities []biz.AgentCapability, traceID string) (biz.AgentCapability, float64, int, bool) {
	// 同域候选收敛：前缀匹配（任一方向）或归并后同一级域。
	cands := make([]biz.AgentCapability, 0, len(capabilities))
	for _, cap := range capabilities {
		if DomainPathRelated(cap.DomainPath, domainPath) {
			cands = append(cands, cap)
		}
	}
	if len(cands) == 0 {
		return biz.AgentCapability{}, 0, 0, false
	}

	// similarity：embedder 非 nil → cosine(embedding)；nil/失败 → TF-IDF。
	sims := make([]float64, len(cands))
	embedded := false
	if impl.embedder != nil {
		texts := make([]string, 0, len(cands)+1)
		texts = append(texts, taskText)
		for _, cap := range cands {
			texts = append(texts, missionMatchText(cap))
		}
		vectors, err := impl.embedder.Embed(ctx, texts)
		switch {
		case err != nil:
			impl.lg.Warn("L1 使命匹配 embedding 失败，回退 TF-IDF",
				loggateway.StepID(biz.SpiritStepAllocatorMatch),
				loggateway.Str("trace_id", traceID),
				loggateway.Err(err),
			)
		case len(vectors) != len(texts) || len(vectors[0]) == 0:
			impl.lg.Warn("L1 使命匹配 embedding 返回维度异常，回退 TF-IDF",
				loggateway.StepID(biz.SpiritStepAllocatorMatch),
				loggateway.Str("trace_id", traceID),
				loggateway.Int("want", len(texts)),
				loggateway.Int("got", len(vectors)),
			)
		default:
			embedded = true
			for i := range cands {
				sims[i] = cosineSimilarity32(vectors[0], vectors[i+1])
			}
		}
	}
	if !embedded {
		for i, cap := range cands {
			sims[i] = semanticScoreText(taskText, missionMatchText(cap))
		}
	}

	// score = similarity × 0.4 + perfSuccessRate("domain:"+domainPath) × 0.6。
	// 履历批量拉取一次（GetBestForTaskType），避免候选循环内逐条查询；
	// 候选未入榜（如已删除 agent 占用 top-N）回退默认 0.5，仅影响排序 tie-break。
	successRates := map[string]float64{}
	if impl.perfRepo != nil {
		if perfs, err := impl.perfRepo.GetBestForTaskType(ctx, "domain:"+domainPath, len(cands)); err == nil {
			for _, p := range perfs {
				if p != nil {
					successRates[p.AgentKey] = p.SuccessRate
				}
			}
		}
	}
	bestIdx, bestScore := -1, 0.0
	for i, cap := range cands {
		successRate, ok := successRates[cap.AgentKey]
		if !ok {
			successRate = defaultSuccessRate
		}
		if score := sims[i]*missionSimWeight + successRate*missionPerfWeight; score > bestScore {
			bestIdx, bestScore = i, score
		}
	}
	if bestIdx < 0 || bestScore <= missionMatchMinScore {
		return biz.AgentCapability{}, 0, len(cands), false
	}
	impl.lg.Info("L1 使命匹配命中",
		loggateway.StepID(biz.SpiritStepAllocatorMatch),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("domain_path", domainPath),
		loggateway.AgentKey(cands[bestIdx].AgentKey),
		loggateway.Float64("score", bestScore),
		loggateway.Int("candidate_count", len(cands)),
	)
	return cands[bestIdx], bestScore, len(cands), true
}

// missionMatchReason renders the L1 mission-match display reason.
func missionMatchReason(domainPath string, candidates int, score float64) string {
	return fmt.Sprintf("使命匹配 (domain: %s, 同域候选 %d, score %.2f)", domainPath, candidates, score)
}

// missionMatchText returns the agent's mission corpus: Mission with
// Description fallback（存量 Agent 未回填使命时仍可匹配，不变量 2）。
func missionMatchText(cap biz.AgentCapability) string {
	if cap.Mission != "" {
		return cap.Mission
	}
	return cap.Description
}

// findCapabilityByKey looks up an agent in the live capability list.
func findCapabilityByKey(capabilities []biz.AgentCapability, key string) (biz.AgentCapability, bool) {
	for _, cap := range capabilities {
		if cap.AgentKey == key {
			return cap, true
		}
	}
	return biz.AgentCapability{}, false
}
