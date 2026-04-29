package adkruntime

import (
	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/plugin"
	"google.golang.org/genai"
)

func newSensitiveDataMaskPlugin() (*plugin.Plugin, error) {
	return plugin.New(plugin.Config{
		Name: "sensitive_data_mask",
		OnUserMessageCallback: func(_ agent.InvocationContext, content *genai.Content) (*genai.Content, error) {
			masked := redactContent(content)
			if masked == content {
				return nil, nil
			}
			return masked, nil
		},
		BeforeModelCallback: func(_ agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			if req == nil {
				return nil, nil
			}
			for i, content := range req.Contents {
				req.Contents[i] = redactContent(content)
			}
			if req.Config != nil && req.Config.SystemInstruction != nil {
				req.Config.SystemInstruction = redactContent(req.Config.SystemInstruction)
			}
			return nil, nil
		},
		AfterModelCallback: func(_ agent.CallbackContext, resp *model.LLMResponse, responseErr error) (*model.LLMResponse, error) {
			if resp == nil || responseErr != nil {
				return nil, nil
			}
			maskedContent := redactContent(resp.Content)
			if maskedContent == resp.Content {
				return nil, nil
			}
			masked := *resp
			masked.Content = maskedContent
			return &masked, nil
		},
	})
}
