package orgimport

import (
	"fmt"
	"strings"
)

// PlanAction represents one create/update operation in the import plan.
type PlanAction struct {
	Kind        string // "create_industry" | "create_department" | "create_position" | "create_agent" | "create_team"
	Key         string
	DisplayName string
	ParentKey   string
	IsUpdate    bool // true when an existing resource will be updated (idempotent)
}

// Plan is the computed list of actions for a Spec.
type Plan struct {
	Actions []PlanAction
}

// BuildPlan constructs the ordered list of actions from a validated Spec.
// Actions are ordered so that parents are always created before children.
func BuildPlan(spec *Spec, existing ExistingResources) Plan {
	var actions []PlanAction

	for _, ind := range spec.Spec.Companies {
		isUpdate := existing.HasCategory(ind.Key)
		actions = append(actions, PlanAction{
			Kind:        "create_industry",
			Key:         ind.Key,
			DisplayName: ind.Name,
			IsUpdate:    isUpdate,
		})
		for _, dept := range ind.Departments {
			isUpdate := existing.HasCategory(dept.Key)
			actions = append(actions, PlanAction{
				Kind:        "create_department",
				Key:         dept.Key,
				DisplayName: dept.Name,
				ParentKey:   ind.Key,
				IsUpdate:    isUpdate,
			})
			for _, pos := range dept.Positions {
				isUpdate := existing.HasCategory(pos.Key)
				actions = append(actions, PlanAction{
					Kind:        "create_position",
					Key:         pos.Key,
					DisplayName: pos.Name,
					ParentKey:   dept.Key,
					IsUpdate:    isUpdate,
				})
			}
		}
	}

	for _, ag := range spec.Spec.Agents {
		isUpdate := existing.HasAgent(ag.Key)
		actions = append(actions, PlanAction{
			Kind:        "create_agent",
			Key:         ag.Key,
			DisplayName: ag.DisplayName,
			IsUpdate:    isUpdate,
		})
	}

	for _, team := range spec.Spec.Teams {
		isUpdate := existing.HasTeam(team.Key)
		actions = append(actions, PlanAction{
			Kind:        "create_team",
			Key:         team.Key,
			DisplayName: team.Name,
			IsUpdate:    isUpdate,
		})
	}

	return Plan{Actions: actions}
}

// FormatPlanTree renders the plan as an ASCII tree for CLI output.
func FormatPlanTree(plan Plan) string {
	var b strings.Builder
	b.WriteString("Import Plan:\n")
	for _, a := range plan.Actions {
		action := "CREATE"
		if a.IsUpdate {
			action = "UPDATE"
		}
		prefix := indent(a.Kind)
		b.WriteString(fmt.Sprintf("%s [%s] %s (%s)\n", prefix, action, a.DisplayName, a.Key))
	}
	return b.String()
}

func indent(kind string) string {
	switch kind {
	case "create_industry":
		return "├─ 🏭"
	case "create_department":
		return "│  ├─ 🏢"
	case "create_position":
		return "│  │  ├─ 👤"
	case "create_agent":
		return "├─ 🤖"
	case "create_team":
		return "├─ 👥"
	default:
		return "├─"
	}
}

// ExistingResources is an interface for looking up already-existing backend resources.
// The CLI applier implements this by fetching lists from the API before building the plan.
type ExistingResources interface {
	HasCategory(key string) bool
	HasAgent(key string) bool
	HasTeam(key string) bool
}

// EmptyExistingResources is a no-op implementation (used in dry-run when no API is available).
type EmptyExistingResources struct{}

func (EmptyExistingResources) HasCategory(key string) bool { return false }
func (EmptyExistingResources) HasAgent(key string) bool    { return false }
func (EmptyExistingResources) HasTeam(key string) bool     { return false }
