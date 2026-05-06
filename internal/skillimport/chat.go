package skillimport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/biz"
)

type chatModelCfg struct {
	ProviderType string
	APIBaseURL   string
	APIKey       string
	ModelAPIID   string
}

func providerModelHasCredentials(cfgJSON string) bool {
	var cfg struct {
		APIBaseURL string `json:"api_base_url"`
		APIKey     string `json:"api_key"`
	}
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		return false
	}
	return strings.TrimSpace(cfg.APIBaseURL) != "" && strings.TrimSpace(cfg.APIKey) != ""
}

func (e *Engine) resolveChatModel(ctx context.Context, provider, model string) (chatModelCfg, error) {
	if e.llm == nil {
		return chatModelCfg{}, ErrNoChatModelConfigured
	}
	rows, err := e.llm.List(ctx)
	if err != nil {
		return chatModelCfg{}, err
	}
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	for _, row := range rows {
		if row.DeletedAt != "" {
			continue
		}
		if !row.Enabled {
			continue
		}
		if provider != "" && row.Provider != provider {
			continue
		}
		if model != "" && row.Model != model {
			continue
		}
		if !providerModelHasCredentials(row.ConfigJSON) {
			continue
		}
		var cfg struct {
			ProviderType string `json:"provider_type"`
			APIBaseURL   string `json:"api_base_url"`
			APIKey       string `json:"api_key"`
		}
		_ = json.Unmarshal([]byte(row.ConfigJSON), &cfg)
		return chatModelCfg{
			ProviderType: cfg.ProviderType,
			APIBaseURL:   strings.TrimSpace(cfg.APIBaseURL),
			APIKey:       strings.TrimSpace(cfg.APIKey),
			ModelAPIID:   strings.TrimSpace(row.Model),
		}, nil
	}
	return chatModelCfg{}, ErrNoChatModelConfigured
}

func chatCompletionsEndpoint(apiBase string) string {
	u := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if strings.HasSuffix(u, "/chat/completions") {
		return u
	}
	// DeepSeek OpenAI 兼容：POST {base}/chat/completions（无 /v1 前缀）。见 https://api-docs.deepseek.com/zh-cn/
	if strings.Contains(strings.ToLower(u), "api.deepseek.com") {
		return u + "/chat/completions"
	}
	if strings.HasSuffix(u, "/v1") {
		return u + "/chat/completions"
	}
	return u + "/v1/chat/completions"
}

func anthropicMessagesEndpoint(apiBase string) string {
	u := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if strings.HasSuffix(u, "/messages") {
		return u
	}
	if strings.HasSuffix(u, "/v1") {
		return u + "/messages"
	}
	return u + "/v1/messages"
}

func completeChat(ctx context.Context, cfg chatModelCfg, prompt string) (string, error) {
	client := &http.Client{Timeout: 120 * time.Second}
	pt := strings.ToLower(cfg.ProviderType)
	if strings.Contains(pt, "anthropic") {
		return completeAnthropic(ctx, client, cfg, prompt)
	}
	return completeOpenAICompatible(ctx, client, cfg, prompt)
}

func completeOpenAICompatible(ctx context.Context, client *http.Client, cfg chatModelCfg, prompt string) (string, error) {
	body := map[string]any{
		"model": cfg.ModelAPIID,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionsEndpoint(cfg.APIBaseURL), bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("chat completion failed: %s %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("empty chat completion response")
	}
	return out.Choices[0].Message.Content, nil
}

func completeAnthropic(ctx context.Context, client *http.Client, cfg chatModelCfg, prompt string) (string, error) {
	body := map[string]any{
		"model":      cfg.ModelAPIID,
		"max_tokens": 4096,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicMessagesEndpoint(cfg.APIBaseURL), bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("anthropic messages failed: %s %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
	var out struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", err
	}
	if len(out.Content) == 0 || strings.TrimSpace(out.Content[0].Text) == "" {
		return "", fmt.Errorf("empty anthropic response")
	}
	return out.Content[0].Text, nil
}

func buildSimilarityPrompt(candidate candidateState, source biz.SkillSimilaritySource) string {
	return fmt.Sprintf(`你是 Skill 管理系统的相似度评估器。请比较候选 Skill 与已有 Skill，并只返回 JSON，不要返回 Markdown。
JSON 字段必须包含：
similarity_score,name_similarity,description_similarity,body_similarity,trigger_similarity,tool_similarity,conflict_risk,recommendation,confidence,reason,evidence。
所有相似度和 confidence 为 0 到 1。recommendation 只能是 keep_separate、suggest_refine、block_duplicate。

候选 Skill:
名称: %s
简介: %s
正文:
%s

已有 Skill:
名称: %s
简介: %s
正文:
%s`, candidate.public.Name, candidate.public.Description, truncateRunes(candidate.body, 5000), source.Name, source.Description, truncateRunes(source.Body, 5000))
}

func buildRefinePrompt(group biz.SkillConflictGroup, candidates []candidateState, instructions string) string {
	var b strings.Builder
	b.WriteString("你是 Skill 炼化器。请将下列相似功能 Skill 合并成一个更清晰、不重复的新 Skill，并只返回 JSON。JSON 字段: merged_name, merged_description, merged_body, merged_tags。merged_tags 为 [{\"name\":\"...\",\"source\":\"user\"}]。\n")
	if instructions != "" {
		b.WriteString("额外要求: " + instructions + "\n")
	}
	for _, candidate := range candidates {
		b.WriteString("\n候选 Skill:\n名称: " + candidate.public.Name + "\n简介: " + candidate.public.Description + "\n正文:\n" + truncateRunes(candidate.body, 5000) + "\n")
	}
	for _, existing := range group.ExistingSkills {
		b.WriteString("\n已有 Skill:\n名称: " + existing.Name + "\n简介: " + existing.Description + "\n正文:\n" + truncateRunes(existing.Body, 5000) + "\n")
	}
	return b.String()
}

func parseRefineResult(raw string) (biz.SkillRefineResult, error) {
	var out biz.SkillRefineResult
	if err := decodeModelJSON(raw, &out); err != nil {
		return out, err
	}
	if strings.TrimSpace(out.MergedName) == "" || strings.TrimSpace(out.MergedBody) == "" {
		return out, fmt.Errorf("model refine result missing merged_name or merged_body")
	}
	return out, nil
}
