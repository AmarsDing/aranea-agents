package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// ── G-37 实体消解灰区裁决器（ADR-KN-20261021-F 生产实现）────────────────────
//
// biz runEntityMergeTask 产出 [0.80,0.90) 灰区候选对后，由本实现单轮 LLM
// 二元裁决：judge 同名/别名/类型/sim 证据 → JSON {"verdict":"yes|no",
// "reason":"≤120 runes"}。判定失败（解析失败/非 yes/no）一律 fail-close 为
// no（零提案、零 memo——ADR-F 零噪音）；LLM 调用失败上抛 error（biz 侧
// Warn 降级+连败熔断、留日轮转下轮重试）。usage 经窄口
// EntityMergeUsageRecorder 记账（aux_entity_merge，晚绑同 mocUsageRecorder /新B种植）。

const (
	// entityMergeAdjudicateTimeout 单次裁决 LLM 调用超时（对齐 moc summary）。
	entityMergeAdjudicateTimeout = 60 * time.Second
	// entityMergeReasonMaxRunes 裁决理由 rune 上限（与提案载荷一致截断）。
	entityMergeReasonMaxRunes = 120
)

// EntityMergeUsageInput 裁决 usage 记账入参（CompletionTok 承载 totalTokens——
// biz.LLMCaller 聚合口径，UsageSourceResponse 计；moc 先例同式）。
type EntityMergeUsageInput struct {
	Provider   string
	Model      string
	Tokens     int
	Latency    time.Duration
	KeeperID   int64
	KeeperName string
	MergeeID   int64
	MergeeName string
	Similarity float64
	Err        error
}

// EntityMergeUsageRecorder 裁决 usage 记账窄口（生产 = service.
// entityMergeUsageRecorder；nil = 不记录）。
type EntityMergeUsageRecorder interface {
	RecordEntityMergeUsage(ctx context.Context, in EntityMergeUsageInput)
}

// EntityMergeAdjudicator 实现 bizknowledge.EntityMergeAdjudicator（LLM 单轮灰区裁决）。
type EntityMergeAdjudicator struct {
	llm      biz.LLMCaller
	sys      RefineLLMSettingsGetter
	catalog  LLMCatalogLister
	usageRec EntityMergeUsageRecorder
	lg       loggateway.Logger
}

// NewEntityMergeAdjudicator 构造裁决器；lg 为 nil 时降级 Noop。
func NewEntityMergeAdjudicator(llm biz.LLMCaller, sys RefineLLMSettingsGetter, catalog LLMCatalogLister, lg loggateway.Logger) *EntityMergeAdjudicator {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &EntityMergeAdjudicator{llm: llm, sys: sys, catalog: catalog, lg: lg}
}

// SetUsageRecorder 注入 usage 记录器（装配层调用，同 embedder SetUsageRecorder 模式）。
func (a *EntityMergeAdjudicator) SetUsageRecorder(r EntityMergeUsageRecorder) {
	if a == nil {
		return
	}
	a.usageRec = r
}

// EntityMergeAdjudicatorDisabled 环境开关（与其他 worker 同纪律）：置 1 时
// 生产装配跳过接线（biz 侧未接线即跳整个 entity_merge 任务）。
func EntityMergeAdjudicatorDisabled() bool {
	return strings.TrimSpace(os.Getenv("KNOWLEDGE_ENTITY_MERGE_ADJUDICATOR_DISABLED")) == "1"
}

// emAdjudicationPayload LLM 裁决 JSON 载荷。
type emAdjudicationPayload struct {
	Verdict string `json:"verdict"`
	Reason  string `json:"reason"`
}

// AdjudicateEntityMerge 裁决一对灰区候选是否同一实体。LLM/调用失败上抛
// error；解析失败或 verdict 非 "yes" 一律 fail-close 返回 Verdict=false
// （ADR-F 零噪音：对 LLM 不确定的候选永不落提案）。
func (a *EntityMergeAdjudicator) AdjudicateEntityMerge(ctx context.Context, req bizknowledge.EntityMergeAdjudicateRequest) (bizknowledge.EntityMergeAdjudication, error) {
	if a == nil || a.llm == nil {
		return bizknowledge.EntityMergeAdjudication{}, fmt.Errorf("entity merge adjudicator not wired")
	}
	provider, model, err := ResolveLLM(ctx, a.sys, a.catalog, "entity merge adjudication", a.lg)
	if err != nil {
		return bizknowledge.EntityMergeAdjudication{}, err
	}

	user := fmt.Sprintf("实体 A：名称「%s」类型「%s」\n实体 B：名称「%s」类型「%s」\n向量相似度：%.4f\n",
		strings.TrimSpace(req.KeeperName), strings.TrimSpace(req.KeeperType),
		strings.TrimSpace(req.MergeeName), strings.TrimSpace(req.MergeeType), req.Similarity)

	callCtx, cancel := context.WithTimeout(ctx, entityMergeAdjudicateTimeout)
	start := time.Now()
	resp, totalTokens, callErr := a.llm.Call(callCtx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System: `你是知识库实体消解裁决助手。给定两个实体的名称、类型与向量相似度（已处于 0.80~0.90 灰区），判断它们是否指向同一真实世界实体（别名/全半角/大小写/简繁/缩写变体等）。
要求：
1. 只输出一行 JSON：{"verdict":"yes","reason":"≤120字理由"} 或 {"verdict":"no","reason":"≤120字理由"}，不要任何前后缀说明或 markdown 代码块；
2. 仅在证据明确（同一事物的不同写法/译名/缩写）时裁 yes；名称不同义、类型不符、或仅靠相似度臆测时一律裁 no；
3. 拿不准一律裁 no——宁可漏并，不可错并。`,
		User: user,
	})
	cancel()
	a.recordUsage(ctx, req, provider, model, totalTokens, time.Since(start), callErr)
	if callErr != nil {
		return bizknowledge.EntityMergeAdjudication{}, callErr
	}

	// 严解析 fail-close：```json 围栏剥除 → JSON 解码 → verdict 仅受理 "yes"。
	verdict, reason, perr := parseEntityMergeAdjudication(resp)
	if perr != nil {
		a.lg.Warn("治理：实体消解裁决解析失败（fail-close=no）",
			loggateway.StepID("knowledge.curate.entity_merge"),
			loggateway.Int64("keeper_id", req.KeeperID),
			loggateway.Int64("mergee_id", req.MergeeID),
			loggateway.Err(perr))
		return bizknowledge.EntityMergeAdjudication{Verdict: false, Reason: ""}, nil
	}
	return bizknowledge.EntityMergeAdjudication{Verdict: verdict, Reason: reason}, nil
}

// parseEntityMergeAdjudication 剥围栏严解析 {verdict,reason}；
// verdict=="yes"（大小写容忍）才为肯定；reason 超 120 runes 截断。
func parseEntityMergeAdjudication(resp string) (bool, string, error) {
	s := strings.TrimSpace(resp)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSpace(s)
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = strings.TrimSpace(s[:i])
		}
	}
	var p emAdjudicationPayload
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return false, "", fmt.Errorf("entity merge adjudication json: %w", err)
	}
	if v := strings.ToLower(strings.TrimSpace(p.Verdict)); v != "yes" {
		return false, "", nil // 非 yes（含 no/畸形值）一律 fail-close 否决
	}
	reason := strings.TrimSpace(p.Reason)
	if r := []rune(reason); len(r) > entityMergeReasonMaxRunes {
		reason = strings.TrimSpace(string(r[:entityMergeReasonMaxRunes]))
	}
	return true, reason, nil
}

// recordUsage usage 记账（best-effort：recorder 内部 Warn 降级，不污染主路径）。
// F12 终审加固（G-34 先例）：零 token 成功行不记账（provider 漏报 usage 时
// 幻影成功行虚报调用量）；失败行照记。
func (a *EntityMergeAdjudicator) recordUsage(ctx context.Context, req bizknowledge.EntityMergeAdjudicateRequest, provider, model string, tokens int, latency time.Duration, callErr error) {
	if tokens <= 0 && callErr == nil {
		return
	}
	rec := a.usageRec
	if rec == nil {
		return
	}
	rec.RecordEntityMergeUsage(ctx, EntityMergeUsageInput{
		Provider: provider, Model: model, Tokens: tokens, Latency: latency,
		KeeperID: req.KeeperID, KeeperName: req.KeeperName,
		MergeeID: req.MergeeID, MergeeName: req.MergeeName,
		Similarity: req.Similarity, Err: callErr,
	})
}

// 编译期接口断言。
var _ bizknowledge.EntityMergeAdjudicator = (*EntityMergeAdjudicator)(nil)
