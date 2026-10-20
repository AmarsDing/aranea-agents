package tool

import (
	"aranea-agents/pkg/apierror"
	"context"
	"encoding/json"
	"strings"
)

func (u *ToolUsecase) UpsertToolAgentOverride(ctx context.Context, in ToolAgentOverrideInput) (ToolAgentOverride, error) {
	if strings.TrimSpace(in.AgentID) == "" {
		return ToolAgentOverride{}, apierror.BadRequest("TOOL", "agent id is required")
	}
	tool, err := u.GetTool(ctx, in.ToolKey)
	if err != nil {
		return ToolAgentOverride{}, err
	}
	in.ToolKey = tool.Key
	if in.Mode == "" {
		in.Mode = "inherit"
	}
	in.ConfigOverrideJSON = strings.TrimSpace(in.ConfigOverrideJSON)
	if in.ConfigOverrideJSON == "" {
		in.ConfigOverrideJSON = "{}"
	}
	if !json.Valid([]byte(in.ConfigOverrideJSON)) {
		return ToolAgentOverride{}, apierror.BadRequest("TOOL", "config_override_json must be valid JSON")
	}
	return u.repo.UpsertToolAgentOverride(ctx, in, tool.ID)
}

func (u *ToolUsecase) DeleteToolAgentOverride(ctx context.Context, toolIDOrKey string, agentID string) error {
	if strings.TrimSpace(agentID) == "" {
		return apierror.BadRequest("TOOL", "agent id is required")
	}
	key, err := u.ResolveToolKey(ctx, toolIDOrKey)
	if err != nil {
		return err
	}
	return u.repo.DeleteToolAgentOverride(ctx, key, agentID)
}
