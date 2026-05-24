package biz

import (
	"context"
	"errors"
)

// ErrLLMExtractorUnavailable indicates no LLM-backed extractor is wired.
var ErrLLMExtractorUnavailable = errors.New("memory: llm extractor not configured")

// MemoryTextExtractor turns recent messages into memory proposals via an LLM call.
type MemoryTextExtractor interface {
	ExtractFacts(ctx context.Context, in ConsolidateInput) ([]MemoryProposal, error)
}

// LLMConsolidator delegates extraction to MemoryTextExtractor.
type LLMConsolidator struct {
	extractor MemoryTextExtractor
}

func NewLLMConsolidator(extractor MemoryTextExtractor) *LLMConsolidator {
	return &LLMConsolidator{extractor: extractor}
}

func (c *LLMConsolidator) Extract(ctx context.Context, in ConsolidateInput) ([]MemoryProposal, error) {
	if c == nil || c.extractor == nil {
		return nil, ErrLLMExtractorUnavailable
	}
	return c.extractor.ExtractFacts(ctx, in)
}

// DefaultMemoryConsolidator returns LLM→heuristic chain; without extractor only heuristic runs.
func DefaultMemoryConsolidator(extractor MemoryTextExtractor) MemoryConsolidator {
	heuristic := NewHeuristicConsolidator()
	if extractor == nil {
		return heuristic
	}
	return NewChainConsolidator(NewLLMConsolidator(extractor), heuristic)
}
