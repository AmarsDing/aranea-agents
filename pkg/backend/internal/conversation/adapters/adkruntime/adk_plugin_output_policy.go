package adkruntime

import (
	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/plugin"
	"google.golang.org/genai"
)

func newOutputPolicyPlugin() (*plugin.Plugin, error) {
	return newOutputPolicyPluginWithConfig(nil)
}

func newOutputPolicyPluginWithConfig(config map[string]any) (*plugin.Plugin, error) {
	blockedPatterns := configStringSlice(config, "blocked_patterns")
	return plugin.New(plugin.Config{
		Name: "output_policy",
		AfterModelCallback: func(_ agent.CallbackContext, resp *model.LLMResponse, responseErr error) (*model.LLMResponse, error) {
			if resp == nil || responseErr != nil {
				return nil, nil
			}
			text := llmResponseText(resp)
			if reason := outputPolicyViolation(text, blockedPatterns); reason != "" {
				message := "Output blocked by policy: " + reason
				blocked := *resp
				blocked.Content = genai.NewContentFromText(message, genai.RoleModel)
				blocked.ErrorCode = "OUTPUT_POLICY_BLOCKED"
				blocked.ErrorMessage = message
				return &blocked, nil
			}
			return nil, nil
		},
	})
}
