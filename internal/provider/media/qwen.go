package media

import (
	"context"
	"fmt"
)

// QwenProvider implements MediaProvider for Alibaba Tongyi Wanxiang (通义万相).
// Supports text-to-image and text-to-video via DashScope API.
type QwenProvider struct {
	cfg ProviderConfig
}

// NewQwenProvider creates a new QwenProvider.
func NewQwenProvider(cfg ProviderConfig) MediaProvider {
	return &QwenProvider{cfg: cfg}
}

func (p *QwenProvider) Name() string { return "qwen" }

func (p *QwenProvider) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	// TODO: Implement DashScope text-to-image API call
	// POST /services/aigc/text2image/image-synthesis
	// Async: submit → poll GET /tasks/{task_id} → fetch result
	return nil, fmt.Errorf("qwen GenerateImage not yet implemented")
}

func (p *QwenProvider) GenerateVideo(ctx context.Context, req VideoRequest) (*VideoResult, error) {
	// TODO: Implement DashScope video-generation API call
	// POST /services/aigc/video-generation/video-synthesis
	return nil, fmt.Errorf("qwen GenerateVideo not yet implemented")
}

func (p *QwenProvider) ImageToVideo(ctx context.Context, req ImageToVideoRequest) (*VideoResult, error) {
	return nil, fmt.Errorf("qwen ImageToVideo not yet implemented")
}

func init() {
	Register("qwen", func(cfg ProviderConfig) (MediaProvider, error) {
		return NewQwenProvider(cfg), nil
	})
}
