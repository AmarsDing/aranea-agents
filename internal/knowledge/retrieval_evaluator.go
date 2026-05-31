package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

const evalTimeout = 10 * time.Second

type RetrievalAssessment struct {
	Sufficient      bool
	Confidence      float32
	SupplementQuery string
}

type RetrievalEvaluator struct {
	llm     biz.LLMCaller
	sys     *biz.SystemSettingUsecase
	catalog *biz.LlmProviderModelUsecase
}

func NewRetrievalEvaluator(llm biz.LLMCaller, sys *biz.SystemSettingUsecase, catalog *biz.LlmProviderModelUsecase) *RetrievalEvaluator {
	return &RetrievalEvaluator{llm: llm, sys: sys, catalog: catalog}
}

func (e *RetrievalEvaluator) Evaluate(ctx context.Context, query string, chunks []biz.KnowledgeChunk) (*RetrievalAssessment, error) {
	if len(chunks) == 0 {
		return &RetrievalAssessment{Sufficient: false, Confidence: 0, SupplementQuery: query}, nil
	}

	// Degradation strategy 1 (conservative): when no LLM is configured, assume
	// existing results are sufficient. The caller can still decide to do a
	// follow-up search based on domain heuristics.
	if e.llm == nil {
		return &RetrievalAssessment{Sufficient: true, Confidence: 1.0}, nil
	}

	// Degradation strategy 2 (conservative): when the LLM model cannot be
	// resolved (e.g. no evaluation model configured in system settings), treat
	// existing results as sufficient rather than triggering unnecessary
	// supplementary searches that would waste tokens and latency.
	provider, model, err := e.resolveModel(ctx)
	if err != nil {
		return &RetrievalAssessment{Sufficient: true, Confidence: 0.5}, nil
	}

	evalCtx, cancel := context.WithTimeout(ctx, evalTimeout)
	defer cancel()

	chunksSummary := buildChunksSummary(chunks, 2000)

	sys := `你是一名检索质量评估助手。给定用户问题和检索到的文本片段，评估检索结果是否足以回答用户问题。

请以 JSON 格式输出评估结果：
{"sufficient": true/false, "confidence": 0.0-1.0, "supplement_query": "补充查询（仅在 insufficient 时填写）"}

判断标准：
- sufficient: 检索到的片段是否包含足够信息回答用户问题
- confidence: 对判断的置信度（0.0-1.0）
- supplement_query: 如果 insufficient，建议的补充搜索查询

只输出 JSON，不要任何解释。`

	user := fmt.Sprintf("用户问题：%s\n\n检索到的文本片段：\n%s", query, chunksSummary)

	text, _, err := e.llm.Call(evalCtx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System:   sys,
		User:     user,
	})
	if err != nil {
		loggateway.Global().Warn("检索质量评估失败，降级为需要补充检索",
			loggateway.StepID("knowledge.retrieval_eval.fail"),
			loggateway.Err(err))
		// Degradation strategy 3 (safe): when the LLM call itself fails, we
		// cannot confirm the results are sufficient, so mark as insufficient
		// with the original query as supplement. This errs on the side of
		// recall — the caller may perform a supplementary search.
		return &RetrievalAssessment{Sufficient: false, Confidence: 0, SupplementQuery: query}, nil
	}

	return parseAssessment(text)
}

func (e *RetrievalEvaluator) resolveModel(ctx context.Context) (string, string, error) {
	var sys RefineLLMSettingsGetter
	if e.sys != nil {
		sys = e.sys
	}
	var cat LLMCatalogLister
	if e.catalog != nil {
		cat = e.catalog
	}
	return ResolveLLM(ctx, sys, cat, "retrieval evaluation")
}

func buildChunksSummary(chunks []biz.KnowledgeChunk, maxChars int) string {
	var b strings.Builder
	for i, ch := range chunks {
		if b.Len() > maxChars {
			break
		}
		b.WriteString(fmt.Sprintf("[片段%d] %s\n", i+1, truncateString(ch.Content, 200)))
	}
	return b.String()
}

func truncateString(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

func parseAssessment(raw string) (*RetrievalAssessment, error) {
	raw = strings.TrimSpace(raw)
	raw = stripCodeFenceJSON(raw)

	type assessJSON struct {
		Sufficient      bool    `json:"sufficient"`
		Confidence      float32 `json:"confidence"`
		SupplementQuery string  `json:"supplement_query"`
	}
	var a assessJSON
	if err := parseJSONLoose(raw, &a); err != nil {
		return &RetrievalAssessment{Sufficient: true, Confidence: 0.5}, nil
	}
	return &RetrievalAssessment{
		Sufficient:      a.Sufficient,
		Confidence:      a.Confidence,
		SupplementQuery: strings.TrimSpace(a.SupplementQuery),
	}, nil
}

func parseJSONLoose(s string, v any) error {
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < 0 || end <= start {
		return kerrors.InternalServer("KNOWLEDGE", "no JSON object found in LLM response")
	}
	return json.Unmarshal([]byte(s[start:end+1]), v)
}


