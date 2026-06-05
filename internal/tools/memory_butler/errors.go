package memory_butler

import kerrors "github.com/go-kratos/kratos/v2/errors"

var (
	errAgentIDRequired  = kerrors.BadRequest("MEMORY_BUTLER", "agent_id is required")
	errContentRequired  = kerrors.BadRequest("MEMORY_BUTLER", "content is required")
	errFactIDRequired   = kerrors.BadRequest("MEMORY_BUTLER", "fact_id is required")
	errNoFactsToDelete  = kerrors.BadRequest("MEMORY_BUTLER", "no facts to delete")
	errDreamCycleFailed = kerrors.InternalServer("MEMORY_BUTLER", "dream_cycle execution failed")
)
