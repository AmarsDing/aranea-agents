package memory_butler

import "aranea-agents/pkg/apierror"

var (
	ErrAgentIDRequired = apierror.BadRequest(apierror.DomainMemory, "agent_id is required")
	ErrContentRequired = apierror.BadRequest(apierror.DomainMemory, "content is required")
	// ErrFactIDRequired and ErrNoFactsToDelete are reserved for future tools.
	ErrFactIDRequired  = apierror.BadRequest(apierror.DomainMemory, "fact_id is required")
	ErrNoFactsToDelete = apierror.BadRequest(apierror.DomainMemory, "no facts to delete")
)
