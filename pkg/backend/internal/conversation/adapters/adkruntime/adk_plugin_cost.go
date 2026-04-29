package adkruntime

import (
	"log"
	"strings"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/plugin"
)

func newCostGuardPlugin() (*plugin.Plugin, error) {
	return newCostGuardPluginWithConfig(nil)
}

func newCostGuardPluginWithConfig(config map[string]any) (*plugin.Plugin, error) {
	cfg := costGuardConfigFromConfig(config)
	return plugin.New(plugin.Config{
		Name: "cost_guard",
		BeforeModelCallback: func(_ agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			if req == nil {
				return nil, nil
			}
			if req.Model == "" {
				req.Model = cfg.DefaultModel
			}
			promptTokens := estimateTextTokens(llmRequestPreview(req))
			if cfg.MaxPromptTokens > 0 && promptTokens > cfg.MaxPromptTokens {
				if cfg.FallbackModel != "" {
					log.Printf("adk plugin cost_guard action=fallback reason=prompt_tokens model=%q fallback=%q prompt_tokens=%d max=%d", req.Model, cfg.FallbackModel, promptTokens, cfg.MaxPromptTokens)
					req.Model = cfg.FallbackModel
					return nil, nil
				}
				return blockedModelResponse("Prompt token budget exceeded."), nil
			}
			if cfg.BlockedModels[strings.ToLower(req.Model)] || (cfg.BlockPremiumModels && isPremiumModel(req.Model)) {
				if cfg.FallbackModel != "" {
					log.Printf("adk plugin cost_guard action=fallback reason=blocked_model model=%q fallback=%q", req.Model, cfg.FallbackModel)
					req.Model = cfg.FallbackModel
					return nil, nil
				}
				return blockedModelResponse("Model is blocked by cost policy."), nil
			}
			return nil, nil
		},
	})
}
