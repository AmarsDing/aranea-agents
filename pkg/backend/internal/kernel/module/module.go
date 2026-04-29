// Package module defines the Module + DrivingRegistry contract that every
// bounded Context (identity, catalog, capability, conversation, memory,
// operations) must implement. The four-stage lifecycle (RegisterPorts →
// ResolvePorts → RegisterDriving → Start/Shutdown) is the single composition
// surface used by the application launcher.
//
// See aranea/docs/0 main design.md §2 / §11 for the canonical specification.
package module

import (
	"context"
	"encoding/json"

	"github.com/go-chi/chi/v5"

	"arenea/backend/internal/kernel/contracts"
)

// DrivingRegistry is supplied by the application layer at stage 3
// (RegisterDriving). It abstracts the global /api/v1 chi mount and the
// OpenAPI fragment collector so that Context modules never reference the
// concrete router type.
type DrivingRegistry interface {
	// WithAPIV1 mounts routes under the global /api/v1 group. The provided
	// chi.Router is already scoped; Contexts may sub-route freely.
	WithAPIV1(fn func(r chi.Router))

	// RegisterOpenAPISpec records a Context's OpenAPI fragment for later
	// merging by the application layer. name is a stable identifier
	// (typically Module.Name()).
	RegisterOpenAPISpec(name string, spec json.RawMessage)
}

// Module is the per-Context shell. Each Context exposes exactly one
// implementation, returned via a NewModule constructor.
type Module interface {
	// Name returns a stable identifier (e.g. "identity", "capability").
	Name() string
	// Version returns the Context schema/contract version.
	Version() string

	// RegisterPorts (stage 1) publishes this Context's output port
	// implementations to the shared registry.
	RegisterPorts(reg *contracts.Registry)
	// ResolvePorts (stage 2) reads dependencies the Context needs from the
	// shared registry. All RegisterPorts calls have completed before this
	// stage runs, breaking cyclic resolution.
	ResolvePorts(reg *contracts.Registry)

	// RegisterDriving (stage 3) mounts HTTP/CLI/Cron driving adapters via
	// the supplied registry. Side effects only — long-running goroutines
	// belong in Start.
	RegisterDriving(d DrivingRegistry)

	// Start (stage 4) boots background workers (cron, watchers, event bus
	// subscribers). HTTP listeners are owned by the launcher.
	Start(ctx context.Context) error
	// Shutdown is idempotent and bounded by the caller's context deadline.
	Shutdown(ctx context.Context) error

	// OpenAPISpec returns this Context's OpenAPI fragment. May return
	// (nil, nil) when the Context exposes no HTTP surface.
	OpenAPISpec() (json.RawMessage, error)
}
