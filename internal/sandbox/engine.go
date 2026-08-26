package sandbox

import (
	"context"
	"io"
)

// Handle identifies an engine-native sandbox instance (e.g. a container name).
type Handle struct {
	ID        string // engine-native id (docker container name)
	SandboxID string // platform sandbox id (matches label aranea.sandbox.id)
	// EgressNet is the per-sandbox egress network name (review 2026-08-26 #3),
	// set by Create only for NetworkEgress profiles so Destroy can reclaim it
	// without a profile lookup. Empty for none/full.
	EgressNet string
}

// Engine abstracts the isolation backend (ADR-82-1). DockerEngine is the P0
// implementation; runsc / Firecracker / E2B are reserved second engines.
type Engine interface {
	// Create provisions a new running instance with the given labels.
	Create(ctx context.Context, sandboxID string, p Profile, labels map[string]string) (Handle, error)
	// Exec runs a command inside the instance.
	Exec(ctx context.Context, h Handle, spec ExecSpec) (ExecResult, error)
	// CopyFrom reads path from the instance as a tar stream.
	CopyFrom(ctx context.Context, h Handle, path string) (io.ReadCloser, error)
	// Destroy removes the instance and all its resources (idempotent).
	Destroy(ctx context.Context, h Handle) error
	// ListByLabels returns live instances matching ALL given labels (reconcile).
	ListByLabels(ctx context.Context, labels map[string]string) ([]Handle, error)
}

// NetworkReaper is an optional Engine capability (review 2026-08-26 #3):
// egress sandboxes each get a dedicated internal network (shared only with
// the proxy), an engine-side resource the registry does not track. The
// startup reconcile asks engines implementing this interface to sweep
// labeled orphan networks left by a previous process lifetime.
type NetworkReaper interface {
	// ReapOrphanNetworks removes every network matching ALL given labels and
	// returns the count reaped.
	ReapOrphanNetworks(ctx context.Context, labels map[string]string) (int, error)
}
