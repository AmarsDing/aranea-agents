package media

import (
	"context"
	"fmt"
)

// ComfyUILocalProvider implements MediaProvider for a locally deployed ComfyUI.
// Communicates via ComfyUI HTTP API (POST /prompt + GET /history/{prompt_id})
// and WebSocket (ws://<host>/ws) for execution progress.
type ComfyUILocalProvider struct {
	cfg ProviderConfig
}

// NewComfyUILocalProvider creates a new ComfyUILocalProvider.
func NewComfyUILocalProvider(cfg ProviderConfig) MediaProvider {
	return &ComfyUILocalProvider{cfg: cfg}
}

func (p *ComfyUILocalProvider) Name() string { return "comfyui_local" }

func (p *ComfyUILocalProvider) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	// TODO: Build ComfyUI workflow JSON from ImageRequest
	// POST /prompt → poll GET /history/{prompt_id}
	return nil, fmt.Errorf("comfyui_local GenerateImage not yet implemented")
}

func (p *ComfyUILocalProvider) GenerateVideo(ctx context.Context, req VideoRequest) (*VideoResult, error) {
	return nil, fmt.Errorf("comfyui_local GenerateVideo not yet implemented")
}

func (p *ComfyUILocalProvider) ImageToVideo(ctx context.Context, req ImageToVideoRequest) (*VideoResult, error) {
	return nil, fmt.Errorf("comfyui_local ImageToVideo not yet implemented")
}

func init() {
	Register("comfyui_local", func(cfg ProviderConfig) (MediaProvider, error) {
		return NewComfyUILocalProvider(cfg), nil
	})
}
