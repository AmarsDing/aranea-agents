package biz

import (
	"context"
	"strconv"
	"strings"

	"aranea-agents/pkg/auth"
)

// AgentListCreatedByMine is the ListAgents created_by filter token for the current user.
const AgentListCreatedByMine = "mine"

// AgentCreatedByFromContext returns the authenticated user id as a string, or "".
func AgentCreatedByFromContext(ctx context.Context) string {
	if a, ok := auth.FromContext(ctx); ok && a != nil && a.UserID > 0 {
		return strconv.FormatInt(a.UserID, 10)
	}
	return ""
}

// ResolveListCreatedByFilter maps API filter tokens to a stored created_by value.
func ResolveListCreatedByFilter(ctx context.Context, filter string) string {
	filter = strings.TrimSpace(strings.ToLower(filter))
	switch filter {
	case "", "all":
		return ""
	case AgentListCreatedByMine:
		return AgentCreatedByFromContext(ctx)
	default:
		return strings.TrimSpace(filter)
	}
}
