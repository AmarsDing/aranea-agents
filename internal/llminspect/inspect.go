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
	"strconv"
	"strings"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/pkg/strutil"
)

type Input struct {
	ResourceID   string
	ProviderCode string
	ProviderType string
	ModelAPIID   string
	APIBaseURL   string
	APIKey       string
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
	if in.ProviderCode == "" || in.ModelAPIID == "" {
		return Result{}, kerrors.BadRequest("LLM_INSPECT", "provider_code and model_api_id are required")
	}
	if strings.Contains(strings.ToLower(in.APIBaseURL), "openrouter.ai") || strings.Contains(strings.ToLower(in.ProviderCode), "openrouter") {
		return inspectOpenRouterModel(in)
	}
	if deepSeekOpenAICompatBase(in.APIBaseURL) {
		return inspectOpenAICompatibleModel(in)
	}
	if strings.Contains(strings.ToLower(in.ProviderType), "anthropic") {
		return inspectAnthropicModel(in)
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
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextLength int    `json:"context_length"`
			Pricing       struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
			TopProvider struct {
				MaxCompletionTokens int `json:"max_completion_tokens"`
			} `json:"top_provider"`
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
			OK:                       true,
			Message:                  "已从 OpenRouter 获取模型参数",
			ProviderCode:             in.ProviderCode,
			ProviderType:             strutil.FirstNonEmpty(in.ProviderType, "openai"),
			ModelAPIID:               item.ID,
			ModelDisplayName:         strutil.FirstNonEmpty(item.Name, item.ID),
			ModelSizeLabel:           inferModelSizeLabel(item.ID + " " + item.Name),
			ContextWindowK:           tokensToK(item.ContextLength),
			MaxOutputTokens:          item.TopProvider.MaxCompletionTokens,
			InputPriceMicroUSDPer1K:  priceStringToMicroUSDPer1K(item.Pricing.Prompt),
			OutputPriceMicroUSDPer1K: priceStringToMicroUSDPer1K(item.Pricing.Completion),
			Source:                   "openrouter",
			RawMetadataJSON:          string(raw),
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
				Message:          "已验证模型存在；该 Provider 未返回上下文和价格",
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
			fallback := anthropicKnownModelDefaults(in)
			return Result{
				OK:                       true,
				Message:                  "已从 Anthropic 获取模型名称；价格和上下文需手动维护",
				ProviderCode:             in.ProviderCode,
				ProviderType:             strutil.FirstNonEmpty(in.ProviderType, "anthropic"),
				ModelAPIID:               item.ID,
				ModelDisplayName:         strutil.FirstNonEmpty(item.DisplayName, item.ID),
				ModelSizeLabel:           inferModelSizeLabel(item.ID + " " + item.DisplayName),
				ContextWindowK:           fallback.ContextWindowK,
				MaxOutputTokens:          fallback.MaxOutputTokens,
				InputPriceMicroUSDPer1K:  fallback.InputPriceMicroUSDPer1K,
				OutputPriceMicroUSDPer1K: fallback.OutputPriceMicroUSDPer1K,
				Source:                   "anthropic",
				RawMetadataJSON:          string(raw),
			}, nil
		}
	}
	return anthropicKnownModelFallback(in, "Anthropic 元数据接口未返回该模型，已根据模型ID使用内置参数"), nil
}

func anthropicKnownModelFallback(in Input, message string) Result {
	result := anthropicKnownModelDefaults(in)
	result.OK = true
	result.Message = message
	result.RawMetadataJSON = fmt.Sprintf(`{"source":"anthropic-known-defaults","model":"%s"}`, in.ModelAPIID)
	return result
}

func anthropicKnownModelDefaults(in Input) Result {
	model := strings.ToLower(in.ModelAPIID)
	result := Result{
		ProviderCode:     in.ProviderCode,
		ProviderType:     strutil.FirstNonEmpty(in.ProviderType, "anthropic"),
		ModelAPIID:       in.ModelAPIID,
		ModelDisplayName: in.ModelAPIID,
		ContextWindowK:   200,
		MaxOutputTokens:  8192,
		Source:           "anthropic-known-defaults",
	}
	switch {
	case strings.Contains(model, "opus"):
		result.InputPriceMicroUSDPer1K = 15000
		result.OutputPriceMicroUSDPer1K = 75000
	case strings.Contains(model, "haiku"):
		result.InputPriceMicroUSDPer1K = 800
		result.OutputPriceMicroUSDPer1K = 4000
	default:
		result.InputPriceMicroUSDPer1K = 3000
		result.OutputPriceMicroUSDPer1K = 15000
	}
	return result
}

func getProviderJSON(endpoint string, apiKey string, headers map[string]string, out any) error {
	client := &http.Client{Timeout: 15 * time.Second}
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

func priceStringToMicroUSDPer1K(value string) int64 {
	numberValue, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || numberValue <= 0 {
		return 0
	}
	return int64(numberValue*1_000_000_000 + 0.5)
}

func tokensToK(tokens int) int {
	if tokens <= 0 {
		return 0
	}
	return int(float64(tokens)/1000 + 0.5)
}

func inferModelSizeLabel(value string) string {
	re := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?\s*x\s*)?\d+(?:\.\d+)?\s*b`)
	match := re.FindString(value)
	if match == "" {
		return ""
	}
	return strings.ToUpper(strings.ReplaceAll(match, " ", ""))
}
