package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
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
	if e.llm == nil {
		return &RetrievalAssessment{Sufficient: true, Confidence: 1.0}, nil
	}
	if len(chunks) == 0 {
		return &RetrievalAssessment{Sufficient: false, Confidence: 0, SupplementQuery: query}, nil
	}

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
		event.SysLogWarn("knowledge.retrieval_eval.fail", "检索质量评估失败",
			event.P("error", err.Error()))
		return &RetrievalAssessment{Sufficient: true, Confidence: 0.5}, nil
	}

	return parseAssessment(text)
}

func (e *RetrievalEvaluator) resolveModel(ctx context.Context) (string, string, error) {
	if e.sys != nil {
		s, err := e.sys.Get(ctx)
		if err == nil && strings.TrimSpace(s.DefaultRefineLLM.Provider) != "" && strings.TrimSpace(s.DefaultRefineLLM.Model) != "" {
			return s.DefaultRefineLLM.Provider, s.DefaultRefineLLM.Model, nil
		}
	}
	if e.catalog != nil {
		models, err := e.catalog.List(ctx)
		if err == nil {
			for _, m := range models {
				if m.Provider != "" && m.Model != "" && m.Enabled {
					return m.Provider, m.Model, nil
				}
			}
		}
	}
	return "", "", fmt.Errorf("no LLM available for retrieval evaluation")
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
		return fmt.Errorf("no JSON object found")
	}
	return jsonUnmarshal([]byte(s[start:end+1]), v)
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
