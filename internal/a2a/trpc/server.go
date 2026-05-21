package trpc

import (
	"net/http"
	"strings"

	"aranea-agents/internal/biz"

	a2aprotocol "trpc.group/trpc-go/trpc-a2a-go/server"
	a2atrpcserver "trpc.group/trpc-go/trpc-agent-go/server/a2a"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

// BuildA2AEndpointServer exposes a catalog LLM agent via the A2A HTTP protocol.
func BuildA2AEndpointServer(runner trpcrunner.Runner, ag biz.Agent, card biz.A2AAgentCard, publicURL string, streaming bool) (http.Handler, error) {
	protocolCard, err := buildProtocolAgentCard(ag, card, publicURL, streaming)
	if err != nil {
		return nil, err
	}
	srv, err := a2atrpcserver.New(
		a2atrpcserver.WithRunner(runner),
		a2atrpcserver.WithAgentCard(protocolCard),
	)
	if err != nil {
		return nil, err
	}
	return srv.Handler(), nil
}

func buildProtocolAgentCard(ag biz.Agent, card biz.A2AAgentCard, publicURL string, streaming bool) (a2aprotocol.AgentCard, error) {
	name := strings.TrimSpace(card.DisplayName)
	if name == "" {
		name = strings.TrimSpace(ag.DisplayName)
	}
	if name == "" {
		name = strings.TrimSpace(ag.AgentKey)
	}
	desc := strings.TrimSpace(ag.AgentDescription)
	skills := make([]a2aprotocol.AgentSkill, 0, len(card.Capabilities))
	for _, cap := range card.Capabilities {
		capDesc := cap.Description
		skills = append(skills, a2aprotocol.AgentSkill{
			Name:        cap.Name,
			Description: &capDesc,
			InputModes:  []string{"text"},
			OutputModes: []string{"text"},
			Tags:        []string{"capability"},
		})
	}
	if len(skills) == 0 {
		skills = append(skills, a2aprotocol.AgentSkill{
			Name:        "chat",
			Description: &desc,
			InputModes:  []string{"text"},
			OutputModes: []string{"text"},
			Tags:        []string{"default"},
		})
	}
	base, err := a2atrpcserver.NewAgentCard(name, desc, publicURL, streaming)
	if err != nil {
		return a2aprotocol.AgentCard{}, err
	}
	base.Skills = skills
	return base, nil
}
