package service

import (
	"testing"

	"aranea-agents/internal/biz"
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
