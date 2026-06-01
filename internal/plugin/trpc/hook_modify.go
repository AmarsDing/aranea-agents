package plugintrpc

import (
	"encoding/json"
	"strings"

	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ApplyModelModifyPatch merges modify_patch into a model request before invocation.
func ApplyModelModifyPatch(req *trpcmodel.Request, patch map[string]any, lg loggateway.Logger) {
	if req == nil || len(patch) == 0 {
		return
	}
	if raw, ok := patch["generation_config"]; ok && raw != nil {
		overlay := decodeGenerationOverlay(raw, lg)
		mergeGenerationConfig(&req.GenerationConfig, overlay)
	}
	if sys := strings.TrimSpace(stringField(patch, "append_system")); sys != "" {
		req.Messages = append([]trpcmodel.Message{{
			Role:    trpcmodel.RoleSystem,
			Content: sys,
		}}, req.Messages...)
	}
	if user := strings.TrimSpace(stringField(patch, "append_user")); user != "" {
		req.Messages = append(req.Messages, trpcmodel.Message{
			Role:    trpcmodel.RoleUser,
			Content: user,
		})
	}
	if extra, ok := patch["extra_fields"].(map[string]any); ok && len(extra) > 0 {
		if req.ExtraFields == nil {
			req.ExtraFields = make(map[string]any, len(extra))
		}
		for k, v := range extra {
			req.ExtraFields[k] = v
		}
	}
}

// ApplyToolModifyPatch returns modified tool arguments when modify_patch is set.
func ApplyToolModifyPatch(args *trpctool.BeforeToolArgs, patch map[string]any, lg loggateway.Logger) []byte {
	if args == nil || len(patch) == 0 {
		return nil
	}
	return mergeToolArgumentsJSON(args.Arguments, patch, lg)
}

func decodeGenerationOverlay(raw any, lg loggateway.Logger) trpcmodel.GenerationConfig {
	b, err := json.Marshal(raw)
	if err != nil {
		return trpcmodel.GenerationConfig{}
	}
	var overlay trpcmodel.GenerationConfig
	if err := json.Unmarshal(b, &overlay); err != nil {
		lg.Warn("解析 generation overlay 失败", loggateway.StepID("plugin.trpc.hook_modify"), loggateway.Err(err))
	}
	return overlay
}

func mergeGenerationConfig(base *trpcmodel.GenerationConfig, overlay trpcmodel.GenerationConfig) {
	if base == nil {
		return
	}
	if overlay.MaxTokens != nil {
		base.MaxTokens = overlay.MaxTokens
	}
	if overlay.Temperature != nil {
		base.Temperature = overlay.Temperature
	}
	if overlay.TopP != nil {
		base.TopP = overlay.TopP
	}
	if overlay.PresencePenalty != nil {
		base.PresencePenalty = overlay.PresencePenalty
	}
	if overlay.FrequencyPenalty != nil {
		base.FrequencyPenalty = overlay.FrequencyPenalty
	}
	if overlay.ReasoningEffort != nil {
		base.ReasoningEffort = overlay.ReasoningEffort
	}
	if overlay.ThinkingEnabled != nil {
		base.ThinkingEnabled = overlay.ThinkingEnabled
	}
	if overlay.ThinkingTokens != nil {
		base.ThinkingTokens = overlay.ThinkingTokens
	}
	if len(overlay.Stop) > 0 {
		base.Stop = append([]string(nil), overlay.Stop...)
	}
	if overlay.Stream {
		base.Stream = overlay.Stream
	}
}

func stringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}
