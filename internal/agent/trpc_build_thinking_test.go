package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"aranea-agents/internal/biz"
)

// P2-5 思考强度路由：主运行时静态档接线。
//
// 激活语义：仅 reasoning_mode=custom 且 level 为合法档时注入 thinking 参数；
// 默认（provider_default + off / nil Settings）不注入任何 thinking 字段——
// 存量 agent 零行为变化。

func TestGenerationConfigForAgent_DefaultNoInjection(t *testing.T) {
	// nil Settings。
	cfg := generationConfigForAgent(biz.Agent{})
	require.True(t, cfg.Stream)
	require.Nil(t, cfg.ThinkingEnabled)
	require.Nil(t, cfg.ReasoningEffort)

	// provider_default + off（存量默认）。
	cfg = generationConfigForAgent(biz.Agent{Settings: &biz.AgentRuntimeSettings{
		ReasoningMode: "provider_default", ReasoningLevel: "off",
	}})
	require.True(t, cfg.Stream)
	require.Nil(t, cfg.ThinkingEnabled)
	require.Nil(t, cfg.ReasoningEffort)

	// provider_default + high：mode 非 custom 仍不注入（跟随厂商）。
	cfg = generationConfigForAgent(biz.Agent{Settings: &biz.AgentRuntimeSettings{
		ReasoningMode: "provider_default", ReasoningLevel: "high",
	}})
	require.Nil(t, cfg.ThinkingEnabled)
	require.Nil(t, cfg.ReasoningEffort)
}

func TestGenerationConfigForAgent_CustomOffMapsThinkingDisabled(t *testing.T) {
	cfg := generationConfigForAgent(biz.Agent{Settings: &biz.AgentRuntimeSettings{
		ReasoningMode: "custom", ReasoningLevel: "off",
	}})
	require.NotNil(t, cfg.ThinkingEnabled)
	require.False(t, *cfg.ThinkingEnabled)
	require.Nil(t, cfg.ReasoningEffort)
}

func TestGenerationConfigForAgent_CustomLevelMapsReasoningEffort(t *testing.T) {
	for _, level := range []string{"low", "medium", "high"} {
		cfg := generationConfigForAgent(biz.Agent{Settings: &biz.AgentRuntimeSettings{
			ReasoningMode: "custom", ReasoningLevel: level,
		}})
		require.NotNil(t, cfg.ReasoningEffort, "level=%s", level)
		require.Equal(t, level, *cfg.ReasoningEffort)
		require.Nil(t, cfg.ThinkingEnabled, "level=%s 不得注入 thinking 开关", level)
	}
}

func TestGenerationConfigForAgent_CustomGarbageLevelNoInjection(t *testing.T) {
	cfg := generationConfigForAgent(biz.Agent{Settings: &biz.AgentRuntimeSettings{
		ReasoningMode: "custom", ReasoningLevel: "turbo",
	}})
	require.Nil(t, cfg.ThinkingEnabled)
	require.Nil(t, cfg.ReasoningEffort)
}
