package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// QwenProvider implements MediaProvider for Alibaba Tongyi Wanxiang (通义万相).
// Supports text-to-image and text-to-video via DashScope API.
type QwenProvider struct {
	cfg    ProviderConfig
	client *http.Client
}

// NewQwenProvider creates a new QwenProvider.
func NewQwenProvider(cfg ProviderConfig) MediaProvider {
	return &QwenProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (p *QwenProvider) Name() string { return "qwen" }

// ── DashScope API types ──

type dashscopeSubmitRequest struct {
	Model string `json:"model"`
	Input struct {
		Prompt string `json:"prompt"`
	} `json:"input"`
	Parameters struct {
		Size  string `json:"size,omitempty"`
		N     int    `json:"n,omitempty"`
		Seed  int64  `json:"seed,omitempty"`
		Style string `json:"style,omitempty"`
	} `json:"parameters"`
}

type dashscopeSubmitResponse struct {
	Output struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
	} `json:"output"`
	RequestID string `json:"request_id"`
}

type dashscopeTaskResponse struct {
	Output struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		Results    []struct {
			URL string `json:"url"`
		} `json:"results"`
	} `json:"output"`
	RequestID string `json:"request_id"`
}

// ── Text-to-Image ──

func (p *QwenProvider) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if req.Count <= 0 {
		req.Count = 1
	}
	if req.Count > 4 {
		req.Count = 4
	}

	taskID, err := p.submitTextToImageTask(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("submit task: %w", err)
	}

	artifacts, err := p.pollTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("poll task: %w", err)
	}

	return &ImageResult{Artifacts: artifacts}, nil
}

func (p *QwenProvider) submitTextToImageTask(ctx context.Context, req ImageRequest) (string, error) {
	body := dashscopeSubmitRequest{
		Model: "wanx2.1-t2i-turbo",
	}
	body.Input.Prompt = req.Prompt
	body.Parameters.Size = req.Size
	if body.Parameters.Size == "" {
		body.Parameters.Size = "1024*1024"
	}
	body.Parameters.N = req.Count
	if req.Seed > 0 {
		body.Parameters.Seed = req.Seed
	}
	if req.Style != "" {
		body.Parameters.Style = req.Style
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/services/aigc/text2image/image-synthesis", p.cfg.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("X-DashScope-Async", "enable")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("dashscope returned %d: %s", resp.StatusCode, string(b))
	}

	var result dashscopeSubmitResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return result.Output.TaskID, nil
}

// ── Text-to-Video ──

func (p *QwenProvider) GenerateVideo(ctx context.Context, req VideoRequest) (*VideoResult, error) {
	return nil, fmt.Errorf("qwen GenerateVideo not yet implemented")
}

func (p *QwenProvider) ImageToVideo(ctx context.Context, req ImageToVideoRequest) (*VideoResult, error) {
	return nil, fmt.Errorf("qwen ImageToVideo not yet implemented")
}

// ── Task polling ──

func (p *QwenProvider) pollTask(ctx context.Context, taskID string) ([]MediaArtifact, error) {
	url := fmt.Sprintf("%s/tasks/%s", p.cfg.BaseURL, taskID)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	timeout := time.After(3 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, fmt.Errorf("polling timeout after 3 minutes")
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return nil, fmt.Errorf("create request: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

			resp, err := p.client.Do(req)
			if err != nil {
				continue
			}

			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				continue
			}

			var result dashscopeTaskResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				resp.Body.Close()
				continue
			}
			resp.Body.Close()

			switch result.Output.TaskStatus {
			case "SUCCEEDED":
				var artifacts []MediaArtifact
				for i, r := range result.Output.Results {
					artifacts = append(artifacts, MediaArtifact{
						ArtifactID: fmt.Sprintf("qwen_%s_%d", taskID, i),
						URL:        r.URL,
						MimeType:   "image/png",
					})
				}
				return artifacts, nil
			case "FAILED":
				return nil, fmt.Errorf("task failed")
			case "RUNNING", "PENDING":
				continue
			default:
				continue
			}
		}
	}
}

func init() {
	Register("qwen", func(cfg ProviderConfig) (MediaProvider, error) {
		return NewQwenProvider(cfg), nil
	})
}
