// Package provider routes catalog-backed LLM calls through implementations of
// google.golang.org/adk/model.LLM (same contract as pkg/trpc-agent-go/model).
//
// Subpackages provider/openai, provider/deepseek, and provider/gemini supply backends;
// Registry.Resolve binds catalog provider_type to a factory.
//
// Biz is the configuration source-of-truth: biz.ProviderModel + ConfigJSON (provider_type, api_base_url, api_key).
package provider
