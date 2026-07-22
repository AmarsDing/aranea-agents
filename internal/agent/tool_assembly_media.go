package agent

import (
	"context"

	bizmedia "aranea-agents/internal/biz/media"
	mediaprovider "aranea-agents/internal/provider/media"
	mediatools "aranea-agents/internal/tools/media"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Media generation tool keys (catalog tool_key values, seeded as builtins).
const (
	toolKeyGenerateImage = "generate_image"
	toolKeyGenerateVideo = "generate_video"
	toolKeyImageToVideo  = "image_to_video"
)

// mediaToolSpec maps an effective tool key to the capability its provider
// must support and the tool constructor.
type mediaToolSpec struct {
	key        string
	capability bizmedia.Capability
	newTool    func(mp mediaprovider.MediaProvider) trpctool.Tool
}

var mediaToolSpecs = []mediaToolSpec{
	{key: toolKeyGenerateImage, capability: bizmedia.CapabilityImage, newTool: mediatools.NewGenerateImageTool},
	{key: toolKeyGenerateVideo, capability: bizmedia.CapabilityVideo, newTool: mediatools.NewGenerateVideoTool},
	{key: toolKeyImageToVideo, capability: bizmedia.CapabilityImageToVideo, newTool: mediatools.NewImageToVideoTool},
}

// resolveMediaTools builds media generation tools for enabled effective keys.
// Each tool resolves the first active provider supporting its capability from
// the media_providers catalog and wraps it with the artifact persisting
// decorator (PersistingProvider), so generated media is downloaded and stored
// as a session artifact instead of expiring on the remote host.
//
// Resolution failures (no provider configured, unknown provider type) skip
// the tool with a warning and never fail the agent build. When
// deps.MediaProviders is nil the feature is unavailable and nil is returned.
func resolveMediaTools(ctx context.Context, eff map[string]bool, deps TRPCBuilderDeps) []trpctool.Tool {
	if deps.MediaProviders == nil || len(eff) == 0 {
		return nil
	}
	lg := deps.Logger()
	var out []trpctool.Tool
	for _, spec := range mediaToolSpecs {
		if !eff[spec.key] {
			continue
		}
		cfg, err := deps.MediaProviders.ActiveProviderFor(ctx, spec.capability)
		if err != nil {
			lg.Warn("媒体工具跳过：未配置可用媒体提供方",
				loggateway.StepID("agent.tool_build"),
				loggateway.Str("tool_key", spec.key),
				loggateway.Str("capability", string(spec.capability)),
				loggateway.Err(err))
			continue
		}
		inner, err := mediaprovider.Get(cfg.ProviderType, mediaprovider.ProviderConfig{
			Name:         cfg.Name,
			ProviderType: cfg.ProviderType,
			BaseURL:      cfg.BaseURL,
			APIKey:       cfg.APIKey,
			Extra:        cfg.Extra(),
		})
		if err != nil {
			lg.Warn("媒体工具跳过：提供方构造失败",
				loggateway.StepID("agent.tool_build"),
				loggateway.Str("tool_key", spec.key),
				loggateway.Str("provider_type", cfg.ProviderType),
				loggateway.Err(err))
			continue
		}
		out = append(out, spec.newTool(mediaprovider.NewPersistingProvider(inner, deps.ArtifactWriter, lg)))
	}
	return out
}
