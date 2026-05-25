package agent

import (
	"aranea-agents/internal/llmcontext"
)

// RoughTokenEstimate approximates token count (~4 runes per token, display-only fallback).
func RoughTokenEstimate(s string) int {
	return llmcontext.RoughTokenEstimate(s)
}
