// Package conversation is the Conversation bounded Context: sessions, turns,
// streaming, and the dialog orchestrator. It is the only Context permitted
// to import the embedded ADK runtime (via its private adapters/adkruntime
// sub-package).
//
// Skeleton state (P0): all four lifecycle stages are no-ops; row-level
// migrations (see aranea/docs/0 main design.md §12.1.1) will move logic in.
package conversation

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

// Module is the Conversation Context shell; it implements module.Module.
type Module struct{}

// NewModule constructs the Conversation module.
func NewModule() *Module { return &Module{} }

func (m *Module) Name() string                            { return "conversation" }
func (m *Module) Version() string                         { return "0.1.0" }
func (m *Module) RegisterPorts(reg *contracts.Registry)   {}
func (m *Module) ResolvePorts(reg *contracts.Registry)    {}
func (m *Module) RegisterDriving(d module.DrivingRegistry) {}
func (m *Module) Start(ctx context.Context) error         { return nil }
func (m *Module) Shutdown(ctx context.Context) error      { return nil }
func (m *Module) OpenAPISpec() (json.RawMessage, error) {
	return json.RawMessage(openAPISpec), nil
}
