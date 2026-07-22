package media

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

type stubInnerProvider struct {
	image *ImageResult
	video *VideoResult
	err   error
}

func (s *stubInnerProvider) Name() string { return "stub" }

func (s *stubInnerProvider) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.image, nil
}

func (s *stubInnerProvider) GenerateVideo(ctx context.Context, req VideoRequest) (*VideoResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.video, nil
}

func (s *stubInnerProvider) ImageToVideo(ctx context.Context, req ImageToVideoRequest) (*VideoResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.video, nil
}

type fakeArtifactWriter struct {
	saved   []artifactbiz.Artifact
	saveErr error
}

func (f *fakeArtifactWriter) Save(ctx context.Context, sessionID, name, mimeType string, data []byte) (artifactbiz.Artifact, error) {
	if f.saveErr != nil {
		return artifactbiz.Artifact{}, f.saveErr
	}
	a := artifactbiz.Artifact{ID: "art-123", SessionID: sessionID, Name: name, MimeType: mimeType, Size: int64(len(data))}
	f.saved = append(f.saved, a)
	return a, nil
}

func (f *fakeArtifactWriter) Delete(ctx context.Context, id string) error { return nil }

func (f *fakeArtifactWriter) DeleteVersion(ctx context.Context, sessionID, name string, version int) error {
	return nil
}

func ctxWithSession(sessionID string) context.Context {
	inv := trpcagent.NewInvocation()
	inv.Session = &trpcsession.Session{ID: sessionID}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

func TestPersistingProviderNameDelegates(t *testing.T) {
	p := NewPersistingProvider(&stubInnerProvider{}, nil, loggateway.NewNoop())
	if p.Name() != "stub" {
		t.Errorf("Name() = %q, want %q", p.Name(), "stub")
	}
}

func TestPersistingProviderGenerateImagePersistsRemoteURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-bytes"))
	}))
	defer srv.Close()

	inner := &stubInnerProvider{image: &ImageResult{Artifacts: []MediaArtifact{
		{ArtifactID: "qwen_task_0", URL: srv.URL + "/img.png", MimeType: "image/png"},
	}}}
	writer := &fakeArtifactWriter{}
	p := NewPersistingProvider(inner, writer, loggateway.NewNoop())

	res, err := p.GenerateImage(ctxWithSession("sess-1"), ImageRequest{Prompt: "cat"})
	if err != nil {
		t.Fatalf("GenerateImage error: %v", err)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("artifacts len = %d, want 1", len(res.Artifacts))
	}
	got := res.Artifacts[0]
	if got.URL != "artifact://art-123" {
		t.Errorf("URL = %q, want artifact://art-123", got.URL)
	}
	if got.ArtifactID != "art-123" {
		t.Errorf("ArtifactID = %q, want art-123", got.ArtifactID)
	}
	if len(writer.saved) != 1 {
		t.Fatalf("saved len = %d, want 1", len(writer.saved))
	}
	saved := writer.saved[0]
	if saved.SessionID != "sess-1" {
		t.Errorf("saved SessionID = %q, want sess-1", saved.SessionID)
	}
	if !strings.HasPrefix(saved.Name, "generate_image-") || !strings.HasSuffix(saved.Name, ".png") {
		t.Errorf("saved Name = %q, want generate_image-*.png", saved.Name)
	}
	if saved.MimeType != "image/png" {
		t.Errorf("saved MimeType = %q, want image/png", saved.MimeType)
	}
	if saved.Size != int64(len("png-bytes")) {
		t.Errorf("saved Size = %d, want %d", saved.Size, len("png-bytes"))
	}
}

func TestPersistingProviderKeepsRemoteURLOnDownloadFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	inner := &stubInnerProvider{image: &ImageResult{Artifacts: []MediaArtifact{
		{ArtifactID: "qwen_task_0", URL: srv.URL + "/img.png", MimeType: "image/png"},
	}}}
	writer := &fakeArtifactWriter{}
	p := NewPersistingProvider(inner, writer, loggateway.NewNoop())

	res, err := p.GenerateImage(ctxWithSession("sess-1"), ImageRequest{Prompt: "cat"})
	if err != nil {
		t.Fatalf("GenerateImage error: %v", err)
	}
	if res.Artifacts[0].URL != srv.URL+"/img.png" {
		t.Errorf("URL = %q, want original remote URL kept", res.Artifacts[0].URL)
	}
	if len(writer.saved) != 0 {
		t.Errorf("saved len = %d, want 0", len(writer.saved))
	}
}

func TestPersistingProviderKeepsRemoteURLOnSaveFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("png-bytes"))
	}))
	defer srv.Close()

	inner := &stubInnerProvider{image: &ImageResult{Artifacts: []MediaArtifact{
		{ArtifactID: "qwen_task_0", URL: srv.URL + "/img.png", MimeType: "image/png"},
	}}}
	writer := &fakeArtifactWriter{saveErr: errors.New("disk full")}
	p := NewPersistingProvider(inner, writer, loggateway.NewNoop())

	res, err := p.GenerateImage(ctxWithSession("sess-1"), ImageRequest{Prompt: "cat"})
	if err != nil {
		t.Fatalf("GenerateImage error: %v", err)
	}
	if res.Artifacts[0].URL != srv.URL+"/img.png" {
		t.Errorf("URL = %q, want original remote URL kept", res.Artifacts[0].URL)
	}
}

func TestPersistingProviderSkipsWithoutSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("png-bytes"))
	}))
	defer srv.Close()

	inner := &stubInnerProvider{image: &ImageResult{Artifacts: []MediaArtifact{
		{ArtifactID: "qwen_task_0", URL: srv.URL + "/img.png", MimeType: "image/png"},
	}}}
	writer := &fakeArtifactWriter{}
	p := NewPersistingProvider(inner, writer, loggateway.NewNoop())

	res, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "cat"})
	if err != nil {
		t.Fatalf("GenerateImage error: %v", err)
	}
	if res.Artifacts[0].URL != srv.URL+"/img.png" {
		t.Errorf("URL = %q, want original remote URL kept", res.Artifacts[0].URL)
	}
	if len(writer.saved) != 0 {
		t.Errorf("saved len = %d, want 0", len(writer.saved))
	}
}

func TestPersistingProviderSkipsNonHTTPURL(t *testing.T) {
	inner := &stubInnerProvider{image: &ImageResult{Artifacts: []MediaArtifact{
		{ArtifactID: "a1", URL: "artifact://existing", MimeType: "image/png"},
		{ArtifactID: "a2", URL: "", MimeType: "image/png"},
	}}}
	writer := &fakeArtifactWriter{}
	p := NewPersistingProvider(inner, writer, loggateway.NewNoop())

	res, err := p.GenerateImage(ctxWithSession("sess-1"), ImageRequest{Prompt: "cat"})
	if err != nil {
		t.Fatalf("GenerateImage error: %v", err)
	}
	if res.Artifacts[0].URL != "artifact://existing" {
		t.Errorf("URL = %q, want unchanged", res.Artifacts[0].URL)
	}
	if len(writer.saved) != 0 {
		t.Errorf("saved len = %d, want 0", len(writer.saved))
	}
}

func TestPersistingProviderNilWriterPassThrough(t *testing.T) {
	inner := &stubInnerProvider{image: &ImageResult{Artifacts: []MediaArtifact{
		{ArtifactID: "qwen_task_0", URL: "https://example.com/img.png", MimeType: "image/png"},
	}}}
	p := NewPersistingProvider(inner, nil, loggateway.NewNoop())

	res, err := p.GenerateImage(ctxWithSession("sess-1"), ImageRequest{Prompt: "cat"})
	if err != nil {
		t.Fatalf("GenerateImage error: %v", err)
	}
	if res.Artifacts[0].URL != "https://example.com/img.png" {
		t.Errorf("URL = %q, want unchanged", res.Artifacts[0].URL)
	}
}

func TestPersistingProviderGenerateVideoPersists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("mp4-bytes"))
	}))
	defer srv.Close()

	inner := &stubInnerProvider{video: &VideoResult{Artifacts: []MediaArtifact{
		{ArtifactID: "qwen_v_0", URL: srv.URL + "/v.mp4", MimeType: "video/mp4"},
	}}}
	writer := &fakeArtifactWriter{}
	p := NewPersistingProvider(inner, writer, loggateway.NewNoop())

	res, err := p.GenerateVideo(ctxWithSession("sess-1"), VideoRequest{Prompt: "run"})
	if err != nil {
		t.Fatalf("GenerateVideo error: %v", err)
	}
	if res.Artifacts[0].URL != "artifact://art-123" {
		t.Errorf("URL = %q, want artifact://art-123", res.Artifacts[0].URL)
	}
	if !strings.HasSuffix(writer.saved[0].Name, ".mp4") {
		t.Errorf("saved Name = %q, want *.mp4", writer.saved[0].Name)
	}
}

func TestPersistingProviderImageToVideoPersists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("mp4-bytes"))
	}))
	defer srv.Close()

	inner := &stubInnerProvider{video: &VideoResult{Artifacts: []MediaArtifact{
		{ArtifactID: "qwen_i2v_0", URL: srv.URL + "/v.mp4", MimeType: "video/mp4"},
	}}}
	writer := &fakeArtifactWriter{}
	p := NewPersistingProvider(inner, writer, loggateway.NewNoop())

	res, err := p.ImageToVideo(ctxWithSession("sess-1"), ImageToVideoRequest{ImageArtifactID: "a1"})
	if err != nil {
		t.Fatalf("ImageToVideo error: %v", err)
	}
	if res.Artifacts[0].URL != "artifact://art-123" {
		t.Errorf("URL = %q, want artifact://art-123", res.Artifacts[0].URL)
	}
}

func TestPersistingProviderInnerErrorPropagates(t *testing.T) {
	inner := &stubInnerProvider{err: errors.New("boom")}
	p := NewPersistingProvider(inner, &fakeArtifactWriter{}, loggateway.NewNoop())
	if _, err := p.GenerateImage(ctxWithSession("sess-1"), ImageRequest{Prompt: "x"}); err == nil {
		t.Fatal("expected error, got nil")
	}
}
