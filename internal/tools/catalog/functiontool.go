package catalog

import (
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// FunctionToolConfig is the functiontool construction config (ADK-aligned).
type FunctionToolConfig = functiontool.Config

// NewFunctionTool wraps any Go handler as an ADK callable tool (same as functiontool.New).
func NewFunctionTool[TArgs, TResults any](cfg FunctionToolConfig, h functiontool.Func[TArgs, TResults]) (tool.Tool, error) {
	return functiontool.New(cfg, h)
}

// FilterToolset see tool.FilterToolset (ADK framework helper).
func FilterToolset(ts tool.Toolset, predicate tool.Predicate) tool.Toolset {
	return tool.FilterToolset(ts, predicate)
}

// AllowedToolsPredicate see tool.AllowedToolsPredicate.
func AllowedToolsPredicate(names []string) tool.Predicate {
	return tool.AllowedToolsPredicate(names)
}
