package backends

import (
	"fmt"
	"strings"
)

type Base struct {
	Key          string
	Label        string
	Desc         string
	ToolCategory string
	InSchema     map[string]any
	OutSchema    map[string]any
	Required     []string
}

func (b Base) Name() string                 { return b.Key }
func (b Base) DisplayName() string          { return firstNonEmpty(b.Label, b.Key) }
func (b Base) Description() string          { return b.Desc }
func (b Base) Category() string             { return b.ToolCategory }
func (b Base) InputSchema() map[string]any  { return cloneMap(b.InSchema) }
func (b Base) OutputSchema() map[string]any { return cloneMap(b.OutSchema) }

func (b Base) Validate(params map[string]any) error {
	for _, key := range b.Required {
		value, ok := params[key]
		if !ok || strings.TrimSpace(fmt.Sprint(value)) == "" {
			return fmt.Errorf("%s is required", key)
		}
	}
	return nil
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	out := map[string]any{}
	for key, value := range in {
		out[key] = value
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringParam(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(params[key]))
}
