package app

import (
	"encoding/json"

	"arenea/backend/internal/kernel/module"
)

// MergeOpenAPISpecs aggregates each Context's OpenAPI fragment into a single
// document. Stub state (P0): returns the first non-empty spec verbatim or a
// minimal empty doc. Real merge logic (paths union, schema namespacing,
// tag conflict detection per §6.1) lands with the openapi aggregation row.
func MergeOpenAPISpecs(modules []module.Module) (json.RawMessage, error) {
	for _, m := range modules {
		spec, err := m.OpenAPISpec()
		if err != nil {
			return nil, err
		}
		if len(spec) > 0 {
			return spec, nil
		}
	}
	return json.RawMessage(`{"openapi":"3.0.3","info":{"title":"Aranea","version":"0.1.0"},"paths":{}}`), nil
}
