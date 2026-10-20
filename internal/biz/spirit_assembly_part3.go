package biz

import (
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"context"
)

// InjectDeptLeadIntoTeam adds the department lead agent to a team's definition.
// Called during team assembly for cross-department collaboration.
// Domain: Assembly — dept lead injection into team definition.
func (a *SpiritAssembly) InjectDeptLeadIntoTeam(ctx context.Context, teamID string) error {
	if a.deptLeadMgr == nil {
		return nil
	}
	t, err := a.teamUC.Get(ctx, teamID)
	if err != nil {
		return err
	}
	if t.DepartmentID == "" {
		return nil
	}
	lead, err := a.deptLeadMgr.GetDeptLeadForTeam(ctx, t.DepartmentID)
	if err != nil {
		a.lg.Warn("获取部门主管失败，跳过注入",
			loggateway.StepID("spirit.inject_dept_lead"),
			loggateway.Str("team_id", teamID),
			loggateway.Str("dept_id", t.DepartmentID),
			loggateway.Err(err),
		)
		return nil
	}

	_, err = a.teamUC.Update(ctx, teamID, Team{DeptLeadAgentID: lead.ID})
	if err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, "SPIRIT")
	}

	a.lg.Info("注入部门主管到团队",
		loggateway.StepID("spirit.inject_dept_lead"),
		loggateway.Str("team_id", teamID),
		loggateway.Str("dept_lead_agent_id", lead.ID),
	)
	return nil
}
