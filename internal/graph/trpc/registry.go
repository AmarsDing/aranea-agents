package graph

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

const maxSubgraphResolveDepth = 10

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
		return nil, apierror.NotFound(apierror.DomainGraph, fmt.Sprintf("graph registry: node func %q not found", name))
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

// StateRouteCondFuncPrefix 是内置条件函数：`state.route:<字段>` 取该状态字段的值作为
// 分支键（对应 PathMap 的 key）。不进注册表，任何 Registry 都能解析。
const StateRouteCondFuncPrefix = "state.route:"

func stateRouteCondFunc(field string) trpcgraph.ConditionalFunc {
	return func(_ context.Context, state trpcgraph.State) (string, error) {
		v, ok := state[field]
		key := ""
		if ok && v != nil {
			key = strings.TrimSpace(fmt.Sprint(v))
		}
		if key == "" {
			return "", apierror.BadRequest(apierror.DomainGraph,
				fmt.Sprintf("graph: 状态字段 %q 为空，无法选择分支（执行时在初始状态里给它一个分支名）", field))
		}
		return key, nil
	}
}

func (r *Registry) GetCondFunc(name string) (any, error) {
	if field, ok := strings.CutPrefix(name, StateRouteCondFuncPrefix); ok && strings.TrimSpace(field) != "" {
		return stateRouteCondFunc(strings.TrimSpace(field)), nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.condFuncs[name]
	if !ok {
		return nil, apierror.NotFound(apierror.DomainGraph, fmt.Sprintf("graph registry: cond func %q not found", name))
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

func (r *Registry) ResolveNodeDef(n biz.NodeDef) (NodeDef, error) {
	if n.FuncRef == "" {
		return NodeDef{NodeDef: n}, nil
	}
	// biz.SkipNodeFuncRef 是内置 sentinel（由 ApplySkipNodeSemantics 注入到
	// verification/skip 节点），不会注册进 Registry。与 node_wiring.go 的
	// 特判对齐：直接返回 SkipNodeFunc，避免查表报 NOT_FOUND。
	if n.FuncRef == biz.SkipNodeFuncRef {
		return NodeDef{NodeDef: n, Func: SkipNodeFunc(n.ID)}, nil
	}
	fn, err := r.GetNodeFunc(n.FuncRef)
	if err != nil {
		return NodeDef{NodeDef: n}, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("graph registry: node %q: %v", n.ID, err))
	}
	return NodeDef{NodeDef: n, Func: fn}, nil
}

func (r *Registry) ResolveConditionalEdgeDef(ce biz.ConditionalEdgeDef) (ConditionalEdgeDef, error) {
	if ce.CondFuncRef == "" {
		return ConditionalEdgeDef{ConditionalEdgeDef: ce}, nil
	}
	fn, err := r.GetCondFunc(ce.CondFuncRef)
	if err != nil {
		return ConditionalEdgeDef{ConditionalEdgeDef: ce}, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("graph registry: conditional edge from %q: %v", ce.From, err))
	}
	return ConditionalEdgeDef{ConditionalEdgeDef: ce, CondFunc: fn}, nil
}

func (r *Registry) ResolveBuildConfig(cfg GraphBuildConfig) (*resolvedBuildConfig, error) {
	return r.resolveBuildConfig(cfg, 0)
}

func (r *Registry) resolveBuildConfig(cfg GraphBuildConfig, depth int) (*resolvedBuildConfig, error) {
	if depth >= maxSubgraphResolveDepth {
		return nil, apierror.BadRequest(apierror.DomainGraph, fmt.Sprintf("graph registry: subgraph nesting depth exceeds limit (%d)", maxSubgraphResolveDepth))
	}
	rbc := &resolvedBuildConfig{
		cfg:       cfg,
		nodes:     make([]NodeDef, len(cfg.Nodes)),
		condEdges: make([]ConditionalEdgeDef, len(cfg.ConditionalEdges)),
		subs:      make([]SubgraphDef, len(cfg.Subgraphs)),
		subRbcs:   make(map[string]*resolvedBuildConfig),
	}

	for i, n := range cfg.Nodes {
		resolvedNode, err := r.ResolveNodeDef(n)
		if err != nil {
			return nil, err
		}
		rbc.nodes[i] = resolvedNode
	}

	for i, ce := range cfg.ConditionalEdges {
		resolvedEdge, err := r.ResolveConditionalEdgeDef(ce)
		if err != nil {
			return nil, err
		}
		rbc.condEdges[i] = resolvedEdge
	}

	for i, s := range cfg.Subgraphs {
		rbc.subs[i] = SubgraphDef{SubgraphDef: s}
		subRbc, err := r.resolveBuildConfig(s.BuildConfig, depth+1)
		if err != nil {
			return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("graph registry: subgraph %q: %v", s.ID, err))
		}
		rbc.subRbcs[s.ID] = subRbc
	}

	return rbc, nil
}

func PassthroughNodeFunc(id string) trpcgraph.NodeFunc {
	return func(ctx context.Context, state trpcgraph.State) (any, error) {
		return state, nil
	}
}
