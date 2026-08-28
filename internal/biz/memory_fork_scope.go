package biz

import "context"

type forkMemoryPrivateKey struct{}

// WithForkMemoryPrivate marks the ctx so memory writes from this turn stay in
// the forked session's private domain instead of the shared user scope.
// Fork isolates message history but previously reused the user-scoped L3
// write path (S13). Shared writes require an explicit future promote.
func WithForkMemoryPrivate(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, forkMemoryPrivateKey{}, true)
}

// ForkMemoryPrivateFromContext reports whether memory writes must use the
// session-private scope (fork sessions).
func ForkMemoryPrivateFromContext(ctx context.Context) bool {
	v, _ := ctx.Value(forkMemoryPrivateKey{}).(bool)
	return v
}
