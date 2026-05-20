package trpc

import (
	"context"
	"net/http"
	"strings"
	"sync"

	a2apkg "aranea-agents/internal/a2a"
	"aranea-agents/internal/biz"
)

const PublicPathPrefix = "/v1/a2a/public/"

// EndpointBuilder builds a per-agent A2A HTTP handler.
type EndpointBuilder interface {
	BuildHandler(ctx context.Context, agentID, publicBaseURL string) (http.Handler, func(), error)
}

type cachedEndpoint struct {
	handler http.Handler
	closeFn func()
	baseURL string
}

// EndpointRegistry routes /v1/a2a/public/{agent_id}/* to enabled endpoint agents.
type EndpointRegistry struct {
	mu        sync.RWMutex
	cache     map[string]cachedEndpoint
	builder   EndpointBuilder
	a2aUC     *biz.A2AUsecase
	baseStore *a2apkg.PublicBaseURLStore
}

// NewEndpointRegistry constructs a registry backed by a hot-reloadable public base URL store.
func NewEndpointRegistry(builder EndpointBuilder, a2aUC *biz.A2AUsecase, baseStore *a2apkg.PublicBaseURLStore) *EndpointRegistry {
	return &EndpointRegistry{
		cache:     make(map[string]cachedEndpoint),
		builder:   builder,
		a2aUC:     a2aUC,
		baseStore: baseStore,
	}
}

// BaseURL returns the configured public origin for A2A endpoints (no trailing slash).
func (r *EndpointRegistry) BaseURL() string {
	if r == nil || r.baseStore == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(r.baseStore.Get().URL), "/")
}

// Invalidate drops a cached handler (call after AgentCard update/disable).
func (r *EndpointRegistry) Invalidate(agentID string) {
	if r == nil {
		return
	}
	agentID = strings.TrimSpace(agentID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if item, ok := r.cache[agentID]; ok {
		if item.closeFn != nil {
			item.closeFn()
		}
		delete(r.cache, agentID)
	}
}

// InvalidateAll drops all cached handlers (call after public base URL changes).
func (r *EndpointRegistry) InvalidateAll() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for agentID, item := range r.cache {
		if item.closeFn != nil {
			item.closeFn()
		}
		delete(r.cache, agentID)
	}
}

// ServeHTTP implements http.Handler.
func (r *EndpointRegistry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if r == nil || r.builder == nil {
		http.NotFound(w, req)
		return
	}
	agentID, suffix := splitPublicPath(req.URL.Path)
	if agentID == "" {
		http.NotFound(w, req)
		return
	}
	card, err := r.a2aUC.GetAgentCard(req.Context(), agentID)
	if err != nil || !card.Enabled {
		http.NotFound(w, req)
		return
	}
	handler, err := r.handlerFor(req.Context(), agentID)
	if err != nil {
		http.Error(w, "A2A endpoint unavailable", http.StatusServiceUnavailable)
		return
	}
	req.URL.Path = suffix
	if req.URL.Path == "" {
		req.URL.Path = "/"
	}
	handler.ServeHTTP(w, req)
}

func (r *EndpointRegistry) handlerFor(ctx context.Context, agentID string) (http.Handler, error) {
	baseURL := r.BaseURL()
	r.mu.RLock()
	if item, ok := r.cache[agentID]; ok && item.handler != nil && item.baseURL == baseURL {
		r.mu.RUnlock()
		return item.handler, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if item, ok := r.cache[agentID]; ok && item.handler != nil && item.baseURL == baseURL {
		return item.handler, nil
	}
	if item, ok := r.cache[agentID]; ok {
		if item.closeFn != nil {
			item.closeFn()
		}
		delete(r.cache, agentID)
	}
	publicURL := baseURL + "/" + agentID
	handler, closeFn, err := r.builder.BuildHandler(ctx, agentID, publicURL)
	if err != nil {
		return nil, err
	}
	r.cache[agentID] = cachedEndpoint{handler: handler, closeFn: closeFn, baseURL: baseURL}
	return handler, nil
}

func splitPublicPath(path string) (agentID, suffix string) {
	path = strings.TrimPrefix(path, PublicPathPrefix)
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return "", "/"
	}
	parts := strings.SplitN(path, "/", 2)
	agentID = strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return agentID, "/"
	}
	suffix = "/" + parts[1]
	return agentID, suffix
}
