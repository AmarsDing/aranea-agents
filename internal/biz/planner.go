package biz

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
)

const (
	PlannerKindBuiltin = "builtin"
	PlannerKindReact   = "react"
	PlannerKindA2UI    = "a2ui"
)

var (
	builtinConfigKeys = map[string]struct{}{
		"reasoning_effort": {},
		"thinking_enabled": {},
		"thinking_tokens":  {},
	}
	validReasoningEfforts = map[string]struct{}{
		"low": {}, "medium": {}, "high": {}, "max": {},
	}
	a2uiConfigKeys = map[string]struct{}{
		"instruction": {},
		"server_to_client_with_standard_catalog_schema_json": {},
		"client_to_server_schema_json":                       {},
		"client_capabilities_schema_json":                    {},
		"server_to_client_only_schema_json":                  {},
		"standard_catalog_definition_json":                   {},
		"catalog_description_schema_json":                    {},
	}
)

// ValidPlannerKinds lists allowed AgentRuntimeSettings.PlannerKind values (empty = legacy dialog-mode).
func ValidPlannerKinds() []string {
	return []string{"", PlannerKindBuiltin, PlannerKindReact, PlannerKindA2UI}
}

// ValidatePlannerKind rejects unknown planner identifiers at the persistence boundary.
func ValidatePlannerKind(raw string) error {
	k := strings.ToLower(strings.TrimSpace(raw))
	for _, allowed := range ValidPlannerKinds() {
		if k == allowed {
			return nil
		}
	}
	return errors.BadRequest("AGENT", fmt.Sprintf("invalid planner_kind %q; allowed: (empty), builtin, react, a2ui", raw))
}

// ValidatePlannerConfigJSON validates JSON object shape and kind-specific fields.
func ValidatePlannerConfigJSON(plannerKind, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return errors.BadRequest("AGENT", "planner_config_json must be a JSON object")
	}
	kind := strings.ToLower(strings.TrimSpace(plannerKind))
	switch kind {
	case PlannerKindReact:
		if len(obj) > 0 {
			return errors.BadRequest("AGENT", "planner_config_json must be {} for react planner")
		}
	case PlannerKindBuiltin:
		return validatePlannerConfigKeys(obj, builtinConfigKeys, "builtin")
	case PlannerKindA2UI:
		return validatePlannerConfigKeys(obj, a2uiConfigKeys, "a2ui")
	default:
		if len(obj) > 0 {
			return errors.BadRequest("AGENT", "planner_config_json requires planner_kind (builtin, react, or a2ui); legacy empty kind only allows {}")
		}
	}
	return validatePlannerConfigValueTypes(obj)
}

func validatePlannerConfigKeys(obj map[string]json.RawMessage, allowed map[string]struct{}, label string) error {
	for key := range obj {
		if _, ok := allowed[key]; !ok {
			return errors.BadRequest("AGENT", fmt.Sprintf("unknown %s planner_config_json field %q", label, key))
		}
	}
	return validatePlannerConfigValueTypes(obj)
}

func validatePlannerConfigValueTypes(obj map[string]json.RawMessage) error {
	for key, raw := range obj {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return errors.BadRequest("AGENT", fmt.Sprintf("invalid planner_config_json field %q: %v", key, err))
		}
		switch key {
		case "thinking_enabled":
			if _, ok := v.(bool); !ok {
				return errors.BadRequest("AGENT", "planner_config_json.thinking_enabled must be a boolean")
			}
		case "thinking_tokens":
			if _, ok := v.(float64); !ok {
				return errors.BadRequest("AGENT", "planner_config_json.thinking_tokens must be a number")
			}
		case "reasoning_effort":
			s, ok := v.(string)
			if !ok {
				return errors.BadRequest("AGENT", "planner_config_json.reasoning_effort must be a string")
			}
			s = strings.ToLower(strings.TrimSpace(s))
			if s != "" {
				if _, allowed := validReasoningEfforts[s]; !allowed {
					return errors.BadRequest("AGENT", "planner_config_json.reasoning_effort must be one of: low, medium, high, max")
				}
			}
		default:
			if s, ok := v.(string); !ok {
				return errors.BadRequest("AGENT", fmt.Sprintf("planner_config_json.%s must be a string", key))
			} else if strings.TrimSpace(s) == "" && len(obj) > 0 {
				// allow empty strings for optional schema overrides
				_ = s
			}
		}
	}
	return nil
}
