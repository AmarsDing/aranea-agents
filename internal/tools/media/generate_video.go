package media

import (
	"context"
	"fmt"

	"aranea-agents/internal/provider/media"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// GenerateVideoInput is the input for the generate_video tool.
type GenerateVideoInput struct {
	Prompt     string `json:"prompt" jsonschema:"description=视频描述,required"`
	DurationMs int64  `json:"duration_ms,omitempty" jsonschema:"description=时长 1000-30000"`
	FPS        int    `json:"fps,omitempty" jsonschema:"description=帧率 24-60"`
	Resolution string `json:"resolution,omitempty" jsonschema:"description=分辨率,enum=720p,enum=1080p"`
}

// GenerateVideoOutput is the output for the generate_video tool.
type GenerateVideoOutput struct {
	Artifacts []media.MediaArtifact `json:"artifacts"`
}

// NewGenerateVideoTool creates the generate_video tool.
func NewGenerateVideoTool(mp media.MediaProvider) trpctool.Tool {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, in GenerateVideoInput) (GenerateVideoOutput, error) {
			return executeGenerateVideo(ctx, mp, in)
		},
		trpcfunction.WithName("generate_video"),
		trpcfunction.WithDescription("根据文本描述生成视频。支持时长（1000-30000ms）、帧率（24-60）和分辨率（720p/1080p）。"),
	)
}

func executeGenerateVideo(ctx context.Context, mp media.MediaProvider, in GenerateVideoInput) (GenerateVideoOutput, error) {
	if in.Prompt == "" {
		return GenerateVideoOutput{}, fmt.Errorf("prompt is required")
	}
	if mp == nil {
		return GenerateVideoOutput{}, fmt.Errorf("media provider not configured")
	}
	if in.DurationMs <= 0 {
		in.DurationMs = 5000
	}
	if in.FPS <= 0 {
		in.FPS = 24
	}
	if in.Resolution == "" {
		in.Resolution = "720p"
	}

	result, err := mp.GenerateVideo(ctx, media.VideoRequest{
		Prompt:     in.Prompt,
		DurationMs: in.DurationMs,
		FPS:        in.FPS,
		Resolution: in.Resolution,
	})
	if err != nil {
		return GenerateVideoOutput{}, fmt.Errorf("generate video: %w", err)
	}
	return GenerateVideoOutput{Artifacts: result.Artifacts}, nil
}
