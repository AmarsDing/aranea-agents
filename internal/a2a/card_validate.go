package a2a

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// CheckCalleeCard verifies the agent is A2A-enabled and advertises the capability.
func CheckCalleeCard(card biz.A2AAgentCard, getErr error, capability string) error {
	if getErr != nil || !card.Enabled {
		return apierror.Forbidden(apierror.DomainA2A, "agent is not A2A-enabled")
	}
	capability = strings.TrimSpace(capability)
	for _, c := range card.Capabilities {
		if c.Name == capability {
			return nil
		}
	}
	return apierror.BadRequest(apierror.DomainA2A, "capability is not advertised by target agent")
}
