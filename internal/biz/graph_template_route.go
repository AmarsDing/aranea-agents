package biz

import "strings"

// GraphTemplateRoute is how a playbook stage uses an existing M53 graph
// without inventing a second engine. Empty GraphTemplateID stays on the
// ordinary Team Turn compile (mode → builtin template).
type GraphTemplateRoute struct {
	Mode            string
	LinkedGraphID   string
	CompileTemplate string
	Builtin         bool
}

// ResolveGraphTemplateRoute maps a playbook graph_template_id onto M53.
// Builtin names remap intra-team mode so PlanExecutor still owns the DAG.
// Any other id is treated as a persisted graph_definitions.id (load at
// compile; missing asset falls through to ordinary Team Turn).
func ResolveGraphTemplateRoute(mode, graphTemplateID string) GraphTemplateRoute {
	out := GraphTemplateRoute{Mode: strings.TrimSpace(mode)}
	id := strings.TrimSpace(graphTemplateID)
	if id == "" {
		return out
	}
	switch normalizeBuiltinGraphTemplate(id) {
	case "parallel_review":
		return GraphTemplateRoute{Mode: "parallel", CompileTemplate: "parallel_review", Builtin: true}
	case "dispatch":
		return GraphTemplateRoute{Mode: "coordinator", CompileTemplate: "dispatch", Builtin: true}
	case "review_loop":
		return GraphTemplateRoute{Mode: "critic_loop", CompileTemplate: "review_loop", Builtin: true}
	case "pipeline":
		return GraphTemplateRoute{Mode: "sequential", CompileTemplate: "pipeline", Builtin: true}
	default:
		out.LinkedGraphID = id
		return out
	}
}

// IsBuiltinGraphTemplateID reports whether id is a compile-time M53 template
// (pipeline / parallel_review / dispatch / review_loop) rather than an asset.
func IsBuiltinGraphTemplateID(id string) bool {
	return normalizeBuiltinGraphTemplate(id) != ""
}

func normalizeBuiltinGraphTemplate(id string) string {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "pipeline", "sequential":
		return "pipeline"
	case "parallel_review", "parallel":
		return "parallel_review"
	case "dispatch", "coordinator", "adaptive":
		return "dispatch"
	case "review_loop", "critic_loop":
		return "review_loop"
	default:
		return ""
	}
}
