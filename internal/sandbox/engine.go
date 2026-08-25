package sandbox

import (
	"context"
	"io"
)

// Handle identifies an engine-native sandbox instance (e.g. a container name).
type Handle struct {
	ID        string // engine-native id (docker container name)
	SandboxID string // platform sandbox id (matches label aranea.sandbox.id)
}

// Engine abstracts the isolation backend (ADR-82-1). DockerEngine is the P0
// implementation; runsc / Firecracker / E2B are reserved second engines.
type Engine interface {
	// Create provisions a new running instance with the given labels.
	Create(ctx context.Context, sandboxID string, p Profile, labels map[string]string) (Handle, error)
	// Exec runs a command inside the instance.
	Exec(ctx context.Context, h Handle, spec ExecSpec) (ExecResult, error)
	// CopyTo writes r (a tar stream) into the instance at path.
	CopyTo(ctx context.Context, h Handle, path string, r io.Reader) error
	// CopyFrom reads path from the instance as a tar stream.
	CopyFrom(ctx context.Context, h Handle, path string) (io.ReadCloser, error)
	// Destroy removes the instance and all its resources (idempotent).
	Destroy(ctx context.Context, h Handle) error
	// ListByLabels returns live instances matching ALL given labels (reconcile).
	ListByLabels(ctx context.Context, labels map[string]string) ([]Handle, error)
}
