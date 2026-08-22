package agent

import (
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

// CollaborationUnifier is the session-level coordinator. Department leads
// stay on governance (borrow / quality gate) and never unify team I/O.
const (
	unifierSpiritOnly     = "spirit_synthesis"
	unifierBriefAndSpirit = "brief_handoff+spirit_synthesis"
)

// CollaborationSketch describes how multiple specialty teams share work
// without a dept_lead dispatcher.
type CollaborationSketch struct {
	SlotCount int
	EdgeCount int
	Unifier   string
	Summary   string
}

func collaborationSketch(plan *biz.TaskPlan) CollaborationSketch {
	if plan == nil || len(plan.SubTasks) == 0 {
		return CollaborationSketch{}
	}
	edges := 0
	names := make([]string, 0, len(plan.SubTasks))
	for _, st := range plan.SubTasks {
		edges += len(st.DependsOn)
		label := strings.TrimSpace(st.DomainPath)
		if label == "" {
			label = strings.TrimSpace(st.Name)
		}
		if label != "" {
			names = append(names, label)
		}
	}
	unifier := unifierSpiritOnly
	if edges > 0 {
		unifier = unifierBriefAndSpirit
	}
	summary := strings.Join(names, " · ")
	if edges > 0 {
		summary = fmt.Sprintf("%s；依赖边 %d（结论信封交接）", summary, edges)
	} else if len(plan.SubTasks) > 1 {
		summary += "；并行，精灵汇总各队结论"
	}
	return CollaborationSketch{
		SlotCount: len(plan.SubTasks),
		EdgeCount: edges,
		Unifier:   unifier,
		Summary:   summary,
	}
}

func (s CollaborationSketch) meta() map[string]any {
	return map[string]any{
		"slot_count": s.SlotCount,
		"edge_count": s.EdgeCount,
		"unifier":    s.Unifier,
		"sketch":     s.Summary,
	}
}
