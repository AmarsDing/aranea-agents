package biz

import (
	"strings"

	"entgo.io/ent/dialect/sql/sqlgraph"
)

func isAgentKeyDuplicate(err error) bool {
	if err == nil {
		return false
	}
	if sqlgraph.IsConstraintError(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") && strings.Contains(msg, "agent_key")
}
