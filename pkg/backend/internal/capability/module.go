// Package capability is the Capability bounded Context: tools, skills,
// providers, models, plus the unified execution subsystem (tooldef, toolctx,
// middleware, executor, registry, backends, adkbridge, schema, telemetry).
//
// Skeleton state (P0): all four lifecycle stages are no-ops; the execution
// subsystem will be migrated incrementally per
// aranea/docs/0 main design.md §12.1.1 and §四.
package capability

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

// Module is the Capability Context shell; it implements module.Module.
type Module struct{}

// NewModule constructs the Capability module.
func NewModule() *Module { return &Module{} }

func (m *Module) Name() string                            { return "capability" }
func (m *Module) Version() string                         { return "0.1.0" }
func (m *Module) RegisterPorts(reg *contracts.Registry)   {}
func (m *Module) ResolvePorts(reg *contracts.Registry)    {}
func (m *Module) RegisterDriving(d module.DrivingRegistry) {}
func (m *Module) Start(ctx context.Context) error         { return nil }
func (m *Module) Shutdown(ctx context.Context) error      { return nil }
func (m *Module) OpenAPISpec() (json.RawMessage, error) {
	return json.RawMessage(openAPISpec), nil
}
