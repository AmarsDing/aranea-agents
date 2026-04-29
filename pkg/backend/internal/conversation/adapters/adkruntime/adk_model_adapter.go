package adkruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"strings"

	"arenea/backend/internal/domain"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

type providerModelLLM struct {
	adapter        *ADKRuntimeAdapter
	agent          domain.Agent
	providerModel  domain.PlatformResource
	runtimeContext *RuntimeContext
	onDelta        DeltaFunc
}

func newProviderModelLLM(adapter *ADKRuntimeAdapter, agent domain.Agent, providerModel domain.PlatformResource, runtimeContext *RuntimeContext, onDelta DeltaFunc) model.LLM {
	return &providerModelLLM{adapter: adapter, agent: agent, providerModel: providerModel, runtimeContext: runtimeContext, onDelta: onDelta}
}

func (m *providerModelLLM) Name() string {
	if strings.TrimSpace(m.providerModel.Model) != "" {
		return m.providerModel.Model
	}
	if strings.TrimSpace(m.providerModel.Name) != "" {
		return m.providerModel.Name
	}
	return "aranea-provider-model"
}

func (m *providerModelLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		providerModel := m.providerModel
		if req != nil && strings.TrimSpace(req.Model) != "" {
			providerModel.Model = strings.TrimSpace(req.Model)
		}
		generateReq := GenerateRequest{
			Agent:            m.agent,
			ProviderModel:    providerModel,
			Messages:         llmRequestMessages(req),
			Input:            latestUserInput(req),
			ToolDeclarations: llmRequestToolDeclarations(req),
			RuntimeContext:   m.runtimeContext,
		}
		if strings.TrimSpace(generateReq.Input) == "" {
			generateReq.Input = "Handle the request as specified."
		}

		var result GenerateResult
		var err error
		anthropicWithInitialTools := false
		if cfg, cfgErr := parseProviderConfig(providerModel.ConfigJSON); cfgErr == nil {
			anthropicWithInitialTools = isAnthropicProvider(cfg.ProviderType) && len(generateReq.ToolDeclarations) > 0 && !llmRequestHasFunctionResponse(req)
		}
		if stream && !anthropicWithInitialTools {
			result, err = m.adapter.streamDirect(ctx, generateReq, m.onDelta)
		} else {
			result, err = m.adapter.generateDirect(ctx, generateReq)
		}
		if err != nil {
			yield(nil, err)
			return
		}
		yield(generateResultToLLMResponse(result), nil)
	}
}

func llmRequestHasFunctionResponse(req *model.LLMRequest) bool {
	if req == nil {
		return false
	}
	for _, content := range req.Contents {
		if content == nil {
			continue
		}
		for _, part := range content.Parts {
			if part != nil && part.FunctionResponse != nil {
				return true
			}
		}
	}
	return false
}

func llmRequestMessages(req *model.LLMRequest) []ChatMessage {
	if req == nil {
		return nil
	}
	messages := make([]ChatMessage, 0, len(req.Contents)+1)
	if req.Config != nil && req.Config.SystemInstruction != nil {
		if text := contentText(req.Config.SystemInstruction); text != "" {
			messages = append(messages, ChatMessage{Role: "system", Content: text})
		}
	}
	for _, content := range req.Contents {
		role := "user"
		if content != nil && content.Role != "" {
			role = content.Role
		}
		if text := contentText(content); text != "" {
			messages = append(messages, ChatMessage{Role: role, Content: text})
		}
	}
	return messages
}

func llmRequestToolDeclarations(req *model.LLMRequest) []*genai.FunctionDeclaration {
	if req == nil || req.Config == nil {
		return nil
	}
	var out []*genai.FunctionDeclaration
	for _, item := range req.Config.Tools {
		if item == nil {
			continue
		}
		out = append(out, item.FunctionDeclarations...)
	}
	return out
}

func latestUserInput(req *model.LLMRequest) string {
	if req == nil {
		return ""
	}
	for i := len(req.Contents) - 1; i >= 0; i-- {
		content := req.Contents[i]
		if content == nil {
			continue
		}
		if content.Role == "" || content.Role == genai.RoleUser || content.Role == "user" {
			if text := contentText(content); text != "" {
				return text
			}
		}
	}
	if len(req.Contents) > 0 {
		return contentText(req.Contents[len(req.Contents)-1])
	}
	return ""
}

func contentText(content *genai.Content) string {
	if content == nil {
		return ""
	}
	parts := make([]string, 0, len(content.Parts))
	for _, part := range content.Parts {
		if part == nil {
			continue
		}
		if strings.TrimSpace(part.Text) != "" {
			parts = append(parts, part.Text)
			continue
		}
		if part.FunctionResponse != nil {
			raw, _ := json.Marshal(part.FunctionResponse.Response)
			parts = append(parts, fmt.Sprintf("Tool result from %s: %s", part.FunctionResponse.Name, string(raw)))
			continue
		}
		if part.FunctionCall != nil {
			raw, _ := json.Marshal(part.FunctionCall.Args)
			parts = append(parts, fmt.Sprintf("Tool call %s: %s", part.FunctionCall.Name, string(raw)))
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func generateResultToLLMResponse(result GenerateResult) *model.LLMResponse {
	if len(result.FunctionCalls) > 0 {
		parts := make([]*genai.Part, 0, len(result.FunctionCalls))
		for _, call := range result.FunctionCalls {
			if call != nil {
				parts = append(parts, genai.NewPartFromFunctionCall(call.Name, call.Args))
				parts[len(parts)-1].FunctionCall.ID = call.ID
			}
		}
		return &model.LLMResponse{
			Content:      &genai.Content{Role: genai.RoleModel, Parts: parts},
			ModelVersion: result.ModelName,
			TurnComplete: true,
		}
	}
	response := &model.LLMResponse{
		Content:      genai.NewContentFromText(result.Content, genai.RoleModel),
		ModelVersion: result.ModelName,
		TurnComplete: true,
	}
	if result.PromptTokens > 0 || result.CompletionTokens > 0 {
		response.CustomMetadata = map[string]any{
			"prompt_tokens":     result.PromptTokens,
			"completion_tokens": result.CompletionTokens,
			"latency_ms":        result.LatencyMS,
		}
	}
	return response
}

func llmResponseText(resp *model.LLMResponse) string {
	if resp == nil {
		return ""
	}
	if text := contentText(resp.Content); text != "" {
		return text
	}
	if resp.ErrorMessage != "" {
		return fmt.Sprintf("[%s] %s", resp.ErrorCode, resp.ErrorMessage)
	}
	return ""
}
