package trpc

import (
	"context"
	"net/http"
	"strings"
	"sync"

	a2apkg "aranea-agents/internal/a2a"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
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
	lg        loggateway.Logger
}

// NewEndpointRegistry constructs a registry backed by a hot-reloadable public base URL store.
func NewEndpointRegistry(builder EndpointBuilder, a2aUC *biz.A2AUsecase, baseStore *a2apkg.PublicBaseURLStore, lg loggateway.Logger) *EndpointRegistry {
	return &EndpointRegistry{
		cache:     make(map[string]cachedEndpoint),
		builder:   builder,
		a2aUC:     a2aUC,
		baseStore: baseStore,
		lg:        lg,
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
	agentID := agentIDFromPublicPath(req.URL.Path)
	if agentID == "" {
		http.NotFound(w, req)
		return
	}
	card, err := r.a2aUC.GetAgentCard(req.Context(), agentID)
	if err != nil || !card.Enabled {
		r.lg.Warn("A2A endpoint agent not found or disabled", loggateway.StepID("a2a.endpoint.agent_disabled"), loggateway.Str("agent_id", agentID))
		http.NotFound(w, req)
		return
	}
	handler, err := r.handlerFor(req.Context(), agentID)
	if err != nil {
		r.lg.Error("A2A endpoint handler build failed", loggateway.StepID("a2a.endpoint.build_fail"), loggateway.Str("agent_id", agentID), loggateway.Err(err))
		http.Error(w, "A2A endpoint unavailable", http.StatusServiceUnavailable)
		return
	}
	// 保留完整路径：下游 trpc-a2a-go server 按 agentCard.URL 提取的 basePath
	// 注册路由（basePath + "/.well-known/agent-card.json" 等），若在此剥离
	// /v1/a2a/public/{agent_id} 前缀会导致双重前缀失配、全部 404。
	r.lg.Info("A2A endpoint dispatch", loggateway.StepID("a2a.endpoint.dispatch"), loggateway.Str("agent_id", agentID), loggateway.Str("path", req.URL.Path))
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

// agentIDFromPublicPath extracts the agent id segment from /v1/a2a/public/{agent_id}/*.
func agentIDFromPublicPath(path string) string {
	path = strings.TrimPrefix(path, PublicPathPrefix)
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return ""
	}
	parts := strings.SplitN(path, "/", 2)
	return strings.TrimSpace(parts[0])
}
