package service

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
)

func TestMemoryWorkerProviderModel_PrefersMemoryWorker(t *testing.T) {
	ag := biz.Agent{
		Provider: "openai",
		Model:    "gpt-4",
		Settings: &biz.AgentRuntimeSettings{
			MemoryWorkerProvider: "openai",
			MemoryWorkerModel:    "gpt-4o-mini",
			L0CompressProvider:   "anthropic",
			L0CompressModel:      "claude-3-haiku",
		},
	}
	p, m := memoryWorkerProviderModel(biz.Session{}, ag)
	if p != "openai" || m != "gpt-4o-mini" {
		t.Fatalf("got %s/%s", p, m)
	}
}

func TestMemoryWorkerProviderModel_PrefersL0Compress(t *testing.T) {
	ag := biz.Agent{
		Provider: "openai",
		Model:    "gpt-4",
		Settings: &biz.AgentRuntimeSettings{
			L0CompressProvider: "anthropic",
			L0CompressModel:    "claude-3-haiku",
		},
	}
	p, m := memoryWorkerProviderModel(biz.Session{}, ag)
	if p != "anthropic" || m != "claude-3-haiku" {
		t.Fatalf("got %s/%s", p, m)
	}
}

func TestMemoryWorkerProviderModel_FallsBackToSessionAgent(t *testing.T) {
	ag := biz.Agent{Provider: "openai", Model: "gpt-4"}
	sess := biz.Session{DefaultProvider: "deepseek", DefaultModel: "deepseek-chat"}
	p, m := memoryWorkerProviderModel(sess, ag)
	if p != "deepseek" || m != "deepseek-chat" {
		t.Fatalf("got %s/%s", p, m)
	}
}

func TestConvertFactsToProposals_PassesThroughClassification(t *testing.T) {
	facts := []compress.MemoryExtractFact{{
		Statement:   "User asked to never use the shell tool",
		SubjectType: "constraint",
		Scope:       "user",
		Confidence:  0.9,
	}}
	props := convertFactsToProposals(facts, nil, biz.ExtractionQualityFunctionCall)
	if len(props) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(props))
	}
	p := props[0]
	if p.SubjectType != "constraint" {
		t.Fatalf("SubjectType=%q want constraint", p.SubjectType)
	}
	if p.Scope != "user" {
		t.Fatalf("Scope=%q want user", p.Scope)
	}
	if p.Confidence != 0.9 {
		t.Fatalf("Confidence=%v want 0.9", p.Confidence)
	}
}
