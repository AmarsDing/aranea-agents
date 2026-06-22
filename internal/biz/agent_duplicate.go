package biz

import (
	"context"
	"regexp"
	"strings"

	"aranea-agents/pkg/apierror"
)

var keySanitizer = regexp.MustCompile(`[^a-z0-9]+`)

// Duplicate clones an agent with settings and prompt files (AGT-10).
func (u *AgentUsecase) Duplicate(ctx context.Context, id string) (Agent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, apierror.BadRequest("AGENT", "id is required")
	}
	src, err := u.Get(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	suffix := newAgentCatalogID()
	if len(suffix) > 6 {
		suffix = strings.ToLower(suffix[:6])
	}
	copy := src
	copy.ID = ""
	// Sanitize the source key so the duplicated key always passes the
	// lowercase-letters/digits/hyphens validation rule.
	baseKey := strings.ToLower(strings.TrimSpace(src.AgentKey))
	baseKey = keySanitizer.ReplaceAllString(baseKey, "-")
	baseKey = strings.Trim(baseKey, "-")
	if baseKey == "" {
		baseKey = "agent"
	}
	copy.AgentKey = baseKey + "-copy-" + suffix
	copy.DisplayName = strings.TrimSpace(src.DisplayName) + " Copy"
	copy.IsDefault = BoolPtr(false)
	copy.IsFavorite = BoolPtr(false)
	copy.CreatedAt = ""
	copy.UpdatedAt = ""
	copy.DeletedAt = ""
	copy.LastRunStatus = ""
	copy.LastRunAt = ""
	copy.PendingEvolutionCount = 0
	copy.CreatedBy = ""
	copy.PositionID = ""
	copy.PositionKey = ""
	// The (position_key, agent_variant) unique index requires a distinct pair.
	// Use the already-unique duplicated agent_key as the variant so the copy never
	// collides with the source or any other agent.
	copy.AgentVariant = copy.AgentKey
	copy.VariantDescription = ""
	if copy.Settings != nil {
		settings := *copy.Settings
		settings.AgentID = ""
		copy.Settings = &settings
	}
	if len(copy.Files) > 0 {
		files := make([]AgentPromptFile, len(copy.Files))
		for i, f := range copy.Files {
			files[i] = f
			files[i].ID = ""
			files[i].AgentID = ""
		}
		copy.Files = files
	}
	for attempt := 0; attempt < 5; attempt++ {
		ok, msg, err := u.CheckAgentKeyAvailability(ctx, copy.AgentKey)
		if err != nil {
			return Agent{}, err
		}
		if ok {
			break
		}
		if attempt == 4 {
			return Agent{}, apierror.BadRequest("AGENT_KEY_INVALID", msg)
		}
		copy.AgentKey = baseKey + "-copy-" + strings.ToLower(newAgentCatalogID()[:6])
	}
	return u.create(ctx, copy, true)
}
