package provider

// Package provider resolves catalog-backed LLM calls to concrete model implementations.
//
// Runtime path:
//
//   - trpc path (trpc.group/trpc-go/trpc-agent-go/model): TRPCModelForProviderModel →
//     trpcModelFromCatalogConfig → model/openai|anthropic|gemini|ollama|hunyuan.
//     Uses native provider support, high availability (failover/hedge), and token tailoring.
//
// CatalogConfig is the shared configuration structure bridging biz.ProviderModel to the trpc model factory.
