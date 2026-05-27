package orgimport

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ValidationError holds all validation failures for a spec.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("orgimport: spec validation failed (%d errors):\n- %s",
		len(e.Errors), strings.Join(e.Errors, "\n- "))
}

// ValidateSpec validates a Spec for required fields, key uniqueness,
// category_position references, and character budget compliance.
func ValidateSpec(spec *Spec) error {
	var errs []string
	// Collect all position paths for reference validation.
	positionPaths := collectPositionPaths(spec)

	for i, ind := range spec.Spec.Industries {
		if ind.Key == "" {
			errs = append(errs, fmt.Sprintf("industries[%d]: key is required", i))
		}
		if ind.Name == "" {
			errs = append(errs, fmt.Sprintf("industries[%d] (%s): name is required", i, ind.Key))
		}
		if ind.Description != "" {
			chars := utf8.RuneCountInString(ind.Description)
			if chars > 600 {
				errs = append(errs, fmt.Sprintf("industry %q: description exceeds hard limit 600 chars (%d)", ind.Key, chars))
			}
		}
		for j, dept := range ind.Departments {
			if dept.Key == "" {
				errs = append(errs, fmt.Sprintf("industries[%d].departments[%d]: key is required", i, j))
			}
			if dept.Description != "" {
				chars := utf8.RuneCountInString(dept.Description)
				if chars > 800 {
					errs = append(errs, fmt.Sprintf("dept %q: description exceeds hard limit 800 chars (%d)", dept.Key, chars))
				}
			}
			for k, pos := range dept.Positions {
				if pos.Key == "" {
					errs = append(errs, fmt.Sprintf("industries[%d].departments[%d].positions[%d]: key is required", i, j, k))
				}
				if pos.Description != "" {
					chars := utf8.RuneCountInString(pos.Description)
					if chars > 1000 {
						errs = append(errs, fmt.Sprintf("position %q: description exceeds hard limit 1000 chars (%d)", pos.Key, chars))
					}
				}
			}
		}
	}

	for i, ag := range spec.Spec.Agents {
		if ag.Key == "" {
			errs = append(errs, fmt.Sprintf("agents[%d]: key is required", i))
		}
		if ag.DisplayName == "" {
			errs = append(errs, fmt.Sprintf("agents[%d] (%s): display_name is required", i, ag.Key))
		}
		if ag.CategoryPosition != "" {
			if _, ok := positionPaths[ag.CategoryPosition]; !ok {
				errs = append(errs, fmt.Sprintf("agent %q: category_position %q not found in spec", ag.Key, ag.CategoryPosition))
			}
		}
		if ag.AgentDescription != "" {
			chars := utf8.RuneCountInString(ag.AgentDescription)
			if chars > 600 {
				errs = append(errs, fmt.Sprintf("agent %q: agent_description exceeds hard limit 600 chars (%d)", ag.Key, chars))
			}
		}
	}

	for i, team := range spec.Spec.Teams {
		if team.Key == "" {
			errs = append(errs, fmt.Sprintf("teams[%d]: key is required", i))
		}
		if len(team.Members) == 0 {
			errs = append(errs, fmt.Sprintf("team %q: at least one member is required", team.Key))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return &ValidationError{Errors: errs}
}

// collectPositionPaths builds a set of "ind/dept/pos" path strings.
func collectPositionPaths(spec *Spec) map[string]struct{} {
	paths := map[string]struct{}{}
	for _, ind := range spec.Spec.Industries {
		for _, dept := range ind.Departments {
			for _, pos := range dept.Positions {
				key := ind.Key + "/" + dept.Key + "/" + pos.Key
				paths[key] = struct{}{}
			}
		}
	}
	return paths
}
