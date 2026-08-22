package agent

import (
	"strings"

	"aranea-agents/internal/biz"
)

// TaskGear is the deterministic orchestration gear (R9). No extra LLM.
type TaskGear string

const (
	GearLight  TaskGear = "light"
	GearMedium TaskGear = "medium"
	GearHeavy  TaskGear = "heavy"
)

// GearInput is the evidence used to classify a task. All fields are
// already-known signals (gate, scores, org tree, plan shape).
type GearInput struct {
	// GateSimpleOrClarify is true when PrePlanningGate / DECISION says
	// simple or blocking clarify (R1).
	GateSimpleOrClarify bool
	// UserWantsOrgChain is true when the user explicitly asked for the
	// org reporting chain.
	UserWantsOrgChain bool
	// LongTask is true when existing complexity scores already mark a long task.
	LongTask bool
	// CompanyNodeCount is how many company-level nodes exist in the workspace tree.
	CompanyNodeCount int
	// IntercompanySignal is a coarse lexical hint that the task names another company.
	IntercompanySignal bool
	// CrossDeptDepends is set after Plan/playbook expand when 2+ departments
	// have DependsOn / deliverable contracts.
	CrossDeptDepends bool
	// Current is the gear already chosen (empty = not yet classified).
	Current TaskGear
}

// ClassifyTaskGear decides light / medium / heavy. Early heavy wins over
// medium; light only comes from the existing simple/clarify gate.
func ClassifyTaskGear(in GearInput) TaskGear {
	if in.Current == GearHeavy {
		return GearHeavy
	}
	if in.GateSimpleOrClarify && !in.UserWantsOrgChain && !in.LongTask && !in.CrossDeptDepends {
		return GearLight
	}
	if in.UserWantsOrgChain || in.LongTask || in.CrossDeptDepends {
		return GearHeavy
	}
	if in.CompanyNodeCount >= 2 && in.IntercompanySignal {
		return GearHeavy
	}
	if in.Current == GearLight && !in.GateSimpleOrClarify {
		return GearMedium
	}
	if in.Current == GearMedium {
		return GearMedium
	}
	return GearMedium
}

// UpgradeGearAfterPlan may raise medium → heavy. It never silently
// downgrades an in-flight heavy run.
func UpgradeGearAfterPlan(current TaskGear, crossDeptDepends bool) TaskGear {
	return ClassifyTaskGear(GearInput{Current: current, CrossDeptDepends: crossDeptDepends})
}

// PlanHasCrossDeptDepends is the late-upgrade signal: 2+ domain roots and a DependsOn.
func PlanHasCrossDeptDepends(subtasks []biz.SubTask) bool {
	roots := map[string]struct{}{}
	hasDep := false
	for _, st := range subtasks {
		if len(st.DependsOn) > 0 {
			hasDep = true
		}
		p := strings.TrimSpace(st.DomainPath)
		if p == "" {
			continue
		}
		if i := strings.Index(p, "/"); i > 0 {
			p = p[:i]
		}
		roots[p] = struct{}{}
	}
	return hasDep && len(roots) >= 2
}

// HasOrgChainIntent is a coarse lexical check for explicit org-chain wording.
func HasOrgChainIntent(text string) bool {
	s := strings.ToLower(strings.TrimSpace(text))
	if s == "" {
		return false
	}
	needles := []string{"按组织链", "走编制", "组织汇报", "叫醒总经理", "按公司流程"}
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}
