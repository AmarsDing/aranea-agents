package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// ArtifactURLScheme is the URL scheme used for persisted media artifacts.
// The frontend resolves "artifact://<id>" to a fresh signed download URL.
const ArtifactURLScheme = "artifact://"

// mediaDownloadTimeout bounds a single remote media fetch.
const mediaDownloadTimeout = 30 * time.Second

// mediaFlowStepID is the flow log (流程日志) step ID for media generation,
// registered in internal/event/flow_log.go ("媒体生成").
const mediaFlowStepID = "media.generate"

// maxPromptLogLen bounds prompt text included in flow log extras
// (红线 #25: extras 不含 prompt 全文，仅保留前缀供辨识).
const maxPromptLogLen = 50

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
	flowBus   contract.MonitorBus
}

var _ MediaProvider = (*PersistingProvider)(nil)

// PersistingProviderOption customizes a PersistingProvider (functional
// options, nil-safe).
type PersistingProviderOption func(*PersistingProvider)

// WithMediaFlowBus injects the monitor bus used to emit media.generate flow
// logs (流程日志); nil disables emission.
func WithMediaFlowBus(bus contract.MonitorBus) PersistingProviderOption {
	return func(p *PersistingProvider) { p.flowBus = bus }
}

// NewPersistingProvider wraps inner with artifact persistence. A nil
// artifacts saver disables persistence (results pass through unchanged).
func NewPersistingProvider(inner MediaProvider, artifacts artifactbiz.Saver, lg loggateway.Logger, opts ...PersistingProviderOption) *PersistingProvider {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	p := &PersistingProvider{
		inner:     inner,
		artifacts: artifacts,
		http:      &http.Client{Timeout: mediaDownloadTimeout},
		lg:        lg,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *PersistingProvider) Name() string { return p.inner.Name() }

// newFlow builds a run-scoped flow emitter for one generation call.
// Returns nil when no monitor bus is wired (tests / minimal setups).
func (p *PersistingProvider) newFlow(ctx context.Context) *event.TraceEmitter {
	if p == nil || p.flowBus == nil {
		return nil
	}
	return event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:       ctx,
		SessionID: mediaSessionIDFromCtx(ctx),
		Domain:    event.TraceDomainSystem,
		LG:        p.lg,
		Infra:     event.NewInfraFromBus(p.flowBus),
	})
}

// flowStart emits the media.generate start-phase flow log and returns the
// emitter for the matching done/error phase (nil when bus not wired).
func (p *PersistingProvider) flowStart(ctx context.Context, message string, pairs ...event.Pair) *event.TraceEmitter {
	flow := p.newFlow(ctx)
	if flow == nil {
		return nil
	}
	flow.LogStart(mediaFlowStepID, message, pairs...)
	return flow
}

func (p *PersistingProvider) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	flow := p.flowStart(ctx, "媒体生成开始（文生图）",
		event.P("provider", p.inner.Name()),
		event.P("kind", "image"),
		event.P("size", req.Size),
		event.P("prompt", truncatePrompt(req.Prompt)))
	res, err := p.inner.GenerateImage(ctx, req)
	if err != nil {
		p.lg.Error("媒体生成失败",
			loggateway.StepID(mediaFlowStepID),
			loggateway.Str("provider", p.inner.Name()),
			loggateway.Str("kind", "image"),
			loggateway.Err(err))
		if flow != nil {
			flow.LogError(mediaFlowStepID, "媒体生成失败（文生图）",
				event.P("provider", p.inner.Name()),
				event.P("kind", "image"),
				event.P("error", err.Error()))
		}
		return nil, err
	}
	for i := range res.Artifacts {
		res.Artifacts[i] = p.persistArtifact(ctx, "generate_image", i, res.Artifacts[i])
	}
	if flow != nil {
		flow.LogDone(mediaFlowStepID, "媒体生成完成（文生图）",
			event.P("provider", p.inner.Name()),
			event.P("kind", "image"),
			event.P("size", req.Size),
			event.P("artifact_ids", artifactIDs(res.Artifacts)))
	}
	return res, nil
}

func (p *PersistingProvider) GenerateVideo(ctx context.Context, req VideoRequest) (*VideoResult, error) {
	flow := p.flowStart(ctx, "媒体生成开始（文生视频）",
		event.P("provider", p.inner.Name()),
		event.P("kind", "video"),
		event.P("resolution", req.Resolution),
		event.P("duration_ms", req.DurationMs),
		event.P("prompt", truncatePrompt(req.Prompt)))
	res, err := p.inner.GenerateVideo(ctx, req)
	if err != nil {
		p.lg.Error("媒体生成失败",
			loggateway.StepID(mediaFlowStepID),
			loggateway.Str("provider", p.inner.Name()),
			loggateway.Str("kind", "video"),
			loggateway.Err(err))
		if flow != nil {
			flow.LogError(mediaFlowStepID, "媒体生成失败（文生视频）",
				event.P("provider", p.inner.Name()),
				event.P("kind", "video"),
				event.P("error", err.Error()))
		}
		return nil, err
	}
	for i := range res.Artifacts {
		res.Artifacts[i] = p.persistArtifact(ctx, "generate_video", i, res.Artifacts[i])
	}
	if flow != nil {
		flow.LogDone(mediaFlowStepID, "媒体生成完成（文生视频）",
			event.P("provider", p.inner.Name()),
			event.P("kind", "video"),
			event.P("resolution", req.Resolution),
			event.P("artifact_ids", artifactIDs(res.Artifacts)))
	}
	return res, nil
}

func (p *PersistingProvider) ImageToVideo(ctx context.Context, req ImageToVideoRequest) (*VideoResult, error) {
	flow := p.flowStart(ctx, "媒体生成开始（图生视频）",
		event.P("provider", p.inner.Name()),
		event.P("kind", "image_to_video"),
		event.P("input_artifact_id", req.ImageArtifactID),
		event.P("prompt", truncatePrompt(req.Prompt)))
	res, err := p.inner.ImageToVideo(ctx, req)
	if err != nil {
		p.lg.Error("媒体生成失败",
			loggateway.StepID(mediaFlowStepID),
			loggateway.Str("provider", p.inner.Name()),
			loggateway.Str("kind", "image_to_video"),
			loggateway.Err(err))
		if flow != nil {
			flow.LogError(mediaFlowStepID, "媒体生成失败（图生视频）",
				event.P("provider", p.inner.Name()),
				event.P("kind", "image_to_video"),
				event.P("error", err.Error()))
		}
		return nil, err
	}
	for i := range res.Artifacts {
		res.Artifacts[i] = p.persistArtifact(ctx, "image_to_video", i, res.Artifacts[i])
	}
	if flow != nil {
		flow.LogDone(mediaFlowStepID, "媒体生成完成（图生视频）",
			event.P("provider", p.inner.Name()),
			event.P("kind", "image_to_video"),
			event.P("artifact_ids", artifactIDs(res.Artifacts)))
	}
	return res, nil
}

// truncatePrompt bounds prompt text for log extras (first maxPromptLogLen
// runes + ellipsis).
func truncatePrompt(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > maxPromptLogLen {
		return string(r[:maxPromptLogLen]) + "…"
	}
	return s
}

// artifactIDs collects non-empty artifact IDs for flow log extras.
func artifactIDs(arts []MediaArtifact) []string {
	ids := make([]string, 0, len(arts))
	for _, a := range arts {
		if a.ArtifactID != "" {
			ids = append(ids, a.ArtifactID)
		}
	}
	return ids
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
	// K1：产物落库 Info。远程 URL 可能携带签名凭据，只记 hash 不记全文（红线 #25）。
	p.lg.Info("媒体产物已落盘",
		loggateway.StepID("media.persist"),
		loggateway.Str("tool", toolName),
		loggateway.Str("artifact_id", saved.ID),
		loggateway.Str("name", name),
		loggateway.Str("mime_type", mimeType),
		loggateway.Int("bytes", len(data)),
		loggateway.Str("url_hash", mediaURLHash(a.URL)))
	a.ArtifactID = saved.ID
	a.URL = ArtifactURLScheme + saved.ID
	return a
}

// mediaURLHash returns a short sha256 prefix of the remote URL so logs stay
// traceable without persisting signed URLs (which may carry credentials).
func mediaURLHash(u string) string {
	sum := sha256.Sum256([]byte(u))
	return hex.EncodeToString(sum[:])[:12]
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
