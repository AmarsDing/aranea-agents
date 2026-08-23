package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcllmagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
)

// P0-B: session summary 必须以 user 模式注入（尾部 append 区），禁止默认
// system 模式把摘要合并进 system[0]——摘要随压缩变化会字节级击穿前缀缓存。
func TestBuildTRPCRuntimeOptions_SessionSummaryUsesUserInjection(t *testing.T) {
	s := &biz.AgentRuntimeSettings{SessionSummaryEnabled: true}
	opts := buildTRPCRuntimeOptions(s, true, "", "", nil, nil, loggateway.NewNoop())

	probe := &trpcllmagent.Options{}
	for _, opt := range opts {
		opt(probe)
	}
	require.True(t, probe.AddSessionSummary)
	require.Equal(t, trpcllmagent.SessionSummaryInjectionUser, probe.SessionSummaryInjectionMode)
}

// 未开启 session summary 时不得设置注入模式（保持框架默认，不污染选项）。
func TestBuildTRPCRuntimeOptions_SessionSummaryDisabledNoInjectionMode(t *testing.T) {
	s := &biz.AgentRuntimeSettings{}
	opts := buildTRPCRuntimeOptions(s, true, "", "", nil, nil, loggateway.NewNoop())

	probe := &trpcllmagent.Options{}
	for _, opt := range opts {
		opt(probe)
	}
	require.False(t, probe.AddSessionSummary)
	require.Empty(t, probe.SessionSummaryInjectionMode)
}

func TestOversizedToolResultMaxTokens(t *testing.T) {
	if got := oversizedToolResultMaxTokens("coding"); got != 8192 {
		t.Fatalf("coding = %d", got)
	}
	if got := oversizedToolResultMaxTokens("spirit"); got != 2048 {
		t.Fatalf("spirit = %d", got)
	}
}
