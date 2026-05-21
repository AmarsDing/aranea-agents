package service

import (
	a2apkg "aranea-agents/internal/a2a"
	a2atrpc "aranea-agents/internal/a2a/trpc"
)

// A2APublicBaseReloader applies system-settings changes to the live A2A public URL.
type A2APublicBaseReloader struct {
	store     *a2apkg.PublicBaseURLStore
	endpoints *a2atrpc.EndpointRegistry
	input     a2apkg.PublicBaseURLInput
}

// NewA2APublicBaseReloader constructs a reloader bound to the shared store and endpoint registry.
func NewA2APublicBaseReloader(
	store *a2apkg.PublicBaseURLStore,
	endpoints *a2atrpc.EndpointRegistry,
	input a2apkg.PublicBaseURLInput,
) *A2APublicBaseReloader {
	return &A2APublicBaseReloader{store: store, endpoints: endpoints, input: input}
}

// Reload recomputes the effective URL from the given DB value and invalidates cached handlers when it changes.
func (r *A2APublicBaseReloader) Reload(dbURL string) {
	if r == nil || r.store == nil {
		return
	}
	in := r.input
	in.DBURL = dbURL
	next := a2apkg.ResolvePublicBaseURL(in)
	prev := r.store.Get()
	r.store.Set(next)
	if r.endpoints != nil && prev.URL != next.URL {
		r.endpoints.InvalidateAll()
	}
}
