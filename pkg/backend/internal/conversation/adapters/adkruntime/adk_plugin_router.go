package adkruntime

import (
	"log"
	"strings"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/plugin"
)

func newModelRouterPlugin() (*plugin.Plugin, error) {
	return newModelRouterPluginWithConfig(nil)
}

func newModelRouterPluginWithConfig(config map[string]any) (*plugin.Plugin, error) {
	cfg := modelRouterConfigFromConfig(config)
	return plugin.New(plugin.Config{
		Name: "model_router",
		BeforeModelCallback: func(ctx agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			if req == nil {
				return nil, nil
			}
			current := strings.TrimSpace(req.Model)
			if agentModel := cfg.AgentModels[callbackAgentName(ctx)]; agentModel != "" {
				req.Model = agentModel
				log.Printf("adk plugin model_router action=route reason=agent model=%q previous=%q", req.Model, current)
				return nil, nil
			}
			prompt := llmRequestPreview(req)
			promptTokens := estimateTextTokens(prompt)
			if cfg.LongContextModel != "" && cfg.LongContextThreshold > 0 && promptTokens >= cfg.LongContextThreshold {
				req.Model = cfg.LongContextModel
				log.Printf("adk plugin model_router action=route reason=long_context model=%q previous=%q prompt_tokens=%d threshold=%d", req.Model, current, promptTokens, cfg.LongContextThreshold)
				return nil, nil
			}
			if cfg.CodeModel != "" && looksLikeCodeTask(prompt) {
				req.Model = cfg.CodeModel
				log.Printf("adk plugin model_router action=route reason=code_task model=%q previous=%q", req.Model, current)
				return nil, nil
			}
			if cfg.DefaultModel != "" && strings.TrimSpace(req.Model) == "" {
				req.Model = cfg.DefaultModel
			}
			return nil, nil
		},
	})
}
