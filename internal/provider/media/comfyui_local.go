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

// ComfyUILocalProvider implements MediaProvider for a locally deployed ComfyUI.
// Communicates via ComfyUI HTTP API (POST /prompt + GET /history/{prompt_id}).
type ComfyUILocalProvider struct {
	cfg    ProviderConfig
	client *http.Client
}

// NewComfyUILocalProvider creates a new ComfyUILocalProvider.
func NewComfyUILocalProvider(cfg ProviderConfig) MediaProvider {
	return &ComfyUILocalProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: 5 * time.Minute, // media generation can be slow
		},
	}
}

func (p *ComfyUILocalProvider) Name() string { return "comfyui_local" }

// comfyuiPromptRequest is the request body for POST /prompt.
type comfyuiPromptRequest struct {
	Prompt map[string]any `json:"prompt"`
}

// comfyuiPromptResponse is the response from POST /prompt.
type comfyuiPromptResponse struct {
	PromptID string `json:"prompt_id"`
	Number   int    `json:"number"`
}

// comfyuiHistoryResponse is the response from GET /history/{prompt_id}.
// Key is the prompt_id, value contains outputs.
type comfyuiHistoryResponse map[string]struct {
	Outputs map[string]struct {
		Images []struct {
			Filename  string `json:"filename"`
			Subfolder string `json:"subfolder"`
			Type      string `json:"type"`
		} `json:"images"`
	} `json:"outputs"`
	Status struct {
		StatusStr string `json:"status_str"`
		Completed bool   `json:"completed"`
	} `json:"status"`
}

// buildTextToImageWorkflow creates a ComfyUI workflow for text-to-image.
// Uses a simple KSampler → VAE Decode → Save Image pipeline.
func buildTextToImageWorkflow(req ImageRequest) map[string]any {
	steps := 20
	cfgScale := 7.0
	if req.Seed == 0 {
		req.Seed = time.Now().UnixNano()
	}

	return map[string]any{
		"3": map[string]any{
			"class_type": "KSampler",
			"inputs": map[string]any{
				"seed":        req.Seed,
				"steps":       steps,
				"cfg":         cfgScale,
				"sampler_name": "euler",
				"scheduler":   "normal",
				"denoise":     1.0,
				"model":       []any{"4", 0},
				"positive":    []any{"6", 0},
				"negative":    []any{"7", 0},
				"latent_image": []any{"5", 0},
			},
		},
		"4": map[string]any{
			"class_type": "CheckpointLoaderSimple",
			"inputs": map[string]any{
				"ckpt_name": "v1-5-pruned-emaonly.safetensors",
			},
		},
		"5": map[string]any{
			"class_type": "EmptyLatentImage",
			"inputs": map[string]any{
				"width":     1024,
				"height":    1024,
				"batch_size": req.Count,
			},
		},
		"6": map[string]any{
			"class_type": "CLIPTextEncode",
			"inputs": map[string]any{
				"text": req.Prompt,
				"clip": []any{"4", 1},
			},
		},
		"7": map[string]any{
			"class_type": "CLIPTextEncode",
			"inputs": map[string]any{
				"text": "",
				"clip": []any{"4", 1},
			},
		},
		"8": map[string]any{
			"class_type": "VAEDecode",
			"inputs": map[string]any{
				"samples": []any{"3", 0},
				"vae":     []any{"4", 2},
			},
		},
		"9": map[string]any{
			"class_type": "SaveImage",
			"inputs": map[string]any{
				"filename_prefix": "aranea",
				"images":          []any{"8", 0},
			},
		},
	}
}

func (p *ComfyUILocalProvider) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if req.Count <= 0 {
		req.Count = 1
	}
	if req.Count > 4 {
		req.Count = 4
	}

	workflow := buildTextToImageWorkflow(req)

	// Submit prompt
	promptID, err := p.submitPrompt(ctx, workflow)
	if err != nil {
		return nil, fmt.Errorf("submit prompt: %w", err)
	}

	// Poll for completion
	artifacts, err := p.pollHistory(ctx, promptID)
	if err != nil {
		return nil, fmt.Errorf("poll history: %w", err)
	}

	return &ImageResult{Artifacts: artifacts}, nil
}

func (p *ComfyUILocalProvider) GenerateVideo(ctx context.Context, req VideoRequest) (*VideoResult, error) {
	return nil, fmt.Errorf("comfyui_local GenerateVideo not yet implemented")
}

func (p *ComfyUILocalProvider) ImageToVideo(ctx context.Context, req ImageToVideoRequest) (*VideoResult, error) {
	return nil, fmt.Errorf("comfyui_local ImageToVideo not yet implemented")
}

// submitPrompt posts a workflow to ComfyUI and returns the prompt_id.
func (p *ComfyUILocalProvider) submitPrompt(ctx context.Context, workflow map[string]any) (string, error) {
	body, err := json.Marshal(comfyuiPromptRequest{Prompt: workflow})
	if err != nil {
		return "", fmt.Errorf("marshal prompt: %w", err)
	}

	url := fmt.Sprintf("%s/prompt", p.cfg.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("comfyui returned %d: %s", resp.StatusCode, string(b))
	}

	var result comfyuiPromptResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return result.PromptID, nil
}

// pollHistory polls GET /history/{prompt_id} until completion or timeout.
func (p *ComfyUILocalProvider) pollHistory(ctx context.Context, promptID string) ([]MediaArtifact, error) {
	url := fmt.Sprintf("%s/history/%s", p.cfg.BaseURL, promptID)
	ticker := time.NewTicker(2 * time.Second)
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

			resp, err := p.client.Do(req)
			if err != nil {
				continue // retry on transient errors
			}

			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				continue
			}

			var history comfyuiHistoryResponse
			if err := json.NewDecoder(resp.Body).Decode(&history); err != nil {
				resp.Body.Close()
				continue
			}
			resp.Body.Close()

			// Check if our prompt is done
			entry, ok := history[promptID]
			if !ok {
				continue
			}

			if !entry.Status.Completed {
				continue
			}

			// Extract artifacts from outputs
			var artifacts []MediaArtifact
			for _, output := range entry.Outputs {
				for _, img := range output.Images {
					artifact := MediaArtifact{
						ArtifactID: fmt.Sprintf("comfyui_%s", img.Filename),
						URL:        fmt.Sprintf("%s/view?filename=%s&subfolder=%s&type=%s", p.cfg.BaseURL, img.Filename, img.Subfolder, img.Type),
						MimeType:   "image/png",
					}
					artifacts = append(artifacts, artifact)
				}
			}

			return artifacts, nil
		}
	}
}

func init() {
	Register("comfyui_local", func(cfg ProviderConfig) (MediaProvider, error) {
		return NewComfyUILocalProvider(cfg), nil
	})
}
