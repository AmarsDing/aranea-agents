// Package llminspect probes remote LLM provider metadata (OpenRouter, OpenAI-compatible, Anthropic).
// Lives outside internal/provider so biz can use it without a biz↔provider import cycle.
package llminspect

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/pkg/outboundguard"
	"aranea-agents/pkg/strutil"
)

type Input struct {
	ResourceID   string
	ProviderCode string
	ProviderType string
	ModelAPIID   string
	APIBaseURL   string
	APIKey       string
	Variant      string
	SecretID     string
	SecretKey    string
	AWSRegion    string
}

type Result struct {
	OK                            bool
	Message                       string
	ProviderCode                  string
	ProviderType                  string
	ModelAPIID                    string
	ModelDisplayName              string
	ModelSizeLabel                string
	ContextWindowK                int
	MaxOutputTokens               int
	InputPriceMicroUSDPer1K       int64
	OutputPriceMicroUSDPer1K      int64
	CachedInputPriceMicroUSDPer1K int64
	ReasoningPriceMicroUSDPer1K   int64
	EmbeddingPriceMicroUSDPer1K   int64
	Source                        string
	RawMetadataJSON               string
	Variant                       string
	EnableTokenTailoring          bool
	SupportsCache                 bool
	SupportsThinking              bool
}

func deepSeekOpenAICompatBase(apiBase string) bool {
	u := strings.TrimSpace(strings.ToLower(apiBase))
	if u == "" || !strings.Contains(u, "api.deepseek.com") {
		return false
	}
	u = strings.TrimRight(u, "/")
	return !strings.HasSuffix(u, "/anthropic")
}

func Run(in Input) (Result, error) {
	in.ResourceID = strings.TrimSpace(in.ResourceID)
	in.ProviderCode = strings.TrimSpace(in.ProviderCode)
	in.ProviderType = strings.TrimSpace(in.ProviderType)
	in.ModelAPIID = strings.TrimSpace(in.ModelAPIID)
	in.APIBaseURL = strings.TrimSpace(in.APIBaseURL)
	in.APIKey = strings.TrimSpace(in.APIKey)
	in.Variant = strings.TrimSpace(in.Variant)
	in.SecretID = strings.TrimSpace(in.SecretID)
	in.SecretKey = strings.TrimSpace(in.SecretKey)
	in.AWSRegion = strings.TrimSpace(in.AWSRegion)
	if in.ProviderCode == "" || in.ModelAPIID == "" {
		return Result{}, kerrors.BadRequest("LLM_INSPECT", "provider_code and model_api_id are required")
	}
	if strings.Contains(strings.ToLower(in.APIBaseURL), "openrouter.ai") || strings.Contains(strings.ToLower(in.ProviderCode), "openrouter") {
		return inspectOpenRouterModel(in)
	}
	if deepSeekOpenAICompatBase(in.APIBaseURL) {
		return inspectOpenAICompatibleModel(in)
	}
	pt := strings.ToLower(in.ProviderType)
	switch pt {
	case "anthropic":
		return inspectAnthropicModel(in)
	case "gemini":
		return inspectGeminiModel(in)
	case "ollama":
		return inspectOllamaModel(in)
	case "hunyuan":
		return inspectHunyuanModel(in)
	case "bedrock":
		return inspectBedrockModel(in)
	}
	return inspectOpenAICompatibleModel(in)
}

func inspectOpenRouterModel(in Input) (Result, error) {
	endpoint := "https://openrouter.ai/api/v1/models"
	if in.APIBaseURL != "" {
		endpoint = openRouterModelsURL(in.APIBaseURL)
	}
	var out struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := getProviderJSON(endpoint, in.APIKey, nil, &out); err != nil {
		return Result{OK: false, Message: "OpenRouter 模型参数请求失败：" + err.Error(), ProviderCode: in.ProviderCode, ProviderType: in.ProviderType, ModelAPIID: in.ModelAPIID}, nil
	}
		for _, item := range out.Data {
		if item.ID != in.ModelAPIID {
			continue
		}
		raw, _ := json.Marshal(item)
		return Result{
			OK:               true,
			Message:          "已验证 OpenRouter 模型存在",
			ProviderCode:     in.ProviderCode,
			ProviderType:     strutil.FirstNonEmpty(in.ProviderType, "openai"),
			ModelAPIID:       item.ID,
			ModelDisplayName: strutil.FirstNonEmpty(item.Name, item.ID),
			ModelSizeLabel:   inferModelSizeLabel(item.ID + " " + item.Name),
			Source:           "openrouter",
			RawMetadataJSON:  string(raw),
		}, nil
	}
	return Result{OK: false, Message: "OpenRouter 未找到该模型", ProviderCode: in.ProviderCode, ProviderType: in.ProviderType, ModelAPIID: in.ModelAPIID}, nil
}

func inspectOpenAICompatibleModel(in Input) (Result, error) {
	if in.APIBaseURL == "" {
		return Result{OK: false, Message: "检查模型需要 API 基础 URL", ProviderCode: in.ProviderCode, ProviderType: in.ProviderType, ModelAPIID: in.ModelAPIID}, nil
	}
	var out struct {
		Data []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := getProviderJSON(modelsURL(in.APIBaseURL), in.APIKey, nil, &out); err != nil {
		return Result{OK: false, Message: "Provider 模型参数请求失败：" + err.Error(), ProviderCode: in.ProviderCode, ProviderType: in.ProviderType, ModelAPIID: in.ModelAPIID}, nil
	}
	for _, item := range out.Data {
		if item.ID == in.ModelAPIID {
			raw, _ := json.Marshal(item)
			return Result{
				OK:               true,
				Message:          "已验证 Provider 连通性及模型存在",
				ProviderCode:     in.ProviderCode,
				ProviderType:     strutil.FirstNonEmpty(in.ProviderType, "openai"),
				ModelAPIID:       item.ID,
				ModelDisplayName: item.ID,
				ModelSizeLabel:   inferModelSizeLabel(item.ID),
				Source:           "openai-compatible",
				RawMetadataJSON:  string(raw),
			}, nil
		}
	}
	return Result{OK: false, Message: "Provider /models 未找到该模型", ProviderCode: in.ProviderCode, ProviderType: in.ProviderType, ModelAPIID: in.ModelAPIID}, nil
}

func inspectAnthropicModel(in Input) (Result, error) {
	base := strutil.FirstNonEmpty(in.APIBaseURL, "https://api.anthropic.com/v1")
	var out struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	headers := map[string]string{"anthropic-version": "2023-06-01"}
	if err := getProviderJSON(modelsURL(base), in.APIKey, headers, &out); err != nil {
		return anthropicKnownModelFallback(in, "Anthropic 元数据接口不可用，已根据模型ID使用内置参数："+err.Error()), nil
	}
		for _, item := range out.Data {
		if item.ID == in.ModelAPIID {
			raw, _ := json.Marshal(item)
			return Result{
				OK:               true,
				Message:          "已验证 Anthropic 模型存在",
				ProviderCode:     in.ProviderCode,
				ProviderType:     strutil.FirstNonEmpty(in.ProviderType, "anthropic"),
				ModelAPIID:       item.ID,
				ModelDisplayName: strutil.FirstNonEmpty(item.DisplayName, item.ID),
				ModelSizeLabel:   inferModelSizeLabel(item.ID + " " + item.DisplayName),
				Source:           "anthropic",
				RawMetadataJSON:  string(raw),
			}, nil
		}
	}
	return anthropicKnownModelFallback(in, "Anthropic 元数据接口未返回该模型，已登记模型 ID"), nil
}

func anthropicKnownModelFallback(in Input, message string) Result {
	return Result{
		OK:               true,
		Message:          message,
		ProviderCode:     in.ProviderCode,
		ProviderType:     strutil.FirstNonEmpty(in.ProviderType, "anthropic"),
		ModelAPIID:       in.ModelAPIID,
		ModelDisplayName: in.ModelAPIID,
		Source:           "anthropic-known-defaults",
		RawMetadataJSON:  fmt.Sprintf(`{"source":"anthropic-known-defaults","model":"%s"}`, in.ModelAPIID),
	}
}

func inspectGeminiModel(in Input) (Result, error) {
	base := strutil.FirstNonEmpty(in.APIBaseURL, "https://generativelanguage.googleapis.com/v1beta")
	endpoint := strings.TrimRight(base, "/") + "/models"
	if in.APIKey != "" {
		endpoint += "?key=" + url.QueryEscape(in.APIKey)
	}
	var out struct {
		Models []struct {
			Name                       string `json:"name"`
			DisplayName                string `json:"displayName"`
			InputTokenLimit            int    `json:"inputTokenLimit"`
			OutputTokenLimit           int    `json:"outputTokenLimit"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := getProviderJSON(endpoint, "", nil, &out); err != nil {
		return Result{OK: false, Message: "Gemini 模型参数请求失败：" + err.Error(), ProviderCode: in.ProviderCode, ProviderType: "gemini", ModelAPIID: in.ModelAPIID}, nil
	}
	want := in.ModelAPIID
	if !strings.HasPrefix(want, "models/") {
		want = "models/" + want
	}
	for _, item := range out.Models {
		if item.Name != want && item.Name != in.ModelAPIID && !strings.HasSuffix(item.Name, "/"+in.ModelAPIID) {
			continue
		}
		raw, _ := json.Marshal(item)
		display := strutil.FirstNonEmpty(item.DisplayName, in.ModelAPIID)
		return Result{
			OK:                   true,
			Message:              "已验证 Gemini 模型存在",
			ProviderCode:         in.ProviderCode,
			ProviderType:         "gemini",
			ModelAPIID:           in.ModelAPIID,
			ModelDisplayName:     display,
			ModelSizeLabel:       inferModelSizeLabel(display),
			Source:               "gemini",
			RawMetadataJSON:      string(raw),
			EnableTokenTailoring: true,
			SupportsCache:        true,
		}, nil
	}
	return Result{OK: false, Message: "Gemini 未找到该模型", ProviderCode: in.ProviderCode, ProviderType: "gemini", ModelAPIID: in.ModelAPIID}, nil
}

func inspectOllamaModel(in Input) (Result, error) {
	base := strutil.FirstNonEmpty(in.APIBaseURL, "http://127.0.0.1:11434")
	endpoint := strings.TrimRight(base, "/") + "/api/tags"
	var out struct {
		Models []struct {
			Name       string `json:"name"`
			Model      string `json:"model"`
			Size       int64  `json:"size"`
			Details    struct {
				ParameterSize string `json:"parameter_size"`
			} `json:"details"`
		} `json:"models"`
	}
	if err := getProviderJSON(endpoint, in.APIKey, nil, &out); err != nil {
		return Result{OK: false, Message: "Ollama 模型参数请求失败：" + err.Error(), ProviderCode: in.ProviderCode, ProviderType: "ollama", ModelAPIID: in.ModelAPIID}, nil
	}
	for _, item := range out.Models {
		name := strutil.FirstNonEmpty(item.Name, item.Model)
		if name != in.ModelAPIID && !strings.HasPrefix(name, in.ModelAPIID+":") {
			continue
		}
		raw, _ := json.Marshal(item)
		sizeLabel := item.Details.ParameterSize
		if sizeLabel == "" {
			sizeLabel = inferModelSizeLabel(name)
		}
		return Result{
			OK:                   true,
			Message:              "已从 Ollama 获取本地模型",
			ProviderCode:         in.ProviderCode,
			ProviderType:         "ollama",
			ModelAPIID:           in.ModelAPIID,
			ModelDisplayName:     name,
			ModelSizeLabel:       sizeLabel,
			Source:               "ollama",
			RawMetadataJSON:      string(raw),
			EnableTokenTailoring: true,
		}, nil
	}
	return Result{OK: false, Message: "Ollama 未找到该模型", ProviderCode: in.ProviderCode, ProviderType: "ollama", ModelAPIID: in.ModelAPIID}, nil
}

func inspectHunyuanModel(in Input) (Result, error) {
	if in.SecretID == "" || in.SecretKey == "" {
		if in.APIKey == "" {
			return Result{OK: false, Message: "混元检查需要 SecretId 与 SecretKey", ProviderCode: in.ProviderCode, ProviderType: "hunyuan", ModelAPIID: in.ModelAPIID}, nil
		}
		result, err := inspectOpenAICompatibleModel(in)
		if err == nil && result.OK {
			result.ProviderType = "hunyuan"
			result.Source = "hunyuan"
		}
		return result, err
	}
	probe := in
	probe.ProviderType = "hunyuan"
	probe.APIBaseURL = strutil.FirstNonEmpty(probe.APIBaseURL, "https://api.hunyuan.cloud.tencent.com/v1")
	probe.APIKey = probe.SecretKey
	result, err := inspectOpenAICompatibleModel(probe)
	if err != nil {
		return result, err
	}
	if result.OK {
		result.ProviderType = "hunyuan"
		result.Source = "hunyuan"
		result.Message = "已验证混元模型存在"
		result.ProviderCode = in.ProviderCode
	}
	return result, nil
}

func inspectBedrockModel(in Input) (Result, error) {
	if in.AWSRegion == "" {
		return Result{OK: false, Message: "Bedrock 检查需要 AWS Region", ProviderCode: in.ProviderCode, ProviderType: "bedrock", ModelAPIID: in.ModelAPIID}, nil
	}
	raw, _ := json.Marshal(map[string]string{
		"model_id":   in.ModelAPIID,
		"aws_region": in.AWSRegion,
	})
	return Result{
		OK:               true,
		Message:          "Bedrock 模型 ID 已登记（运行时通过 AWS SDK 调用）",
		ProviderCode:     in.ProviderCode,
		ProviderType:     "bedrock",
		ModelAPIID:       in.ModelAPIID,
		ModelDisplayName: in.ModelAPIID,
		Source:           "bedrock",
		RawMetadataJSON:  string(raw),
		SupportsCache:    true,
	}, nil
}

func getProviderJSON(endpoint string, apiKey string, headers map[string]string, out any) error {
	if err := outboundguard.ValidateURL(endpoint); err != nil {
		return fmt.Errorf("provider inspect blocked: %w", err)
	}
	client := outboundguard.NewClient(15 * time.Second)
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
		request.Header.Set("x-api-key", apiKey)
	}
	request.Header.Set("Accept", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf("provider metadata request failed: %s %s", response.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(response.Body).Decode(out)
}

func openRouterModelsURL(base string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.Contains(trimmed, "/models") {
		return trimmed
	}
	if strings.Contains(trimmed, "/api/v1") {
		return strings.TrimRight(strings.Split(trimmed, "/api/v1")[0], "/") + "/api/v1/models"
	}
	return "https://openrouter.ai/api/v1/models"
}

func modelsURL(base string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(trimmed, "/chat/completions") {
		return strings.TrimSuffix(trimmed, "/chat/completions") + "/models"
	}
	if strings.HasSuffix(trimmed, "/messages") {
		return strings.TrimSuffix(trimmed, "/messages") + "/models"
	}
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return trimmed + "/models"
	}
	return trimmed
}

func inferModelSizeLabel(value string) string {
	re := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?\s*x\s*)?\d+(?:\.\d+)?\s*b`)
	match := re.FindString(value)
	if match == "" {
		return ""
	}
	return strings.ToUpper(strings.ReplaceAll(match, " ", ""))
}
