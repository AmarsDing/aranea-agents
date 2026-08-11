package service

import (
	"testing"

	v1 "aranea-agents/api/kratos/a2a/v1"
	a2apkg "aranea-agents/internal/a2a"
	"aranea-agents/pkg/loggateway"
)

func TestWithEndpointURL(t *testing.T) {
	newSvc := func(base string) *A2AService {
		return NewA2AService(nil, nil, nil, nil,
			a2apkg.NewPublicBaseURLStore(a2apkg.PublicBaseURLResult{URL: base}),
			nil, loggateway.NewNoop())
	}

	t.Run("enabled card gets endpoint url", func(t *testing.T) {
		card := &v1.A2AAgentCard{AgentId: "agent-1", Enabled: true}
		newSvc("https://a2a.example.com/a2a").withEndpointURL(card, true, "agent-1")
		if card.EndpointUrl != "https://a2a.example.com/a2a/agent-1" {
			t.Fatalf("EndpointUrl = %q, want %q", card.EndpointUrl, "https://a2a.example.com/a2a/agent-1")
		}
	})

	t.Run("disabled card has no endpoint url", func(t *testing.T) {
		card := &v1.A2AAgentCard{AgentId: "agent-1"}
		newSvc("https://a2a.example.com/a2a").withEndpointURL(card, false, "agent-1")
		if card.EndpointUrl != "" {
			t.Fatalf("EndpointUrl = %q, want empty", card.EndpointUrl)
		}
	})

	t.Run("empty public base leaves url empty", func(t *testing.T) {
		card := &v1.A2AAgentCard{AgentId: "agent-1", Enabled: true}
		newSvc("").withEndpointURL(card, true, "agent-1")
		if card.EndpointUrl != "" {
			t.Fatalf("EndpointUrl = %q, want empty", card.EndpointUrl)
		}
	})
}
