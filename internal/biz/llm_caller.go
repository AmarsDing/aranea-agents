package biz

import "context"

// LLMCallRequest carries everything the caller needs to make a single completion.
// PromptRefiner.Refine produces this struct after resolveLLM picks the LLM via
// the 3-tier strategy; the underlying caller MUST honor Provider+Model and SHOULD
// look up BaseURL+APIKey from its own infrastructure (the biz layer doesn't know
// the catalog schema details, so credentials are left to the implementation).
//
// PGO-3-BIZ-01 (B-1 / B-4 fix): previously LLMCaller.Call only took (system, user),
// which forced the implementation to re-resolve the LLM independently. That made
// the reported (Provider, Model) inconsistent with the actual call.
type LLMCallRequest struct {
	Provider string // logical provider, e.g. "openai", "anthropic", "openrouter"
	Model    string // concrete model name, e.g. "gpt-4o", "claude-3-5-sonnet-20241022"
	System   string
	User     string
}

// LLMCaller is a minimal interface for single-turn LLM completions.
// biz MUST NOT import pkg/trpc-agent-go; the interface is the boundary.
//
// Concrete implementations:
//   - internal/agent.DynamicLLMCaller — resolves BaseURL+APIKey from SystemSetting / Catalog
//     for the given Provider+Model.
//   - internal/agent.OpenAICompatLLMCaller — uses a statically-configured ProviderAPIConfig
//     (mainly for tests and fixed-credential scenarios).
type LLMCaller interface {
	Call(ctx context.Context, req LLMCallRequest) (text string, totalTokens int, err error)
}

// LLMCallerConfig is the static configuration for OpenAICompatLLMCaller.
// Used only by tests / fixed-credential callers; production code uses
// DynamicLLMCaller which resolves credentials at call time.
type LLMCallerConfig struct {
	BaseURL    string
	APIKey     string
	Provider   string
	Model      string
	MaxTokens  int
	TimeoutSec int
}
