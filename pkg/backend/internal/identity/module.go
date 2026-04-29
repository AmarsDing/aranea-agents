// Package identity is the Identity bounded Context: users, teams, workspaces,
// roles, API keys, and quota. The Module shell is the only externally
// exposed type; concrete logic lives in domain/, application/, ports/, and
// adapters/.
//
// Skeleton state (P0): all four lifecycle stages are no-ops. Real ports and
// HTTP routes will land via the row-level migrations defined in
// aranea/docs/0 main design.md §12.1.1.
package identity

import (
	"context"
	_ "embed"
	"encoding/json"

	"arenea/backend/internal/kernel/contracts"
	"arenea/backend/internal/kernel/module"
)

//go:embed openapi.json
var openAPISpec []byte

// Compile-time assertion that *Module satisfies module.Module.
var _ module.Module = (*Module)(nil)

// Module is the Identity Context shell; it implements module.Module.
type Module struct{}

// NewModule constructs the Identity module. Dependencies (DB pool, clock,
// audit writer) will be added as ports migrate.
func NewModule() *Module { return &Module{} }

// Name returns the stable Context identifier.
func (m *Module) Name() string { return "identity" }

// Version returns the Context contract version.
func (m *Module) Version() string { return "0.1.0" }

// RegisterPorts publishes Identity's output ports (stage 1).
func (m *Module) RegisterPorts(reg *contracts.Registry) {}

// ResolvePorts reads Identity's dependencies from the registry (stage 2).
func (m *Module) ResolvePorts(reg *contracts.Registry) {}

// RegisterDriving mounts HTTP/CLI routes (stage 3).
func (m *Module) RegisterDriving(d module.DrivingRegistry) {}

// Start boots background workers (stage 4).
func (m *Module) Start(ctx context.Context) error { return nil }

// Shutdown stops background workers; idempotent.
func (m *Module) Shutdown(ctx context.Context) error { return nil }

// OpenAPISpec returns the embedded OpenAPI fragment for this Context.
func (m *Module) OpenAPISpec() (json.RawMessage, error) {
	return json.RawMessage(openAPISpec), nil
}
