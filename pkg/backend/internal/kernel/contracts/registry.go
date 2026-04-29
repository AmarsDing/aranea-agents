// Package contracts collects the cross-Context port interfaces that bounded
// contexts publish or consume. It is the only sanctioned channel for Context
// collaboration; no Context may import another Context's internal packages.
//
// Row #28: the legacy monolithic Store port (and list-query structs) lives
// in store.go while Registry remains the composition table for the four-stage
// module protocol. Finer-grained port types (AgentReader, ToolResolver, …) are
// added incrementally (see aranea/docs/0 main design.md §5 / §12.1.1 #28).
package contracts

// Registry is the cross-Context port table. Each Context registers its output
// ports during stage 1 (RegisterPorts) and resolves dependencies during
// stage 2 (ResolvePorts). The zero value is ready to use.
type Registry struct{}

// NewRegistry returns an empty registry. Provided for symmetry with the
// future stateful implementation; safe to call multiple times.
func NewRegistry() *Registry { return &Registry{} }
