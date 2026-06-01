package biz

import (
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestNormalizeAgentKind(t *testing.T) {
	if got := NormalizeAgentKind(""); got != AgentKindLLM {
		t.Fatalf("empty: got %q", got)
	}
	if got := NormalizeAgentKind("a2a_proxy"); got != AgentKindA2AProxy {
		t.Fatalf("proxy: got %q", got)
	}
}

func TestEmbedAndHydrateAgentKind(t *testing.T) {
	cfg := &A2AProxyConfig{RemoteURL: "http://localhost:8087/"}
	raw := EmbedAgentKindInConfigJSON("{}", AgentKindA2AProxy, cfg, loggateway.NewNoop())
	var ag Agent
	ag.ConfigJSON = raw
	HydrateAgentKind(&ag)
	if ag.Kind != AgentKindA2AProxy {
		t.Fatalf("kind: got %q", ag.Kind)
	}
	if ag.A2AProxy == nil || ag.A2AProxy.RemoteURL != cfg.RemoteURL {
		t.Fatalf("proxy config not hydrated: %+v", ag.A2AProxy)
	}
}
