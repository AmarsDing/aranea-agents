package skills_butler

import kerrors "github.com/go-kratos/kratos/v2/errors"

var ErrAgentIDRequired = kerrors.BadRequest("SKILLS_BUTLER", "agent_id is required")

var ErrSkillNameRequired = kerrors.BadRequest("SKILLS_BUTLER", "skill_name is required")

var ErrImprovementDescRequired = kerrors.BadRequest("SKILLS_BUTLER", "improvement_description is required")

var ErrTimeRangeRequired = kerrors.BadRequest("SKILLS_BUTLER", "time_range is required")
