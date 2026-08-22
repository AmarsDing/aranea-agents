package evaluation

import (
	"encoding/json"
	"strings"
	"unicode"

	"aranea-agents/internal/biz"
	evalset "trpc.group/trpc-go/trpc-agent-go/evaluation/evalset"
)

const metricFaithfulness = "faithfulness"

func applyFaithfulness(res *biz.EvalCaseResult, inv *evalset.Invocation) {
	if res == nil || inv == nil {
		return
	}
	ctxText, retrieved := retrievedContext(inv.Tools)
	if !retrieved {
		return
	}
	score := lexicalFaithfulness(res.ActualOutput, ctxText)
	scores := biz.ParseEvalScores(res.ScoresJSON)
	scores[metricFaithfulness] = score
	res.ScoresJSON = biz.MarshalEvalScores(scores)
}

func retrievedContext(tools []*evalset.Tool) (text string, ok bool) {
	var b strings.Builder
	for _, t := range tools {
		if t == nil || !isKnowledgeSearchTool(t.Name) {
			continue
		}
		ok = true
		b.WriteString(toolResultText(t.Result))
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String()), ok
}

func isKnowledgeSearchTool(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return n == "knowledge_search" || n == "knowledge.search" || strings.Contains(n, "knowledge_search")
}

func toolResultText(result any) string {
	if result == nil {
		return ""
	}
	switch v := result.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		var obj map[string]any
		if json.Unmarshal(raw, &obj) == nil {
			if chunks, ok := obj["chunks"].([]any); ok {
				var b strings.Builder
				for _, c := range chunks {
					cm, _ := c.(map[string]any)
					for _, key := range []string{"content", "line", "text", "chunk"} {
						if s, ok := cm[key].(string); ok && s != "" {
							b.WriteString(s)
							b.WriteByte('\n')
							break
						}
					}
				}
				if b.Len() > 0 {
					return b.String()
				}
			}
		}
		return string(raw)
	}
}

func lexicalFaithfulness(output, context string) float32 {
	outToks := tokenizeFaithfulness(output)
	if len(outToks) == 0 {
		return 0
	}
	ctx := map[string]struct{}{}
	for _, t := range tokenizeFaithfulness(context) {
		ctx[t] = struct{}{}
	}
	if len(ctx) == 0 {
		return 0
	}
	hit := 0
	for _, t := range outToks {
		if _, ok := ctx[t]; ok {
			hit++
		}
	}
	return float32(hit) / float32(len(outToks))
}

func tokenizeFaithfulness(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}
	var tok strings.Builder
	var out []string
	flush := func() {
		t := tok.String()
		tok.Reset()
		if len(t) >= 2 {
			out = append(out, t)
		}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			tok.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func averageFaithfulness(results []biz.EvalCaseResult) (float32, bool) {
	var sum float32
	n := 0
	for _, r := range results {
		if v, ok := biz.ParseEvalScores(r.ScoresJSON)[metricFaithfulness]; ok {
			sum += v
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float32(n), true
}
