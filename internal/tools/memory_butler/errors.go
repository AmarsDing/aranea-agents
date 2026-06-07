package memory_butler

import kerrors "github.com/go-kratos/kratos/v2/errors"

var (
	ErrAgentIDRequired  = kerrors.BadRequest("MEMORY_BUTLER", "agent_id is required")
	ErrContentRequired  = kerrors.BadRequest("MEMORY_BUTLER", "content is required")
	ErrFactIDRequired   = kerrors.BadRequest("MEMORY_BUTLER", "fact_id is required")
	ErrNoFactsToDelete  = kerrors.BadRequest("MEMORY_BUTLER", "no facts to delete")
	ErrDreamCycleFailed = kerrors.InternalServer("MEMORY_BUTLER", "dream_cycle execution failed")
)
