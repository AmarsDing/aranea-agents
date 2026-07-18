package media

import (
	"context"
	"testing"
)

func TestMediaProviderInterface(t *testing.T) {
	// Verify interface is satisfiable
	var _ MediaProvider = (*mockProvider)(nil)
}

type mockProvider struct{}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) GenerateImage(_ context.Context, _ ImageRequest) (*ImageResult, error) {
	return &ImageResult{Artifacts: []MediaArtifact{{ArtifactID: "a1", MimeType: "image/png"}}}, nil
}
func (m *mockProvider) GenerateVideo(_ context.Context, _ VideoRequest) (*VideoResult, error) {
	return &VideoResult{Artifacts: []MediaArtifact{{ArtifactID: "v1", MimeType: "video/mp4"}}}, nil
}
func (m *mockProvider) ImageToVideo(_ context.Context, _ ImageToVideoRequest) (*VideoResult, error) {
	return &VideoResult{Artifacts: []MediaArtifact{{ArtifactID: "v2", MimeType: "video/mp4"}}}, nil
}

func TestImageRequestFields(t *testing.T) {
	req := ImageRequest{Prompt: "a cat", Size: "1024x1024", Count: 1}
	if req.Prompt != "a cat" {
		t.Errorf("expected prompt 'a cat', got %q", req.Prompt)
	}
}

func TestMediaArtifactFields(t *testing.T) {
	a := MediaArtifact{
		ArtifactID: "art_1", URL: "https://example.com/img.png",
		MimeType: "image/png", Width: 1024, Height: 1024,
	}
	if a.Width != 1024 {
		t.Errorf("expected width 1024, got %d", a.Width)
	}
}

func TestRegistry(t *testing.T) {
	Register("test_mock", func(cfg ProviderConfig) (MediaProvider, error) {
		return &mockProvider{}, nil
	})

	p, err := Get("test_mock", ProviderConfig{Name: "test_mock"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p.Name() != "mock" {
		t.Errorf("expected name 'mock', got %q", p.Name())
	}
}

func TestRegistryNotFound(t *testing.T) {
	_, err := Get("nonexistent", ProviderConfig{Name: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for unregistered provider")
	}
}
