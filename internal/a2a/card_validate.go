package a2a

import (
	"strings"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// CheckCalleeCard verifies the agent is A2A-enabled and advertises the capability.
func CheckCalleeCard(card biz.A2AAgentCard, getErr error, capability string) error {
	if getErr != nil || !card.Enabled {
		return kerrors.Forbidden("A2A", "agent is not A2A-enabled")
	}
	capability = strings.TrimSpace(capability)
	for _, c := range card.Capabilities {
		if c.Name == capability {
			return nil
		}
	}
	return kerrors.BadRequest("A2A", "capability is not advertised by target agent")
}
