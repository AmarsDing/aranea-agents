package trpc

import (
	"testing"
)

func TestSplitPublicPath_Empty(t *testing.T) {
	agentID, suffix := splitPublicPath("")
	if agentID != "" || suffix != "/" {
		t.Fatalf("expected ('', '/'), got (%q, %q)", agentID, suffix)
	}
}

func TestSplitPublicPath_JustPrefix(t *testing.T) {
	agentID, suffix := splitPublicPath("/v1/a2a/public/")
	if agentID != "" || suffix != "/" {
		t.Fatalf("expected ('', '/'), got (%q, %q)", agentID, suffix)
	}
}

func TestSplitPublicPath_AgentOnly(t *testing.T) {
	agentID, suffix := splitPublicPath("/v1/a2a/public/agent1")
	if agentID != "agent1" || suffix != "/" {
		t.Fatalf("expected ('agent1', '/'), got (%q, %q)", agentID, suffix)
	}
}

func TestSplitPublicPath_AgentWithSuffix(t *testing.T) {
	agentID, suffix := splitPublicPath("/v1/a2a/public/agent1/.well-known/agent.json")
	if agentID != "agent1" || suffix != "/.well-known/agent.json" {
		t.Fatalf("expected ('agent1', '/.well-known/agent.json'), got (%q, %q)", agentID, suffix)
	}
}

func TestSplitPublicPath_AgentWithTrailingSlash(t *testing.T) {
	agentID, suffix := splitPublicPath("/v1/a2a/public/agent1/")
	if agentID != "agent1" || suffix != "/" {
		t.Fatalf("expected ('agent1', '/'), got (%q, %q)", agentID, suffix)
	}
}

func TestSplitPublicPath_WhitespaceAgentID(t *testing.T) {
	agentID, suffix := splitPublicPath("/v1/a2a/public/  ")
	if agentID != "" || suffix != "/" {
		t.Fatalf("expected ('', '/'), got (%q, %q)", agentID, suffix)
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
