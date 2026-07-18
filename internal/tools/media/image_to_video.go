package media

import (
	"context"
	"fmt"

	"aranea-agents/internal/provider/media"

	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ImageToVideoInput is the input for the image_to_video tool.
type ImageToVideoInput struct {
	ImageArtifactID string `json:"image_artifact_id" jsonschema:"description=输入图片 Artifact ID,required"`
	Prompt          string `json:"prompt,omitempty" jsonschema:"description=运动描述"`
	DurationMs      int64  `json:"duration_ms,omitempty"`
	FPS             int    `json:"fps,omitempty"`
}

// ImageToVideoOutput is the output for the image_to_video tool.
type ImageToVideoOutput struct {
	Artifacts []media.MediaArtifact `json:"artifacts"`
}

// NewImageToVideoTool creates the image_to_video tool.
func NewImageToVideoTool(mp media.MediaProvider) trpctool.Tool {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, in ImageToVideoInput) (ImageToVideoOutput, error) {
			return executeImageToVideo(ctx, mp, in)
		},
		trpcfunction.WithName("image_to_video"),
		trpcfunction.WithDescription("将输入图片（通过 Artifact ID 引用）转换为视频，可提供运动描述、时长和帧率。"),
	)
}

func executeImageToVideo(ctx context.Context, mp media.MediaProvider, in ImageToVideoInput) (ImageToVideoOutput, error) {
	if in.ImageArtifactID == "" {
		return ImageToVideoOutput{}, fmt.Errorf("image_artifact_id is required")
	}
	if mp == nil {
		return ImageToVideoOutput{}, fmt.Errorf("media provider not configured")
	}
	if in.DurationMs <= 0 {
		in.DurationMs = 5000
	}
	if in.FPS <= 0 {
		in.FPS = 24
	}

	result, err := mp.ImageToVideo(ctx, media.ImageToVideoRequest{
		ImageArtifactID: in.ImageArtifactID,
		Prompt:          in.Prompt,
		DurationMs:      in.DurationMs,
		FPS:             in.FPS,
	})
	if err != nil {
		return ImageToVideoOutput{}, fmt.Errorf("image to video: %w", err)
	}
	return ImageToVideoOutput{Artifacts: result.Artifacts}, nil
}
