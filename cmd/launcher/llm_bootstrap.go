package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// llmBootstrap carries the optional first-run LLM credential collected by the
// setup wizard. Applied to the seeded provider-model row via the admin API
// (never direct DB — api_key must be encrypted by the biz layer), then removed.
type llmBootstrap struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key"`
}

// providerModelItem mirrors the wire shape of GET /v1/llm-provider-models items.
type providerModelItem struct {
	ID         string `json:"id"`
	Key        string `json:"key"`
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	ConfigJSON string `json:"configJson"`
	Enabled    bool   `json:"enabled"`
	Status     string `json:"status"`
}

func llmBootstrapPath(root string) string {
	return filepath.Join(root, "configs", "llm-bootstrap.json")
}

func saveLLMBootstrap(root string, b llmBootstrap) error {
	if err := os.MkdirAll(filepath.Dir(llmBootstrapPath(root)), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(llmBootstrapPath(root), data, 0o600)
}

func loadLLMBootstrap(root string) *llmBootstrap {
	b, err := os.ReadFile(llmBootstrapPath(root))
	if err != nil {
		return nil
	}
	var lb llmBootstrap
	if json.Unmarshal(b, &lb) != nil || strings.TrimSpace(lb.APIKey) == "" || lb.Provider == "" || lb.Model == "" {
		return nil
	}
	return &lb
}

// mergeAPIKeyIntoConfig returns cfgJSON with api_key set, preserving siblings.
func mergeAPIKeyIntoConfig(cfgJSON, apiKey string) (string, error) {
	m := map[string]any{}
	if strings.TrimSpace(cfgJSON) != "" {
		if err := json.Unmarshal([]byte(cfgJSON), &m); err != nil {
			return "", fmt.Errorf("existing configJson invalid: %w", err)
		}
	}
	m["api_key"] = apiKey
	out, err := json.Marshal(m)
	return string(out), err
}

// pickProviderModel finds the exact provider+model row; no fuzzy fallback —
// writing a key to the wrong model is worse than not writing it.
func pickProviderModel(items []providerModelItem, provider, model string) *providerModelItem {
	for i := range items {
		if items[i].Provider == provider && items[i].Model == model {
			return &items[i]
		}
	}
	return nil
}

// applyLLMBootstrap pushes the wizard-collected API key into the seeded
// provider-model row: login (default admin) → locate row → PATCH configJson.
// On success the bootstrap file is removed; on any failure it is kept so the
// next start retries. Best-effort — callers log and continue.
func applyLLMBootstrap(root, baseURL string, log func(string, ...any)) error {
	lb := loadLLMBootstrap(root)
	if lb == nil {
		return nil
	}
	client := &http.Client{Timeout: 8 * time.Second}
	base := strings.TrimSuffix(baseURL, "/")

	token, err := adminLogin(client, base)
	if err != nil {
		return fmt.Errorf("admin login: %w", err)
	}
	item, err := findProviderModel(client, base, token, lb.Provider, lb.Model)
	if err != nil {
		return err
	}
	merged, err := mergeAPIKeyIntoConfig(item.ConfigJSON, lb.APIKey)
	if err != nil {
		return err
	}
	patch := map[string]any{
		"id": item.ID,
		"providerModel": map[string]any{
			"id":         item.ID,
			"key":        item.Key,
			"name":       item.Name,
			"provider":   item.Provider,
			"model":      item.Model,
			"enabled":    item.Enabled,
			"status":     item.Status,
			"configJson": merged,
		},
	}
	body, _ := json.Marshal(patch)
	req, err := http.NewRequest(http.MethodPatch, base+"/v1/llm-provider-models/"+item.ID, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("patch provider model: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("patch provider model: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if err := os.Remove(llmBootstrapPath(root)); err != nil {
		log("llm bootstrap applied but file removal failed: %v", err)
	}
	log("llm bootstrap applied: %s/%s api key configured", lb.Provider, lb.Model)
	return nil
}

func adminLogin(client *http.Client, base string) (string, error) {
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "changeme"})
	resp, err := client.Post(base+"/v1/admins/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d (default admin password changed? configure the key via Settings UI instead)", resp.StatusCode)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", fmt.Errorf("login response missing token")
	}
	return out.Token, nil
}

func findProviderModel(client *http.Client, base, token, provider, model string) (*providerModelItem, error) {
	req, err := http.NewRequest(http.MethodGet, base+"/v1/llm-provider-models?pageSize=100&search="+provider, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list provider models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list provider models: status %d", resp.StatusCode)
	}
	var out struct {
		Items []providerModelItem `json:"items"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, err
	}
	item := pickProviderModel(out.Items, provider, model)
	if item == nil {
		return nil, fmt.Errorf("seeded provider model %s/%s not found (catalog changed?)", provider, model)
	}
	return item, nil
}
