package schema

import (
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"
)

func JSONSchemaOf[T any]() map[string]any {
	s, err := jsonschema.For[T](nil)
	if err != nil {
		return EmptyObject()
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return EmptyObject()
	}
	var out map[string]any
	if err = json.Unmarshal(raw, &out); err != nil {
		return EmptyObject()
	}
	if len(out) == 0 {
		return EmptyObject()
	}
	return out
}

func EmptyObject() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}
