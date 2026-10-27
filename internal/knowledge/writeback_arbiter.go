// Package knowledge — 自治理知识图谱 G-40：写回冲突仲裁器（LLM，四操作词表）。
//
// 新事实追加进词条页前，与同页既有事实段（同页即同主题，零检索成本）批量仲裁：
// update（新事实是旧段的更新替代）→ 走版本链顶替旧段；
// delete（新事实与旧段矛盾）→ 置信分带（高带人工二审、低带自动失效+墓碑）；
// noop（语义重复/已被覆盖）→ 零旁效应跳过；
// add（与所有段无上述关系）→ 原追加行为（未输出即默认 add）。
// 解析层兼容旧三值（supersedes/contradicts/unrelated 映射 update/delete/noop，
// G-40 D2）；未知值丢弃。仲裁失败由调用方降级为直接追加，不阻断写回。
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

const (
	// arbitrateTimeout 单次仲裁 LLM 调用超时。
	arbitrateTimeout = 60 * time.Second
	// arbitrateMaxExisting 候选旧段上限（控 prompt 规模）。
	arbitrateMaxExisting = 20
	// arbitrateMaxNews 单次仲裁新事实上限。
	arbitrateMaxNews = 10
	// arbitrateBlockMaxRunes 旧段正文截断（陈述足够判断，整段不必全给）。
	arbitrateBlockMaxRunes = 300
	// arbitrateStmtMaxRunes 新事实陈述截断。
	arbitrateStmtMaxRunes = 400
)

// WriteBackArbiter 实现 bizknowledge.WriteBackArbiter（LLM 批量仲裁）。
type WriteBackArbiter struct {
	llm     biz.LLMCaller
	sys     RefineLLMSettingsGetter
	catalog LLMCatalogLister
	lg      loggateway.Logger
}

// NewWriteBackArbiter 构造仲裁器；lg 为 nil 时降级 Noop。
func NewWriteBackArbiter(llm biz.LLMCaller, sys RefineLLMSettingsGetter, catalog LLMCatalogLister, lg loggateway.Logger) *WriteBackArbiter {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &WriteBackArbiter{llm: llm, sys: sys, catalog: catalog, lg: lg}
}

// WriteBackArbiterDisabled 环境开关（与其他 worker 同纪律）：置 1 时生产装配跳过接线。
func WriteBackArbiterDisabled() bool {
	return strings.TrimSpace(os.Getenv("KNOWLEDGE_WRITEBACK_ARBITER_DISABLED")) == "1"
}

type arbitrateVerdict struct {
	FactIndex    int     `json:"fact_index"`
	Operation    string  `json:"operation"` // 四操作键（G-40 新契约）
	Verdict      string  `json:"verdict"`   // 旧键名兼容（D2 灰度窗口——固定缓存提示词变体）
	TargetFactID string  `json:"target_fact_id"`
	Confidence   float64 `json:"confidence"`
	Reason       string  `json:"reason"`
}

// parseWriteBackOp 解析仲裁操作：四操作直出 + 旧三值映射（G-40 D2：supersedes→update、
// contradicts→delete、unrelated→noop）；未知值返回 ok=false 丢弃（与现行过滤同纪律）。
func parseWriteBackOp(raw string) (bizknowledge.WriteBackOp, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "add":
		return bizknowledge.WriteBackOpAdd, true
	case "update", "supersedes":
		return bizknowledge.WriteBackOpUpdate, true
	case "delete", "contradicts":
		return bizknowledge.WriteBackOpDelete, true
	case "noop", "unrelated":
		return bizknowledge.WriteBackOpNoop, true
	}
	return "", false
}

// ArbitrateWriteBack 批量仲裁新事实 vs 同页既有段。add 不必显式输出（未判即默认
// 追加）；update/delete/noop 结论透出由消费面按档处置。LLM/解析失败上抛 error
// （调用方降级直接追加）。
func (a *WriteBackArbiter) ArbitrateWriteBack(ctx context.Context, title string, existing []bizknowledge.WriteBackFactBlock, news []bizknowledge.WriteBackFact) ([]bizknowledge.WriteBackArbitration, error) {
	if a == nil || a.llm == nil {
		return nil, fmt.Errorf("writeback arbiter not wired")
	}
	if len(news) == 0 || len(existing) == 0 {
		return nil, nil
	}
	if len(existing) > arbitrateMaxExisting {
		// C 批修（评审 #16）：截断取尾部——词条页新块一律尾追加（正文序≈创建序），
		// 取头部=保留最旧 20 段、与时间局部性最强的最新段（冲突概率最高）失之交臂。
		// 截断事件 Warn 可观测：成熟词条页仲裁覆盖缺口不再静默。
		dropped := len(existing) - arbitrateMaxExisting
		existing = existing[len(existing)-arbitrateMaxExisting:]
		a.lg.Warn("写回仲裁候选超上限，截断保留最新段（旧段本轮不参与仲裁）",
			loggateway.StepID("knowledge.writeback.arbitrate"),
			loggateway.Str("title", title),
			loggateway.Int("kept", len(existing)),
			loggateway.Int("dropped_existing", dropped),
		)
	}
	if len(news) > arbitrateMaxNews {
		news = news[:arbitrateMaxNews]
	}
	provider, model, err := ResolveLLM(ctx, a.sys, a.catalog, "writeback arbitrate", a.lg)
	if err != nil {
		return nil, err
	}

	type oldBlock struct {
		FactID    string `json:"fact_id"`
		Kind      string `json:"kind"`
		Statement string `json:"statement"`
	}
	olds := make([]oldBlock, 0, len(existing))
	for _, b := range existing {
		olds = append(olds, oldBlock{
			FactID:    b.FactID,
			Kind:      b.Heading,
			Statement: truncateRunes(strings.TrimSpace(b.Body), arbitrateBlockMaxRunes),
		})
	}
	type newFact struct {
		Index     int    `json:"index"`
		Kind      string `json:"kind"`
		Statement string `json:"statement"`
	}
	newsIn := make([]newFact, 0, len(news))
	for i, f := range news {
		newsIn = append(newsIn, newFact{
			Index:     i,
			Kind:      f.FactKind,
			Statement: truncateRunes(strings.TrimSpace(f.Statement), arbitrateStmtMaxRunes),
		})
	}
	oldsJSON, _ := json.Marshal(olds)
	newsJSON, _ := json.Marshal(newsIn)

	callCtx, cancel := context.WithTimeout(ctx, arbitrateTimeout)
	defer cancel()
	resp, _, err := a.llm.Call(callCtx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System: `你是知识库写回冲突仲裁助手。给定词条页标题、页内既有事实段清单（含 fact_id）与待写入的新事实清单，逐条判断新事实应当执行的操作：
- update：新事实是某既有段同一事实的更新/修正（语义指向同一断言，内容更新）；
- delete：新事实与某既有段直接矛盾（不能同时成立）；
- noop：新事实与某既有段语义重复/已被覆盖，无需写入；
- add：与所有既有段无上述关系且确需新增（add 不必输出，未输出即默认 add）。
以 JSON 数组输出 update/delete/noop 结论，每个元素：
{"fact_index": 新事实index, "operation": "update/delete/noop", "target_fact_id": "目标既有段fact_id", "confidence": 0.0-1.0, "reason": "一句话理由"}
要求：
1. target_fact_id 必须原样取自既有段清单（noop 无目标可留空）；
2. confidence 反映判断明确程度（明示 0.8-1.0，推断 0.5-0.8）；
3. 只输出 JSON 数组（可为空数组），不要任何解释或代码块包裹。`,
		User: "词条标题：" + title + "\n\n既有事实段：\n" + string(oldsJSON) + "\n\n新事实：\n" + string(newsJSON),
	})
	if err != nil {
		return nil, err
	}
	var verdicts []arbitrateVerdict
	if err := unmarshalLLMJSONArray(resp, &verdicts); err != nil {
		return nil, fmt.Errorf("parse arbitrations: %w", err)
	}
	out := make([]bizknowledge.WriteBackArbitration, 0, len(verdicts))
	for _, v := range verdicts {
		op, ok := parseWriteBackOp(v.Operation)
		if !ok && strings.TrimSpace(v.Verdict) != "" {
			// 旧键兜底（D2）：operation 为空或非四操作/旧三值词表内的未知值时
			// 均回退 verdict——C 批修（评审 #17）：灰度窗口双键并存（operation 未知
			// + verdict 有效）不再整条丢弃。
			op, ok = parseWriteBackOp(v.Verdict)
		}
		if !ok {
			continue
		}
		out = append(out, bizknowledge.WriteBackArbitration{
			FactIndex:    v.FactIndex,
			Operation:    op,
			TargetFactID: strings.TrimSpace(v.TargetFactID),
			Confidence:   v.Confidence,
			Reason:       truncateRunes(strings.TrimSpace(v.Reason), 200),
		})
	}
	return out, nil
}

// 编译期接口断言。
var _ bizknowledge.WriteBackArbiter = (*WriteBackArbiter)(nil)
