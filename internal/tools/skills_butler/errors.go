package skills_butler

import "aranea-agents/pkg/apierror"

var ErrAgentIDRequired = apierror.BadRequest(apierror.DomainSkill, "agent_id is required")

var ErrSkillNameRequired = apierror.BadRequest(apierror.DomainSkill, "skill_name is required")

var ErrImprovementDescRequired = apierror.BadRequest(apierror.DomainSkill, "improvement_description is required")

var ErrTimeRangeRequired = apierror.BadRequest(apierror.DomainSkill, "time_range is required")
