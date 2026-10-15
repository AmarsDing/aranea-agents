// Code owned by package agent; extracted from agent_allocator_impl.go for AS-COG-01 file-size fitness.

package agent

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"

	"aranea-agents/pkg/loggateway"
	"context"

	"errors"
	"fmt"
	"math"

	"strings"
	"sync"
)

// layer2Shared 是一次 Allocate 调用内 Layer2 匹配的共享状态（P3b，2026-08-21）。
//
// Phase A 跨子任务并行（P-ORCH.5）后，Layer2 内部仍有两个串行热点：
//   - 每个进入 Layer2 的子任务把 N 个 Agent 文本随 taskText 整包重嵌
//     （4 子任务 = 4×(N+1) 条嵌入、4 次大 RPC；Agent 文本完全相同，纯重复）；
//   - 候选融合的 perfRepo.Get 对每个子任务的每个候选各点查一次
//     （N×M 次顺序 DB 往返）。
//
// layer2Shared 把二者降为 per-Allocate 一次性：Agent 向量 sync.Once 懒加载
// 共享（子任务只嵌入自己的 taskText 单条），perf 按 agentKey 去重缓存。
// 生命周期 per-Allocate（非 impl 字段），跨 Allocate 天然并发安全。
type layer2Shared struct {
	embedOnce sync.Once
	agentCaps []biz.AgentCapability // 与 agentVecs 对齐（已滤系统 Agent）
	agentVecs [][]float32
	embedErr  error

	perfMu    sync.Mutex
	perfCache map[string]*biz.AgentPerformance // key 存在 = 已查过（值可为 nil）
}

// errLayer2NoCandidates / errLayer2DimMismatch 是 loadAgentVectors 的哨兵错误，
// 调用方据此区分「无候选静默回退」与「真实失败告警回退」。
var (
	errLayer2NoCandidates = errors.New("layer2: no non-system agent candidates")
	errLayer2DimMismatch  = errors.New("layer2: embedding dimension mismatch")
)

func newLayer2Shared() *layer2Shared {
	return &layer2Shared{perfCache: make(map[string]*biz.AgentPerformance)}
}

// ensureLayer2Shared 容忍 nil（旧测试构造路径直接调 matchLayer2/matchSubTask
// 时不传共享态）——退化为一次性实例，行为与改造前一致。
func ensureLayer2Shared(s *layer2Shared) *layer2Shared {
	if s != nil {
		return s
	}
	return newLayer2Shared()
}

// loadAgentVectors 懒加载全部非系统 Agent 的能力文本嵌入：仅首次调用真实
// 发起一次批量 Embed RPC，后续调用与并发 goroutine 共享结果（含共享错误——
// 嵌入失败时所有子任务一致回退 TF-IDF，与改造前逐子任务失败的语义等价）。
func (s *layer2Shared) loadAgentVectors(ctx context.Context, embedder knowledge.Embedder, capabilities []biz.AgentCapability) ([]biz.AgentCapability, [][]float32, error) {
	s.embedOnce.Do(func() {
		var texts []string
		for _, cap := range capabilities {
			if !cap.IsHeuristicAssignable() {
				continue
			}
			s.agentCaps = append(s.agentCaps, cap)
			texts = append(texts, buildAgentCapabilityText(cap))
		}
		if len(s.agentCaps) == 0 {
			s.embedErr = errLayer2NoCandidates
			return
		}
		vecs, err := embedder.Embed(ctx, texts)
		if err != nil {
			s.embedErr = err
			return
		}
		if len(vecs) != len(texts) || len(vecs[0]) == 0 {
			s.embedErr = errLayer2DimMismatch
			return
		}
		s.agentVecs = vecs
	})
	return s.agentCaps, s.agentVecs, s.embedErr
}

// perfFor 返回 agentKey 的 "general" 履历（按 key 去重缓存：同一 Agent 一次
// Allocate 只点查一次；查询失败/无记录同样缓存 nil，不重复打 DB）。
func (s *layer2Shared) perfFor(ctx context.Context, repo biz.AgentPerformanceRepository, agentKey string) *biz.AgentPerformance {
	if repo == nil {
		return nil
	}
	s.perfMu.Lock()
	defer s.perfMu.Unlock()
	if perf, ok := s.perfCache[agentKey]; ok {
		return perf
	}
	perf, err := repo.Get(ctx, agentKey, "general")
	if err != nil {
		perf = nil
	}
	s.perfCache[agentKey] = perf
	return perf
}

// exactMatch performs Layer 1 matching: overlap between required_capabilities and agent Roles.
func (impl *agentAllocatorImpl) exactMatch(requiredCapabilities []string, capabilities []biz.AgentCapability) (biz.AgentCapability, float64, string) {
	if len(requiredCapabilities) == 0 || len(capabilities) == 0 {
		return biz.AgentCapability{}, 0, ""
	}

	type scored struct {
		cap   biz.AgentCapability
		score float64
	}

	var candidates []scored
	for _, cap := range capabilities {
		if !cap.IsHeuristicAssignable() {
			continue
		}
		overlapRatio := computeOverlapRatio(requiredCapabilities, cap.Roles)
		if overlapRatio == 0 {
			continue
		}

		historicalRate := 0.5
		score := overlapRatio*0.7 + float64(historicalRate)*0.3
		candidates = append(candidates, scored{cap: cap, score: score})
	}

	if len(candidates) == 0 {
		return biz.AgentCapability{}, 0, ""
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}

	reason := fmt.Sprintf("角色重叠率 %.2f, 综合得分 %.2f", computeOverlapRatio(requiredCapabilities, best.cap.Roles), best.score)
	return best.cap, best.score, reason
}

// matchLayer2 performs Layer 2 matching: semantic similarity between task description and agent capabilities.
//
// When an embedder is configured, it uses in-memory embedding cosine similarity (the embedder is
// typically backed by OpenAI/Gemini/Ollama text-embedding APIs) for true semantic matching. When
// the embedder is nil or fails, it gracefully falls back to the existing TF-IDF keyword-based
// matching — no hard error is surfaced to the caller.
//
// Note: pgvector SQL similarity (`<=>` operator on the `vector_embeddings` table) is used by the
// memory domain (internal/data/vector/pgvector_fact.go) for memory retrieval, not by the allocator.
// The allocator computes cosine in Go memory to avoid a Postgres round-trip on the hot allocation path.
func (impl *agentAllocatorImpl) matchLayer2(ctx context.Context, subTask biz.SubTask, capabilities []biz.AgentCapability, traceID string, sharedOpt ...*layer2Shared) (biz.AgentCapability, float64, string) {
	if len(capabilities) == 0 {
		return biz.AgentCapability{}, 0, ""
	}
	var shared *layer2Shared
	if len(sharedOpt) > 0 {
		shared = ensureLayer2Shared(sharedOpt[0])
	} else {
		shared = newLayer2Shared()
	}

	if impl.embedder != nil {
		if cap, score, reason, ok := impl.matchLayer2Embedding(ctx, subTask, capabilities, traceID, shared); ok {
			return cap, score, reason
		}
	}

	return impl.matchLayer2TFIDF(ctx, subTask, capabilities, traceID, shared)
}

// matchLayer2Embedding uses embedding cosine similarity for semantic matching.
// Returns ok=false when the embedder fails or produces unusable vectors; the caller falls back to TF-IDF.
//
// P3b: Agent 侧向量经 shared.loadAgentVectors per-Allocate 一次性批量嵌入共享，
// 每个子任务只嵌入自己的 taskText 单条；候选融合的 perf 履历经 shared.perfFor
// 按 agentKey 去重点查，不再 N×M 次顺序打 DB。
func (impl *agentAllocatorImpl) matchLayer2Embedding(ctx context.Context, subTask biz.SubTask, capabilities []biz.AgentCapability, traceID string, shared *layer2Shared) (biz.AgentCapability, float64, string, bool) {
	taskText := subTask.Name + " " + subTask.Description
	for _, cap := range subTask.RequiredCapabilities {
		taskText += " " + cap
	}

	agentCaps, agentVecs, err := shared.loadAgentVectors(ctx, impl.embedder, capabilities)
	if err != nil {
		if !errors.Is(err, errLayer2NoCandidates) {
			impl.lg.Warn("Layer 2 embedding 失败，降级为 TF-IDF",
				loggateway.StepID(biz.SpiritStepAllocatorMatch),
				loggateway.Str("trace_id", traceID),
				loggateway.Err(err),
			)
		}
		return biz.AgentCapability{}, 0, "", false
	}

	vectors, err := impl.embedder.Embed(ctx, []string{taskText})
	if err != nil {
		impl.lg.Warn("Layer 2 taskText embedding 失败，降级为 TF-IDF",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Err(err),
		)
		return biz.AgentCapability{}, 0, "", false
	}
	if len(vectors) != 1 || len(vectors[0]) == 0 {
		impl.lg.Warn("Layer 2 embedding 维度不匹配，降级为 TF-IDF",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Int("expected", 1),
			loggateway.Int("got", len(vectors)),
		)
		return biz.AgentCapability{}, 0, "", false
	}

	taskVec := vectors[0]

	allow := make(map[string]struct{}, len(capabilities))
	for _, cap := range capabilities {
		allow[cap.AgentKey] = struct{}{}
	}

	type scored struct {
		cap   biz.AgentCapability
		score float64
	}

	var candidates []scored
	for i, cap := range agentCaps {
		if !cap.IsHeuristicAssignable() {
			continue
		}
		if _, ok := allow[cap.AgentKey]; !ok {
			continue
		}
		sim := cosineSimilarity32(taskVec, agentVecs[i])
		if sim <= 0 {
			continue
		}

		score := sim
		if perf := shared.perfFor(ctx, impl.perfRepo, cap.AgentKey); perf != nil {
			score = score*0.6 + perf.SuccessRate*0.4
		}
		candidates = append(candidates, scored{cap: cap, score: score})
	}

	if len(candidates) == 0 {
		return biz.AgentCapability{}, 0, "", false
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}

	reason := fmt.Sprintf("向量语义匹配 (cosine: %.2f)", best.score)
	return best.cap, best.score, reason, true
}

// matchLayer2TFIDF is the TF-IDF keyword-based fallback for Layer 2 matching.
// P3b: perf 履历经 shared.perfFor 按 agentKey 去重点查。
func (impl *agentAllocatorImpl) matchLayer2TFIDF(ctx context.Context, subTask biz.SubTask, capabilities []biz.AgentCapability, traceID string, shared *layer2Shared) (biz.AgentCapability, float64, string) {
	taskText := subTask.Name + " " + subTask.Description
	for _, cap := range subTask.RequiredCapabilities {
		taskText += " " + cap
	}

	type scored struct {
		cap   biz.AgentCapability
		score float64
	}

	var candidates []scored
	for _, cap := range capabilities {
		if !cap.IsHeuristicAssignable() {
			continue
		}
		score := computeSemanticScore(taskText, cap)

		if perf := shared.perfFor(ctx, impl.perfRepo, cap.AgentKey); perf != nil {
			score = score*0.6 + perf.SuccessRate*0.4
		}
		if score > 0 {
			candidates = append(candidates, scored{cap: cap, score: score})
		}
	}

	if len(candidates) == 0 {
		return biz.AgentCapability{}, 0, ""
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}

	reason := fmt.Sprintf("TF-IDF 语义匹配 (score: %.2f)", best.score)
	return best.cap, best.score, reason
}

// matchLayer2ForPlan performs Layer 2 matching for a whole plan (no subtasks).
func (impl *agentAllocatorImpl) matchLayer2ForPlan(ctx context.Context, taskPlan *biz.TaskPlan, capabilities []biz.AgentCapability, traceID string) (biz.AgentCapability, float64, string) {
	if len(capabilities) == 0 {
		return biz.AgentCapability{}, 0, ""
	}

	type scored struct {
		cap   biz.AgentCapability
		score float64
	}

	var candidates []scored
	for _, cap := range capabilities {
		if !cap.IsHeuristicAssignable() {
			continue
		}
		score := computeSemanticScore(taskPlan.UserMessage, cap)
		if impl.perfRepo != nil {
			perf, err := impl.perfRepo.Get(ctx, cap.AgentKey, "general")
			if err == nil && perf != nil {
				score = score*0.6 + perf.SuccessRate*0.4
			}
		}
		if score > 0 {
			candidates = append(candidates, scored{cap: cap, score: score})
		}
	}

	if len(candidates) == 0 {
		return biz.AgentCapability{}, 0, ""
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}

	reason := fmt.Sprintf("语义匹配 (score: %.2f)", best.score)
	return best.cap, best.score, reason
}

// computeSemanticScore computes a TF-IDF-like keyword overlap score between
// a task description and an agent's capability profile.
//
// This function powers the TF-IDF fallback path in matchLayer2TFIDF. The
// primary Layer 2 path now uses embedding cosine similarity via matchLayer2Embedding;
// this keyword scorer remains as the graceful-degradation fallback when the
// embedder is unavailable or fails.
func computeSemanticScore(taskDesc string, cap biz.AgentCapability) float64 {
	return semanticScoreText(taskDesc, buildAgentCapabilityText(cap))
}

// semanticScoreText computes a TF-IDF-like keyword overlap score between a
// task description and an arbitrary agent text corpus.
func semanticScoreText(taskDesc, agentText string) float64 {
	taskTokens := tokenizeForSemantic(taskDesc)
	agentTokens := tokenizeForSemantic(agentText)

	if len(taskTokens) == 0 || len(agentTokens) == 0 {
		return 0
	}

	taskTF := make(map[string]float64, len(taskTokens))
	for _, t := range taskTokens {
		taskTF[t]++
	}
	for k := range taskTF {
		taskTF[k] /= float64(len(taskTokens))
	}

	agentSet := make(map[string]bool, len(agentTokens))
	for _, t := range agentTokens {
		agentSet[t] = true
	}

	var score float64
	for token, tf := range taskTF {
		if agentSet[token] {

			score += tf * 2.0
		}
	}

	score = score / (1.0 + score)

	score = 1.0 / (1.0 + math.Exp(-6*(score-0.5)))

	return score
}

// tokenizeForSemantic splits text into lowercase tokens for semantic comparison.
func tokenizeForSemantic(text string) []string {
	text = strings.ToLower(text)

	for _, sep := range []string{"-", "_", "/", ".", ",", "，", "、", "：", "："} {
		text = strings.ReplaceAll(text, sep, " ")
	}
	fields := strings.Fields(text)
	// Filter out very short tokens
	var result []string
	for _, f := range fields {
		if len(f) >= 2 {
			result = append(result, f)
		}
	}
	return result
}

// buildAgentCapabilityText concatenates an agent's capability profile into a
// single text blob suitable for embedding. The field order matches the
// TF-IDF corpus built by computeSemanticScore so both paths compare the same text.
func buildAgentCapabilityText(cap biz.AgentCapability) string {
	parts := []string{cap.DisplayName, cap.Description}
	parts = append(parts, cap.Roles...)
	parts = append(parts, cap.Domains...)
	parts = append(parts, cap.Tools...)
	parts = append(parts, cap.Skills...)
	return strings.Join(parts, " ")
}

// cosineSimilarity32 computes the cosine similarity between two float32 vectors.
// Returns 0 for empty or mismatched-length vectors, or when either vector has zero magnitude.
func cosineSimilarity32(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
