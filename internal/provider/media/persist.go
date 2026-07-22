package media

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// ArtifactURLScheme is the URL scheme used for persisted media artifacts.
// The frontend resolves "artifact://<id>" to a fresh signed download URL.
const ArtifactURLScheme = "artifact://"

// mediaDownloadTimeout bounds a single remote media fetch.
const mediaDownloadTimeout = 30 * time.Second

// PersistingProvider decorates a MediaProvider so generated media artifacts
// (remote temporary URLs from dashscope/ComfyUI etc.) are downloaded and
// persisted into the artifact store. Persisted artifacts get their URL
// rewritten to "artifact://<artifact_id>" so historical messages keep a
// stable reference resolvable to a fresh signed download URL.
//
// Persistence is best-effort: on any failure (download, size limit, save)
// the original remote URL is kept and a warning is logged, never failing
// the tool result.
type PersistingProvider struct {
	inner     MediaProvider
	artifacts artifactbiz.Saver
	http      *http.Client
	lg        loggateway.Logger
}

var _ MediaProvider = (*PersistingProvider)(nil)

// NewPersistingProvider wraps inner with artifact persistence. A nil
// artifacts saver disables persistence (results pass through unchanged).
func NewPersistingProvider(inner MediaProvider, artifacts artifactbiz.Saver, lg loggateway.Logger) *PersistingProvider {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &PersistingProvider{
		inner:     inner,
		artifacts: artifacts,
		http:      &http.Client{Timeout: mediaDownloadTimeout},
		lg:        lg,
	}
}

func (p *PersistingProvider) Name() string { return p.inner.Name() }

func (p *PersistingProvider) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	res, err := p.inner.GenerateImage(ctx, req)
	if err != nil {
		return nil, err
	}
	for i := range res.Artifacts {
		res.Artifacts[i] = p.persistArtifact(ctx, "generate_image", i, res.Artifacts[i])
	}
	return res, nil
}

func (p *PersistingProvider) GenerateVideo(ctx context.Context, req VideoRequest) (*VideoResult, error) {
	res, err := p.inner.GenerateVideo(ctx, req)
	if err != nil {
		return nil, err
	}
	for i := range res.Artifacts {
		res.Artifacts[i] = p.persistArtifact(ctx, "generate_video", i, res.Artifacts[i])
	}
	return res, nil
}

func (p *PersistingProvider) ImageToVideo(ctx context.Context, req ImageToVideoRequest) (*VideoResult, error) {
	res, err := p.inner.ImageToVideo(ctx, req)
	if err != nil {
		return nil, err
	}
	for i := range res.Artifacts {
		res.Artifacts[i] = p.persistArtifact(ctx, "image_to_video", i, res.Artifacts[i])
	}
	return res, nil
}

// persistArtifact downloads the remote media and stores it as an artifact.
// On success the returned MediaArtifact has ArtifactID replaced by the real
// artifact ID and URL rewritten to the artifact:// scheme. On any failure
// the original MediaArtifact is returned unchanged (best-effort degrade).
func (p *PersistingProvider) persistArtifact(ctx context.Context, toolName string, idx int, a MediaArtifact) MediaArtifact {
	if p.artifacts == nil {
		return a
	}
	if !strings.HasPrefix(a.URL, "http://") && !strings.HasPrefix(a.URL, "https://") {
		return a
	}
	sessionID := mediaSessionIDFromCtx(ctx)
	if sessionID == "" {
		p.lg.Warn("媒体产物跳过落盘：上下文无会话 ID",
			loggateway.StepID("media.persist"),
			loggateway.Str("tool", toolName))
		return a
	}
	data, mimeType, err := p.download(ctx, a.URL, a.MimeType)
	if err != nil {
		p.lg.Warn("媒体产物下载失败，保留远程 URL",
			loggateway.StepID("media.persist"),
			loggateway.Str("tool", toolName),
			loggateway.Err(err))
		return a
	}
	name := mediaArtifactFileName(toolName, idx, mimeType)
	saved, err := p.artifacts.Save(ctx, sessionID, name, mimeType, data)
	if err != nil {
		p.lg.Warn("媒体产物落盘失败，保留远程 URL",
			loggateway.StepID("media.persist"),
			loggateway.Str("tool", toolName),
			loggateway.Err(err))
		return a
	}
	a.ArtifactID = saved.ID
	a.URL = ArtifactURLScheme + saved.ID
	return a
}

// download fetches the remote media bytes with a 30s timeout and enforces
// the artifact upload size limit. mimeType falls back to the response
// Content-Type header when the provider did not supply one.
func (p *PersistingProvider) download(ctx context.Context, url string, mimeType string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build request: %w", err)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("fetch: unexpected status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, artifactbiz.MaxUploadBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}
	if err := artifactbiz.ValidateUploadSize(int64(len(data))); err != nil {
		return nil, "", err
	}
	if mimeType == "" {
		mimeType = resp.Header.Get("Content-Type")
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return data, mimeType, nil
}

// mediaSessionIDFromCtx resolves the current session ID from the agent
// invocation context. Returns empty string outside an agent run.
func mediaSessionIDFromCtx(ctx context.Context) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return ""
	}
	return inv.Session.ID
}

// mediaArtifactFileName builds "<tool>-<UTC timestamp>-<idx>.<ext>".
// Name collisions across turns are handled by artifact versioning.
func mediaArtifactFileName(toolName string, idx int, mimeType string) string {
	ts := time.Now().UTC().Format("20060102T150405Z")
	return fmt.Sprintf("%s-%s-%d.%s", toolName, ts, idx, mediaFileExt(mimeType))
}

// mediaFileExt maps common media MIME types to file extensions.
func mediaFileExt(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(strings.SplitN(mimeType, ";", 2)[0])) {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	case "video/mp4":
		return "mp4"
	case "video/webm":
		return "webm"
	case "audio/mpeg":
		return "mp3"
	case "audio/wav", "audio/x-wav":
		return "wav"
	case "audio/ogg":
		return "ogg"
	default:
		return "bin"
	}
}
