package graph

import (
	"context"
	"fmt"
	"sync"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

type NodeFuncFactory func() (trpcgraph.NodeFunc, error)

type CondFuncFactory func() (any, error)

type Registry struct {
	mu        sync.RWMutex
	nodeFuncs map[string]NodeFuncFactory
	condFuncs map[string]CondFuncFactory
}

func NewRegistry() *Registry {
	return &Registry{
		nodeFuncs: make(map[string]NodeFuncFactory),
		condFuncs: make(map[string]CondFuncFactory),
	}
}

func (r *Registry) RegisterNodeFunc(name string, factory NodeFuncFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodeFuncs[name] = factory
}

func (r *Registry) RegisterNodeFuncInstance(name string, fn trpcgraph.NodeFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodeFuncs[name] = func() (trpcgraph.NodeFunc, error) { return fn, nil }
}

func (r *Registry) GetNodeFunc(name string) (trpcgraph.NodeFunc, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.nodeFuncs[name]
	if !ok {
		return nil, kerrors.NotFound("GRAPH", fmt.Sprintf("graph registry: node func %q not found", name))
	}
	return factory()
}

func (r *Registry) RegisterCondFunc(name string, factory CondFuncFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.condFuncs[name] = factory
}

func (r *Registry) RegisterCondFuncInstance(name string, fn any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.condFuncs[name] = func() (any, error) { return fn, nil }
}

func (r *Registry) GetCondFunc(name string) (any, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.condFuncs[name]
	if !ok {
		return nil, kerrors.NotFound("GRAPH", fmt.Sprintf("graph registry: cond func %q not found", name))
	}
	return factory()
}

func (r *Registry) ListNodeFuncs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.nodeFuncs))
	for name := range r.nodeFuncs {
		names = append(names, name)
	}
	return names
}

func (r *Registry) ListCondFuncs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.condFuncs))
	for name := range r.condFuncs {
		names = append(names, name)
	}
	return names
}

func (r *Registry) ResolveNodeDef(n NodeDef) (NodeDef, error) {
	if n.Func != nil {
		return n, nil
	}
	if n.FuncRef == "" {
		return n, kerrors.BadRequest("GRAPH", fmt.Sprintf("graph registry: node %q has no Func and no FuncRef", n.ID))
	}
	fn, err := r.GetNodeFunc(n.FuncRef)
	if err != nil {
		return n, kerrors.InternalServer("GRAPH", fmt.Sprintf("graph registry: node %q: %v", n.ID, err))
	}
	resolved := n
	resolved.Func = fn
	return resolved, nil
}

func (r *Registry) ResolveConditionalEdgeDef(ce ConditionalEdgeDef) (ConditionalEdgeDef, error) {
	if ce.CondFunc != nil {
		return ce, nil
	}
	if ce.CondFuncRef == "" {
		return ce, kerrors.BadRequest("GRAPH", fmt.Sprintf("graph registry: conditional edge from %q has no CondFunc and no CondFuncRef", ce.From))
	}
	fn, err := r.GetCondFunc(ce.CondFuncRef)
	if err != nil {
		return ce, kerrors.InternalServer("GRAPH", fmt.Sprintf("graph registry: conditional edge from %q: %v", ce.From, err))
	}
	resolved := ce
	resolved.CondFunc = fn
	return resolved, nil
}

func (r *Registry) ResolveBuildConfig(cfg GraphBuildConfig) (GraphBuildConfig, error) {
	resolved := cfg

	for i, n := range resolved.Nodes {
		resolvedNode, err := r.ResolveNodeDef(n)
		if err != nil {
			return cfg, err
		}
		resolved.Nodes[i] = resolvedNode
	}

	for i, ce := range resolved.ConditionalEdges {
		resolvedEdge, err := r.ResolveConditionalEdgeDef(ce)
		if err != nil {
			return cfg, err
		}
		resolved.ConditionalEdges[i] = resolvedEdge
	}

	for i, sub := range resolved.Subgraphs {
		resolvedSub, err := r.ResolveBuildConfig(sub.BuildConfig)
		if err != nil {
			return cfg, kerrors.InternalServer("GRAPH", fmt.Sprintf("graph registry: subgraph %q: %v", sub.ID, err))
		}
		resolved.Subgraphs[i].BuildConfig = resolvedSub
	}

	return resolved, nil
}

func PassthroughNodeFunc(id string) trpcgraph.NodeFunc {
	return func(ctx context.Context, state trpcgraph.State) (any, error) {
		return state, nil
	}
}
