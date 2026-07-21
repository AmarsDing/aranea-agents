// Package media defines the MediaProvider interface for media generation
// (text-to-image, text-to-video, image-to-video). It is independent of the
// LLM Provider system (internal/provider/) because media generation is an
// asynchronous long-running task (submit → poll → fetch result), which is
// fundamentally different from LLM sync/stream text generation.
package media

import "context"

// MediaProvider generates media content (images, videos).
type MediaProvider interface {
	Name() string
	GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error)
	GenerateVideo(ctx context.Context, req VideoRequest) (*VideoResult, error)
	ImageToVideo(ctx context.Context, req ImageToVideoRequest) (*VideoResult, error)
}

// ImageRequest describes a text-to-image generation request.
type ImageRequest struct {
	Prompt string
	Size   string // "1024x1024" / "1792x1024" / "1024x1792"
	Style  string // "realistic" / "anime" / "oil_painting" / ""
	Count  int    // 1-4
	Seed   int64  // 0 = random
}

// ImageResult contains generated image artifacts.
type ImageResult struct {
	Artifacts []MediaArtifact
}

// VideoRequest describes a text-to-video generation request.
type VideoRequest struct {
	Prompt     string
	DurationMs int64
	FPS        int
	Resolution string // "720p" / "1080p"
	Seed       int64
}

// ImageToVideoRequest describes an image-to-video generation request.
type ImageToVideoRequest struct {
	ImageArtifactID string // input image Artifact ID
	Prompt          string
	DurationMs      int64
	FPS             int
}

// VideoResult contains generated video artifacts.
type VideoResult struct {
	Artifacts []MediaArtifact
}

// MediaArtifact represents a single generated media file.
type MediaArtifact struct {
	ArtifactID string `json:"artifact_id"`
	URL        string `json:"url"`
	MimeType   string `json:"mime_type"` // "image/png" / "video/mp4"
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Thumbnail  string `json:"thumbnail,omitempty"` // video poster
}
