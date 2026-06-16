//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/model"
)

// QMSumMetrics stores answer-quality metrics for one mode.
type QMSumMetrics struct {
	F1       float64 `json:"f1"`
	BLEU     float64 `json:"bleu"`
	ROUGE1   float64 `json:"rouge_1"`
	ROUGE2   float64 `json:"rouge_2"`
	ROUGEL   float64 `json:"rouge_l"`
	LLMScore float64 `json:"llm_score,omitempty"`
}

type qmsumLLMJudge struct {
	model model.Model
}

type qmsumLLMJudgeResult struct {
	Correct    bool    `json:"correct"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

func newQMSumLLMJudge(m model.Model) *qmsumLLMJudge {
	return &qmsumLLMJudge{model: m}
}

func evaluateQMSumMetrics(
	ctx context.Context,
	judge *qmsumLLMJudge,
	query, reference, prediction string,
) *QMSumMetrics {
	result := &QMSumMetrics{
		F1:     calculateF1(prediction, reference),
		BLEU:   calculateBLEU(prediction, reference),
		ROUGE1: calculateROUGE1(prediction, reference),
		ROUGE2: calculateROUGE2(prediction, reference),
		ROUGEL: calculateROUGEL(prediction, reference),
	}
	if judge != nil {
		if eval, err := judge.Evaluate(ctx, query, reference, prediction); err == nil && eval.Correct {
			result.LLMScore = eval.Confidence
		}
	}
	return result
}

const qmsumJudgePrompt = `You are an expert evaluator for query-based meeting summarization.

Query: %s
Reference Answer: %s
Predicted Answer: %s

Judge whether the predicted answer is faithful and semantically equivalent to the reference answer for the given query.
Respond with a JSON object:
{"correct": true/false, "confidence": 0.0-1.0, "reason": "brief explanation"}`

func (j *qmsumLLMJudge) Evaluate(
	ctx context.Context,
	query, reference, prediction string,
) (*qmsumLLMJudgeResult, error) {
	if j == nil || j.model == nil {
		return nil, fmt.Errorf("LLM judge is not configured")
	}

	req := &model.Request{
		Messages: []model.Message{
			model.NewUserMessage(
				fmt.Sprintf(qmsumJudgePrompt, query, reference, prediction),
			),
		},
		GenerationConfig: model.GenerationConfig{Stream: false},
	}
	respCh, err := j.model.GenerateContent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate judge response: %w", err)
	}

	var content strings.Builder
	for resp := range respCh {
		if resp.Error != nil {
			return nil, fmt.Errorf("judge response error: %s", resp.Error.Message)
		}
		if len(resp.Choices) > 0 {
			content.WriteString(resp.Choices[0].Message.Content)
		}
	}
	return parseQMSumJudgeResponse(content.String())
}

func parseQMSumJudgeResponse(content string) (*qmsumLLMJudgeResult, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}

	var result qmsumLLMJudgeResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		lower := strings.ToLower(content)
		result.Correct = strings.Contains(lower, "true") ||
			strings.Contains(lower, "correct")
		result.Confidence = 0.5
		result.Reason = "failed to parse JSON response"
	}
	if result.Confidence < 0 {
		result.Confidence = -result.Confidence
	}
	if result.Confidence > 1 {
		result.Confidence = 1
	}
	return &result, nil
}

func calculateF1(prediction, reference string) float64 {
	predTokens := normalizeAndTokenize(prediction)
	refTokens := normalizeAndTokenize(reference)
	if len(predTokens) == 0 || len(refTokens) == 0 {
		if len(predTokens) == 0 && len(refTokens) == 0 {
			return 1.0
		}
		return 0.0
	}
	common := countCommonTokens(predTokens, refTokens)
	precision := float64(common) / float64(len(predTokens))
	recall := float64(common) / float64(len(refTokens))
	if precision+recall == 0 {
		return 0.0
	}
	return 2 * precision * recall / (precision + recall)
}

func calculateBLEU(prediction, reference string) float64 {
	predTokens := normalizeAndTokenize(prediction)
	refTokens := normalizeAndTokenize(reference)
	if len(predTokens) == 0 {
		if len(refTokens) == 0 {
			return 1.0
		}
		return 0.0
	}

	refCounts := make(map[string]int)
	for _, token := range refTokens {
		refCounts[token]++
	}

	matches := 0
	for _, token := range predTokens {
		if refCounts[token] > 0 {
			matches++
			refCounts[token]--
		}
	}

	precision := float64(matches) / float64(len(predTokens))
	return brevityPenalty(len(predTokens), len(refTokens)) * precision
}

func calculateROUGE1(prediction, reference string) float64 {
	return calculateRougeN(
		normalizeAndTokenize(prediction),
		normalizeAndTokenize(reference),
		1,
	)
}

func calculateROUGE2(prediction, reference string) float64 {
	return calculateRougeN(
		normalizeAndTokenize(prediction),
		normalizeAndTokenize(reference),
		2,
	)
}

func calculateROUGEL(prediction, reference string) float64 {
	predTokens := normalizeAndTokenize(prediction)
	refTokens := normalizeAndTokenize(reference)
	if len(predTokens) == 0 || len(refTokens) == 0 {
		if len(predTokens) == 0 && len(refTokens) == 0 {
			return 1.0
		}
		return 0.0
	}

	lcs := lcsLength(predTokens, refTokens)
	precision := float64(lcs) / float64(len(predTokens))
	recall := float64(lcs) / float64(len(refTokens))
	if precision+recall == 0 {
		return 0.0
	}
	return 2 * precision * recall / (precision + recall)
}

func normalizeAndTokenize(text string) []string {
	if text == "" {
		return nil
	}
	text = strings.ReplaceAll(text, "<｜end▁of▁sentence｜>", " ")
	text = strings.ToLower(text)
	text = removePunctuation(text)
	fields := strings.Fields(text)
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			result = append(result, field)
		}
	}
	return result
}

var punctuationRegex = regexp.MustCompile(`[^\p{L}\p{N}\s]`)

func removePunctuation(text string) string {
	return punctuationRegex.ReplaceAllString(text, " ")
}

func countCommonTokens(predTokens, refTokens []string) int {
	refCounts := make(map[string]int)
	for _, token := range refTokens {
		refCounts[token]++
	}
	common := 0
	for _, token := range predTokens {
		if refCounts[token] > 0 {
			common++
			refCounts[token]--
		}
	}
	return common
}

func brevityPenalty(predLen, refLen int) float64 {
	if predLen == 0 {
		return 0
	}
	if predLen >= refLen {
		return 1.0
	}
	return math.Exp(1 - float64(refLen)/float64(predLen))
}

func calculateRougeN(predTokens, refTokens []string, n int) float64 {
	if len(predTokens) < n || len(refTokens) < n {
		if len(predTokens) == 0 && len(refTokens) == 0 {
			return 1.0
		}
		return 0.0
	}

	predNgrams := extractNgrams(predTokens, n)
	refNgrams := extractNgrams(refTokens, n)

	matches := 0
	for ngram, count := range predNgrams {
		if refCount, ok := refNgrams[ngram]; ok {
			if count < refCount {
				matches += count
			} else {
				matches += refCount
			}
		}
	}

	totalPred := 0
	for _, count := range predNgrams {
		totalPred += count
	}
	totalRef := 0
	for _, count := range refNgrams {
		totalRef += count
	}
	if totalPred == 0 || totalRef == 0 {
		return 0.0
	}

	precision := float64(matches) / float64(totalPred)
	recall := float64(matches) / float64(totalRef)
	if precision+recall == 0 {
		return 0.0
	}
	return 2 * precision * recall / (precision + recall)
}

func extractNgrams(tokens []string, n int) map[string]int {
	ngrams := make(map[string]int)
	for i := 0; i <= len(tokens)-n; i++ {
		ngrams[strings.Join(tokens[i:i+n], " ")]++
	}
	return ngrams
}

func lcsLength(a, b []string) int {
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1] + 1
			} else if curr[j-1] > prev[j] {
				curr[j] = curr[j-1]
			} else {
				curr[j] = prev[j]
			}
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}
