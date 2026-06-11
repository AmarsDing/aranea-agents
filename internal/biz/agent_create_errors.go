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
	return strings.Contains(msg, "unique") && strings.Contains(msg, "agent_key")
}
