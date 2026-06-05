package biz

import "context"

// FunctionResolver validates function references during team graph compilation.
// Runtime resolution of actual callable functions is handled by
// internal/graph/trpc.FunctionResolver.
type FunctionResolver interface {
	// Resolve validates that a function reference can be resolved.
	// Returns nil if valid, or an error describing why not.
	Resolve(ctx context.Context, funcRef string) error
}

// DefaultFunctionResolver is a no-op implementation that accepts all function references.
type DefaultFunctionResolver struct{}

func NewDefaultFunctionResolver() *DefaultFunctionResolver {
	return &DefaultFunctionResolver{}
}

func (r *DefaultFunctionResolver) Resolve(_ context.Context, _ string) error {
	return nil
}
