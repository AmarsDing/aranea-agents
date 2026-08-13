package biz

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
)

// PromptPreview returns the composed system prompt text for an agent (after hydration).
func (u *AgentUsecase) PromptPreview(ctx context.Context, id, mode string) (string, error) {
	a, err := u.Get(ctx, id)
	if err != nil {
		return "", err
	}
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "complete"
	}
	return composePromptPreview(a, mode), nil
}

// CreatePromptFile adds a single prompt file to an agent.
func (u *AgentUsecase) CreatePromptFile(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error) {
	f.AgentID = strings.TrimSpace(f.AgentID)
	f.Name = strings.TrimSpace(f.Name)
	if f.AgentID == "" || f.Name == "" {
		return AgentPromptFile{}, apierror.BadRequest("AGENT_FILE", "agent_id and name are required")
	}
	if _, err := u.Get(ctx, f.AgentID); err != nil {
		return AgentPromptFile{}, err
	}
	created, err := u.files.CreateAgentPromptFile(ctx, f)
	if err != nil {
		return AgentPromptFile{}, err
	}
	return created, nil
}

// UpdatePromptFile modifies a single prompt file.
func (u *AgentUsecase) UpdatePromptFile(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error) {
	f.AgentID = strings.TrimSpace(f.AgentID)
	f.ID = strings.TrimSpace(f.ID)
	if f.AgentID == "" || f.ID == "" {
		return AgentPromptFile{}, apierror.BadRequest("AGENT_FILE", "agent_id and id are required")
	}
	if _, err := u.Get(ctx, f.AgentID); err != nil {
		return AgentPromptFile{}, err
	}
	updated, err := u.files.UpdateAgentPromptFile(ctx, f)
	if err != nil {
		return AgentPromptFile{}, err
	}
	return updated, nil
}

// DeletePromptFile removes a single prompt file.
func (u *AgentUsecase) DeletePromptFile(ctx context.Context, agentID, id string) error {
	agentID = strings.TrimSpace(agentID)
	id = strings.TrimSpace(id)
	if agentID == "" || id == "" {
		return apierror.BadRequest("AGENT_FILE", "agent_id and id are required")
	}
	if _, err := u.Get(ctx, agentID); err != nil {
		return err
	}
	if err := u.files.DeleteAgentPromptFile(ctx, agentID, id); err != nil {
		return err
	}
	return nil
}

// EstimateTokens returns an approximate token count for all prompt files of an agent.
func (u *AgentUsecase) EstimateTokens(ctx context.Context, agentID string) (FileTokenEstimates, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return FileTokenEstimates{}, apierror.BadRequest("AGENT_FILE", "agent_id is required")
	}
	a, err := u.Get(ctx, agentID)
	if err != nil {
		return FileTokenEstimates{}, err
	}
	return estimateTokensForFiles(a.Files), nil
}
