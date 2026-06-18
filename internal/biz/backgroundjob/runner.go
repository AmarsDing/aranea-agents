package backgroundjob

import "context"

// Runner executes a single BackgroundJob. Implementations register themselves
// with the worker via BackgroundJobRegistry and are dispatched by Kind.
//
// A Runner must be:
//   - idempotent: the worker may re-run a job after a crash or transient
//     failure (attempts < max_attempts), so Run must tolerate re-execution.
//   - context-aware: ctx is cancelled when the worker is shutting down;
//     long-running Runners should periodically check ctx.Err().
//   - self-contained: the Payload bytes carry all input needed; Runners
//     should not depend on external mutable state that may change between
//     enqueue and execution.
//
// Stability:evolving
type Runner interface {
	// Kind returns the runner kind this implementation handles. Must match
	// the Kind field on Jobs it processes.
	Kind() string

	// Run executes the job. Returning nil marks the job succeeded.
	// Returning an error marks the job failed; if attempts < max_attempts
	// the worker may re-enqueue it (Repo.MarkFailed does not auto-requeue;
	// re-enqueue is the caller's responsibility per the design doc).
	Run(ctx context.Context, job Job) error
}

// Registry is the port interface for registering and looking up Runners by
// Kind. The worker uses it to dispatch claimed jobs.
//
// Stability:evolving
type Registry interface {
	// Register adds a Runner to the registry. Panics on duplicate Kind
	// (programming error — should be caught at startup, not at runtime).
	Register(r Runner)

	// Lookup returns the Runner for the given Kind, or nil if none registered.
	Lookup(kind string) Runner

	// Kinds returns all registered kinds. Used by the worker to claim only
	// jobs it has runners for.
	Kinds() []string
}

// NewRegistry returns a default in-memory Registry implementation.
func NewRegistry() Registry {
	return &registryImpl{runners: make(map[string]Runner)}
}

type registryImpl struct {
	runners map[string]Runner
}

func (r *registryImpl) Register(run Runner) {
	if run == nil {
		return
	}
	kind := run.Kind()
	if _, exists := r.runners[kind]; exists {
		panic("backgroundjob: duplicate runner kind: " + kind)
	}
	r.runners[kind] = run
}

func (r *registryImpl) Lookup(kind string) Runner {
	return r.runners[kind]
}

func (r *registryImpl) Kinds() []string {
	kinds := make([]string, 0, len(r.runners))
	for k := range r.runners {
		kinds = append(kinds, k)
	}
	return kinds
}
