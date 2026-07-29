package a2a

import (
	"context"
	"testing"

	a2abiz "aranea-agents/internal/biz/a2a"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

func TestFederationRemoteInvoker_PortCompliance(t *testing.T) {
	var _ a2abiz.RemoteInvokeExecutor = (*FederationRemoteInvoker)(nil)
}

func TestFederationRemoteInvoker_DelegatesGuards(t *testing.T) {
	invoker := NewFederationRemoteInvoker(loggateway.NewNoop())

	// Disabled remote agent: Forbidden, no dial.
	_, err := invoker.InvokeRemote(context.Background(), a2abiz.RemoteAgent{
		ID: "ra-1", Enabled: false, RemoteURL: "https://b.example.com/a2a",
	}, "chat", `{"text":"hi"}`, 30)
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("disabled agent err = %v, want 403", err)
	}

	// SSRF-blocked URL: BadRequest, no dial.
	_, err = invoker.InvokeRemote(context.Background(), a2abiz.RemoteAgent{
		ID: "ra-2", Enabled: true, RemoteURL: "http://127.0.0.1:1/a2a",
		DiscoveredCard: a2abiz.AgentCard{Enabled: true, Capabilities: []a2abiz.Capability{{Name: "chat"}}},
	}, "chat", `{"text":"hi"}`, 30)
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("SSRF-blocked err = %v, want 400", err)
	}
}
