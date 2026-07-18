package media

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/provider/media"
)

type mockMediaProvider struct {
	imageReq     media.ImageRequest
	imageResult  *media.ImageResult
	imageErr     error
	videoReq     media.VideoRequest
	videoResult  *media.VideoResult
	videoErr     error
	i2vReq       media.ImageToVideoRequest
	i2vResult    *media.VideoResult
	i2vErr       error
}

func (m *mockMediaProvider) Name() string { return "mock" }

func (m *mockMediaProvider) GenerateImage(_ context.Context, req media.ImageRequest) (*media.ImageResult, error) {
	m.imageReq = req
	return m.imageResult, m.imageErr
}

func (m *mockMediaProvider) GenerateVideo(_ context.Context, req media.VideoRequest) (*media.VideoResult, error) {
	m.videoReq = req
	return m.videoResult, m.videoErr
}

func (m *mockMediaProvider) ImageToVideo(_ context.Context, req media.ImageToVideoRequest) (*media.VideoResult, error) {
	m.i2vReq = req
	return m.i2vResult, m.i2vErr
}

func TestExecuteGenerateImage_RequiresPrompt(t *testing.T) {
	mp := &mockMediaProvider{}
	if _, err := executeGenerateImage(context.Background(), mp, GenerateImageInput{}); err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

func TestExecuteGenerateImage_NilProvider(t *testing.T) {
	if _, err := executeGenerateImage(context.Background(), nil, GenerateImageInput{Prompt: "a cat"}); err == nil {
		t.Fatal("expected error for nil provider")
	}
}

func TestExecuteGenerateImage_DefaultsAndClamping(t *testing.T) {
	mp := &mockMediaProvider{
		imageResult: &media.ImageResult{
			Artifacts: []media.MediaArtifact{{ArtifactID: "a1", URL: "http://x/1.png", MimeType: "image/png"}},
		},
	}
	out, err := executeGenerateImage(context.Background(), mp, GenerateImageInput{Prompt: "a cat", Count: 99})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.imageReq.Size != "1024x1024" {
		t.Errorf("expected default size 1024x1024, got %q", mp.imageReq.Size)
	}
	if mp.imageReq.Count != 4 {
		t.Errorf("expected count clamped to 4, got %d", mp.imageReq.Count)
	}
	if len(out.Artifacts) != 1 || out.Artifacts[0].ArtifactID != "a1" {
		t.Errorf("unexpected artifacts: %+v", out.Artifacts)
	}
}

func TestExecuteGenerateImage_ProviderError(t *testing.T) {
	mp := &mockMediaProvider{imageErr: errors.New("boom")}
	if _, err := executeGenerateImage(context.Background(), mp, GenerateImageInput{Prompt: "x"}); err == nil {
		t.Fatal("expected provider error to propagate")
	}
}

func TestExecuteGenerateVideo_RequiresPrompt(t *testing.T) {
	mp := &mockMediaProvider{}
	if _, err := executeGenerateVideo(context.Background(), mp, GenerateVideoInput{}); err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

func TestExecuteGenerateVideo_NilProvider(t *testing.T) {
	if _, err := executeGenerateVideo(context.Background(), nil, GenerateVideoInput{Prompt: "waves"}); err == nil {
		t.Fatal("expected error for nil provider")
	}
}

func TestExecuteGenerateVideo_Defaults(t *testing.T) {
	mp := &mockMediaProvider{
		videoResult: &media.VideoResult{
			Artifacts: []media.MediaArtifact{{ArtifactID: "v1", URL: "http://x/1.mp4", MimeType: "video/mp4"}},
		},
	}
	out, err := executeGenerateVideo(context.Background(), mp, GenerateVideoInput{Prompt: "waves"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.videoReq.DurationMs != 5000 {
		t.Errorf("expected default duration 5000, got %d", mp.videoReq.DurationMs)
	}
	if mp.videoReq.FPS != 24 {
		t.Errorf("expected default fps 24, got %d", mp.videoReq.FPS)
	}
	if mp.videoReq.Resolution != "720p" {
		t.Errorf("expected default resolution 720p, got %q", mp.videoReq.Resolution)
	}
	if len(out.Artifacts) != 1 || out.Artifacts[0].ArtifactID != "v1" {
		t.Errorf("unexpected artifacts: %+v", out.Artifacts)
	}
}

func TestExecuteGenerateVideo_ProviderError(t *testing.T) {
	mp := &mockMediaProvider{videoErr: errors.New("boom")}
	if _, err := executeGenerateVideo(context.Background(), mp, GenerateVideoInput{Prompt: "x"}); err == nil {
		t.Fatal("expected provider error to propagate")
	}
}

func TestExecuteImageToVideo_RequiresArtifactID(t *testing.T) {
	mp := &mockMediaProvider{}
	if _, err := executeImageToVideo(context.Background(), mp, ImageToVideoInput{}); err == nil {
		t.Fatal("expected error for empty image_artifact_id")
	}
}

func TestExecuteImageToVideo_NilProvider(t *testing.T) {
	if _, err := executeImageToVideo(context.Background(), nil, ImageToVideoInput{ImageArtifactID: "a1"}); err == nil {
		t.Fatal("expected error for nil provider")
	}
}

func TestExecuteImageToVideo_Defaults(t *testing.T) {
	mp := &mockMediaProvider{
		i2vResult: &media.VideoResult{
			Artifacts: []media.MediaArtifact{{ArtifactID: "v2", URL: "http://x/2.mp4", MimeType: "video/mp4"}},
		},
	}
	out, err := executeImageToVideo(context.Background(), mp, ImageToVideoInput{ImageArtifactID: "a1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.i2vReq.ImageArtifactID != "a1" {
		t.Errorf("expected artifact id a1, got %q", mp.i2vReq.ImageArtifactID)
	}
	if mp.i2vReq.DurationMs != 5000 {
		t.Errorf("expected default duration 5000, got %d", mp.i2vReq.DurationMs)
	}
	if mp.i2vReq.FPS != 24 {
		t.Errorf("expected default fps 24, got %d", mp.i2vReq.FPS)
	}
	if len(out.Artifacts) != 1 || out.Artifacts[0].ArtifactID != "v2" {
		t.Errorf("unexpected artifacts: %+v", out.Artifacts)
	}
}

func TestExecuteImageToVideo_ProviderError(t *testing.T) {
	mp := &mockMediaProvider{i2vErr: errors.New("boom")}
	if _, err := executeImageToVideo(context.Background(), mp, ImageToVideoInput{ImageArtifactID: "a1"}); err == nil {
		t.Fatal("expected provider error to propagate")
	}
}

func TestNewTools_ReturnNonNil(t *testing.T) {
	mp := &mockMediaProvider{}
	if NewGenerateImageTool(mp) == nil {
		t.Error("NewGenerateImageTool returned nil")
	}
	if NewGenerateVideoTool(mp) == nil {
		t.Error("NewGenerateVideoTool returned nil")
	}
	if NewImageToVideoTool(mp) == nil {
		t.Error("NewImageToVideoTool returned nil")
	}
}
