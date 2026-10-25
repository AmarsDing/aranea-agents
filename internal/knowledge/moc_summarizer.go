package knowledge

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// ── G-34 MOC 摘要生成器（ADR-KN-20261021-C 生产实现）─────────────────────────
//
// biz KnowledgeCurateRepo.ListPendingMOCSummaries 扫出占位 MOC 页后，由本实现
// 一次性生成「## 摘要」正文：hub 词条节选居首 + 簇成员节选保序装帧 → 单轮
// completion → 2~4 句中文摘要（纯文本，无 JSON 契约）。失败上抛 error（biz
// 侧 Warn 降级、占位保留下轮重试）；usage 经窄口 MOCUsageRecorder 记账
//（aux_moc_summary，晚绑同 embedUsageRecorder 模式）。

const (
	// mocSummarizeTimeout 单次摘要 LLM 调用超时（对齐 writeback arbitrate）。
	mocSummarizeTimeout = 60 * time.Second
	// mocSummaryMaxRunes 产出摘要正文 rune 上限（超出截断；防失控长文进页）。
	mocSummaryMaxRunes = 300
)

// MOCUsageInput MOC 摘要 usage 记账入参（CompletionTok 承载 totalTokens——
// biz.LLMCaller 聚合口径，无 prompt/completion 拆分，UsageSourceResponse 计）。
type MOCUsageInput struct {
	Provider string
	Model    string
	Tokens   int
	Latency  time.Duration
	DocID    string
	RelPath  string
	Err      error
}

// MOCUsageRecorder MOC 摘要 usage 记账窄口（生产 = service.mocUsageRecorder；
// nil = 不记录）。
type MOCUsageRecorder interface {
	RecordMOCSummaryUsage(ctx context.Context, in MOCUsageInput)
}

// MOCSummarizer 实现 bizknowledge.MOCSummarizer（LLM 单轮摘要）。
type MOCSummarizer struct {
	llm      biz.LLMCaller
	sys      RefineLLMSettingsGetter
	catalog  LLMCatalogLister
	usageRec MOCUsageRecorder
	lg       loggateway.Logger
}

// NewMOCSummarizer 构造摘要器；lg 为 nil 时降级 Noop。
func NewMOCSummarizer(llm biz.LLMCaller, sys RefineLLMSettingsGetter, catalog LLMCatalogLister, lg loggateway.Logger) *MOCSummarizer {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &MOCSummarizer{llm: llm, sys: sys, catalog: catalog, lg: lg}
}

// SetUsageRecorder 注入 usage 记录器（装配层调用，同 embedder SetUsageRecorder 模式）。
func (s *MOCSummarizer) SetUsageRecorder(r MOCUsageRecorder) {
	if s == nil {
		return
	}
	s.usageRec = r
}

// MOCSummarizerDisabled 环境开关（与其他 worker 同纪律）：置 1 时生产装配跳过接线。
func MOCSummarizerDisabled() bool {
	return strings.TrimSpace(os.Getenv("KNOWLEDGE_MOC_SUMMARIZER_DISABLED")) == "1"
}

// SummarizeMOC 生成 MOC 页「## 摘要」正文（2~4 句中文，纯文本无 markdown）。
// LLM/解析失败上抛 error；usage 记账（成败均记，失败 Tokens=0）。
func (s *MOCSummarizer) SummarizeMOC(ctx context.Context, req bizknowledge.MOCSummaryRequest) (bizknowledge.MOCSummaryResult, error) {
	if s == nil || s.llm == nil {
		return bizknowledge.MOCSummaryResult{}, fmt.Errorf("moc summarizer not wired")
	}
	provider, model, err := ResolveLLM(ctx, s.sys, s.catalog, "moc summary", s.lg)
	if err != nil {
		return bizknowledge.MOCSummaryResult{}, err
	}

	var b strings.Builder
	b.WriteString("MOC 主题：" + req.Title + "\n\n")
	b.WriteString("核心词条（hub）《" + relBase(req.Hub.RelPath) + "》节选：\n" + strings.TrimSpace(req.Hub.Body) + "\n")
	for _, m := range req.Members {
		b.WriteString("\n关联词条《" + relBase(m.RelPath) + "》节选：\n" + strings.TrimSpace(m.Body) + "\n")
	}
	if req.Truncated {
		b.WriteString("\n（部分词条因长度预算被截断，摘要仅基于以上节选。）\n")
	}

	callCtx, cancel := context.WithTimeout(ctx, mocSummarizeTimeout)
	start := time.Now()
	resp, totalTokens, callErr := s.llm.Call(callCtx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System: `你是知识库 MOC（Map of Content）摘要撰写助手。给定一个主题名、核心词条节选与簇内关联词条节选，撰写该知识簇的「摘要」段正文。
要求：
1. 2~4 句中文，概括簇的公共主题、各成员关注点及互补关系；
2. 直接输出摘要正文，不要标题、列表、markdown 标记或前后缀说明；
3. 忠于所给节选内容，不臆造未出现的概念；节选被截断时不得编造未给到的细节。`,
		User: b.String(),
	})
	cancel()
	s.recordUsage(ctx, req, provider, model, totalTokens, time.Since(start), callErr)
	if callErr != nil {
		return bizknowledge.MOCSummaryResult{}, callErr
	}
	summary := strings.TrimSpace(resp)
	// LLM 偶发包裹/引号/行首标记剥除；失控长文截断。
	summary = strings.Trim(summary, "\"'`「」“” \t")
	if r := []rune(summary); len(r) > mocSummaryMaxRunes {
		summary = strings.TrimSpace(string(r[:mocSummaryMaxRunes]))
	}
	if summary == "" {
		return bizknowledge.MOCSummaryResult{}, fmt.Errorf("moc summary empty")
	}
	return bizknowledge.MOCSummaryResult{Summary: summary}, nil
}

// recordUsage usage 记账（best-effort：recorder 内部 Warn 降级，不污染主路径）。
// F12 终审加固：零 token 成功行不记账（provider 漏报 usage 时塌缩 get 为 0，
// 幻影成功行会虚报调用量——title/memory/embed 三先例同纪律）；失败行照记。
func (s *MOCSummarizer) recordUsage(ctx context.Context, req bizknowledge.MOCSummaryRequest, provider, model string, tokens int, latency time.Duration, callErr error) {
	if tokens <= 0 && callErr == nil {
		return
	}
	rec := s.usageRec
	if rec == nil {
		return
	}
	rec.RecordMOCSummaryUsage(ctx, MOCUsageInput{
		Provider: provider, Model: model, Tokens: tokens, Latency: latency,
		DocID: req.DocID, RelPath: req.RelPath, Err: callErr,
	})
}

// relBase rel_path 末段（去 .md）——prompt 展示名。
func relBase(rel string) string {
	base := rel[strings.LastIndex(rel, "/")+1:]
	return strings.TrimSuffix(base, ".md")
}

// 编译期接口断言。
var _ bizknowledge.MOCSummarizer = (*MOCSummarizer)(nil)
