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

// resolvedMediaTool 是一个媒体工具的构建输入快照（P0-2 阶段A 两阶段拆分）：
// 计划期解析提供方配置（进分片指纹），构建期（分片缓存未命中时）才实例化工具。
type resolvedMediaTool struct {
	key string
	cfg bizmedia.ProviderConfig
}

// resolveMediaToolConfigs 为启用的媒体工具解析提供方配置。
// 解析失败（未配置可用提供方）跳过该工具并告警，永不失败 agent 构建。
// deps.MediaProviders 为 nil 时特性不可用，返回 nil。
func resolveMediaToolConfigs(ctx context.Context, eff map[string]bool, deps TRPCBuilderDeps) []resolvedMediaTool {
	if deps.MediaProviders == nil || len(eff) == 0 {
		return nil
	}
	lg := deps.Logger()
	var out []resolvedMediaTool
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
		out = append(out, resolvedMediaTool{key: spec.key, cfg: cfg})
	}
	return out
}

// build 实例化单个媒体工具（分片构建闭包内调用）：提供方客户端 + 制品持久化包装。
// 提供方构造失败跳过（告警），与现状 resolveMediaTools 的降级语义一致。
func (r resolvedMediaTool) build(deps TRPCBuilderDeps) trpctool.Tool {
	lg := deps.Logger()
	for _, spec := range mediaToolSpecs {
		if spec.key != r.key {
			continue
		}
		inner, err := mediaprovider.Get(r.cfg.ProviderType, mediaprovider.ProviderConfig{
			Name:         r.cfg.Name,
			ProviderType: r.cfg.ProviderType,
			BaseURL:      r.cfg.BaseURL,
			APIKey:       r.cfg.APIKey,
			Extra:        r.cfg.Extra(),
		})
		if err != nil {
			lg.Warn("媒体工具跳过：提供方构造失败",
				loggateway.StepID("agent.tool_build"),
				loggateway.Str("tool_key", r.key),
				loggateway.Str("provider_type", r.cfg.ProviderType),
				loggateway.Err(err))
			return nil
		}
		return spec.newTool(mediaprovider.NewPersistingProvider(inner, deps.ArtifactWriter, lg))
	}
	return nil
}

// buildMediaTools 实例化全部已解析媒体工具（nil 结果已剔除）。
func buildMediaTools(resolved []resolvedMediaTool, deps TRPCBuilderDeps) []trpctool.Tool {
	var out []trpctool.Tool
	for _, r := range resolved {
		if t := r.build(deps); t != nil {
			out = append(out, t)
		}
	}
	return out
}
