package trpc

import (
	"testing"
)

func TestAgentIDFromPublicPath_Empty(t *testing.T) {
	if got := agentIDFromPublicPath(""); got != "" {
		t.Fatalf("expected '', got %q", got)
	}
}

func TestAgentIDFromPublicPath_JustPrefix(t *testing.T) {
	if got := agentIDFromPublicPath("/v1/a2a/public/"); got != "" {
		t.Fatalf("expected '', got %q", got)
	}
}

func TestAgentIDFromPublicPath_AgentOnly(t *testing.T) {
	if got := agentIDFromPublicPath("/v1/a2a/public/agent1"); got != "agent1" {
		t.Fatalf("expected 'agent1', got %q", got)
	}
}

func TestAgentIDFromPublicPath_AgentWithSuffix(t *testing.T) {
	if got := agentIDFromPublicPath("/v1/a2a/public/agent1/.well-known/agent-card.json"); got != "agent1" {
		t.Fatalf("expected 'agent1', got %q", got)
	}
}

func TestAgentIDFromPublicPath_AgentWithTrailingSlash(t *testing.T) {
	if got := agentIDFromPublicPath("/v1/a2a/public/agent1/"); got != "agent1" {
		t.Fatalf("expected 'agent1', got %q", got)
	}
}

func TestAgentIDFromPublicPath_WhitespaceAgentID(t *testing.T) {
	if got := agentIDFromPublicPath("/v1/a2a/public/  "); got != "" {
		t.Fatalf("expected '', got %q", got)
	}
}

func TestEndpointRegistry_BaseURL_Nil(t *testing.T) {
	var r *EndpointRegistry
	if r.BaseURL() != "" {
		t.Fatalf("expected empty string, got %q", r.BaseURL())
	}
}

func TestEndpointRegistry_Invalidate_Nil(t *testing.T) {
	var r *EndpointRegistry
	r.Invalidate("agent1")
}

func TestEndpointRegistry_InvalidateAll_Nil(t *testing.T) {
	var r *EndpointRegistry
	r.InvalidateAll()
}

func TestPublicPathPrefix(t *testing.T) {
	if PublicPathPrefix != "/v1/a2a/public/" {
		t.Fatalf("expected /v1/a2a/public/, got %s", PublicPathPrefix)
	}
}
