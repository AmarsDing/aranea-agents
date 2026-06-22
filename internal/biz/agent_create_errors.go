package biz

import (
	"errors"
	"strings"
)

func isAgentKeyDuplicate(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrAgentKeyConflict) {
		return true
	}
	msg := strings.ToLower(err.Error())
	// The (position_key, agent_variant) unique constraint is not an agent_key conflict.
	if strings.Contains(msg, "agent_position_key_agent_variant") {
		return false
	}
	return strings.Contains(msg, "unique") && strings.Contains(msg, "agent_key")
}
