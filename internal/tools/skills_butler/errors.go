package skills_butler

import kerrors "github.com/go-kratos/kratos/v2/errors"

var errAgentIDRequired = kerrors.BadRequest("SKILLS_BUTLER", "agent_id is required")

var errSkillNameRequired = kerrors.BadRequest("SKILLS_BUTLER", "skill_name is required")

var errImprovementDescRequired = kerrors.BadRequest("SKILLS_BUTLER", "improvement_description is required")
