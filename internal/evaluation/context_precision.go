package evaluation

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	evalset "trpc.group/trpc-go/trpc-agent-go/evaluation/evalset"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/runner"
)

const metricContextPrecision = "context_precision"

const maxPrecisionChunks = 12

const contextPrecisionJudgePrompt = `You are scoring retrieval context precision.

User query:
%s

Retrieved chunks:
%s

List the 0-based indices of chunks that are relevant to the query.
Ignore fluency. A chunk is relevant only if it could help answer the query.

Reply with EXACTLY one line and no extra commentary:
relevant: <comma-separated indices, or empty>
`

var relevantIndexLine = regexp.MustCompile(`(?i)relevant\s*[:=]\s*(.*)`)

func retrievedChunks(tools []*evalset.Tool) []string {
	var out []string
	for _, t := range tools {
		if t == nil || !isKnowledgeSearchTool(t.Name) {
			continue
		}
		text := toolResultText(t.Result)
		for _, part := range strings.Split(text, "\n") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			out = append(out, part)
			if len(out) >= maxPrecisionChunks {
				return out
			}
		}
	}
	return out
}

func scoreContextPrecision(ctx context.Context, judge runner.Runner, query string, chunks []string) (float32, bool) {
	if len(chunks) == 0 || strings.TrimSpace(query) == "" {
		return 0, false
	}
	if judge != nil {
		if scored, err := judgeContextPrecision(ctx, judge, query, chunks); err == nil {
			return scored, true
		}
	}
	return lexicalContextPrecision(query, chunks), true
}

func lexicalContextPrecision(query string, chunks []string) float32 {
	q := contentTokens(query)
	if len(q) == 0 || len(chunks) == 0 {
		return 0
	}
	qset := map[string]struct{}{}
	for _, t := range q {
		qset[t] = struct{}{}
	}
	hit := 0
	for _, ch := range chunks {
		if chunkRelevantToQuery(qset, ch) {
			hit++
		}
	}
	return float32(hit) / float32(len(chunks))
}

// lexicalStopWords drops function words so a shared "is"/"the" cannot
// mark an unrelated chunk relevant.
var lexicalStopWords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {}, "be": {},
	"by": {}, "for": {}, "from": {}, "in": {}, "is": {}, "it": {}, "of": {},
	"on": {}, "or": {}, "that": {}, "the": {}, "to": {}, "was": {}, "were": {},
	"what": {}, "when": {}, "where": {}, "which": {}, "who": {}, "with": {},
}

func contentTokens(s string) []string {
	var out []string
	for _, t := range tokenizeFaithfulness(s) {
		if _, stop := lexicalStopWords[t]; stop {
			continue
		}
		out = append(out, t)
	}
	return out
}

func chunkRelevantToQuery(qset map[string]struct{}, chunk string) bool {
	toks := contentTokens(chunk)
	if len(toks) == 0 {
		return false
	}
	n := 0
	for _, t := range toks {
		if _, ok := qset[t]; ok {
			n++
		}
	}
	// Require a real content overlap: two hits, or ≥15% of chunk tokens
	// with at least one content token in common.
	return n >= 2 || (n >= 1 && float32(n)/float32(len(toks)) >= 0.15)
}

func judgeContextPrecision(ctx context.Context, judge runner.Runner, query string, chunks []string) (float32, error) {
	if judge == nil {
		return 0, fmt.Errorf("judge runner is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var b strings.Builder
	for i, ch := range chunks {
		fmt.Fprintf(&b, "[%d] %s\n", i, ch)
	}
	ch, err := judge.Run(ctx, "eval", "context-precision", trpcmodel.NewUserMessage(
		fmt.Sprintf(contextPrecisionJudgePrompt, query, b.String()),
	))
	if err != nil {
		return 0, err
	}
	idx := parseRelevantIndices(collectAssistantText(ch), len(chunks))
	return float32(len(idx)) / float32(len(chunks)), nil
}

func parseRelevantIndices(raw string, n int) []int {
	raw = strings.TrimSpace(raw)
	if raw == "" || n <= 0 {
		return nil
	}
	payload := raw
	if m := relevantIndexLine.FindStringSubmatch(raw); len(m) == 2 {
		payload = m[1]
	}
	seen := map[int]struct{}{}
	var out []int
	for _, part := range strings.FieldsFunc(payload, func(r rune) bool {
		return r == ',' || r == ' ' || r == ';'
	}) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		v, err := strconv.Atoi(part)
		if err != nil || v < 0 || v >= n {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
