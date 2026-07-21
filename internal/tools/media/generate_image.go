package media

import (
	"context"
	"fmt"

	"aranea-agents/internal/provider/media"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// GenerateImageInput is the input for the generate_image tool.
type GenerateImageInput struct {
	Prompt string `json:"prompt" jsonschema:"description=图像描述,required"`
	Size   string `json:"size,omitempty" jsonschema:"description=尺寸,enum=1024x1024,enum=1792x1024,enum=1024x1792"`
	Style  string `json:"style,omitempty" jsonschema:"description=风格,enum=realistic,enum=anime,enum=oil_painting"`
	Count  int    `json:"count,omitempty" jsonschema:"description=生成数量 1-4"`
}

// GenerateImageOutput is the output for the generate_image tool.
type GenerateImageOutput struct {
	Artifacts []media.MediaArtifact `json:"artifacts"`
}

// NewGenerateImageTool creates the generate_image tool.
func NewGenerateImageTool(mp media.MediaProvider) trpctool.Tool {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, in GenerateImageInput) (GenerateImageOutput, error) {
			return executeGenerateImage(ctx, mp, in)
		},
		trpcfunction.WithName("generate_image"),
		trpcfunction.WithDescription("根据文本描述生成图片。支持尺寸（1024x1024/1792x1024/1024x1792）、风格（realistic/anime/oil_painting）和数量（1-4）。"),
	)
}

func executeGenerateImage(ctx context.Context, mp media.MediaProvider, in GenerateImageInput) (GenerateImageOutput, error) {
	if in.Prompt == "" {
		return GenerateImageOutput{}, fmt.Errorf("prompt is required")
	}
	if mp == nil {
		return GenerateImageOutput{}, fmt.Errorf("media provider not configured")
	}
	if in.Count <= 0 {
		in.Count = 1
	}
	if in.Count > 4 {
		in.Count = 4
	}
	if in.Size == "" {
		in.Size = "1024x1024"
	}

	result, err := mp.GenerateImage(ctx, media.ImageRequest{
		Prompt: in.Prompt,
		Size:   in.Size,
		Style:  in.Style,
		Count:  in.Count,
	})
	if err != nil {
		return GenerateImageOutput{}, fmt.Errorf("generate image: %w", err)
	}
	return GenerateImageOutput{Artifacts: result.Artifacts}, nil
}
