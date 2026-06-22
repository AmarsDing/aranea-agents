package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

type ValidationErrorCode string

const (
	ValidationErrEmptyGraph         ValidationErrorCode = "empty_graph"
	ValidationErrNoEntryPoint       ValidationErrorCode = "no_entry_point"
	ValidationErrUnreachableNode    ValidationErrorCode = "unreachable_node"
	ValidationErrOrphanNode         ValidationErrorCode = "orphan_node"
	ValidationErrAgentNotFound      ValidationErrorCode = "agent_not_found"
	ValidationErrFuncRefNotFound    ValidationErrorCode = "func_ref_not_found"
	ValidationErrCondFuncNotFound   ValidationErrorCode = "cond_func_not_found"
	ValidationErrUndefinedField     ValidationErrorCode = "undefined_state_field"
	ValidationErrReducerMismatch    ValidationErrorCode = "reducer_type_mismatch"
	ValidationErrLoopNoExit         ValidationErrorCode = "loop_no_exit"
	ValidationErrDuplicateNode      ValidationErrorCode = "duplicate_node"
	ValidationErrEdgeTargetMissing  ValidationErrorCode = "edge_target_missing"
	ValidationErrInvalidMapperJSON  ValidationErrorCode = "invalid_mapper_json"
	ValidationErrInvalidRetryPolicy ValidationErrorCode = "invalid_retry_policy"
	ValidationErrSubgraphCycle      ValidationErrorCode = "subgraph_cycle"
	ValidationErrSubgraphDepth      ValidationErrorCode = "subgraph_depth_exceeded"
)

type ValidationError struct {
	Code    ValidationErrorCode `json:"code"`
	NodeID  string              `json:"node_id,omitempty"`
	Field   string              `json:"field,omitempty"`
	Message string              `json:"message"`
}

type ValidationWarning struct {
	Code    ValidationErrorCode `json:"code"`
	NodeID  string              `json:"node_id,omitempty"`
	Field   string              `json:"field,omitempty"`
	Message string              `json:"message"`
}

type ValidationResult struct {
	Errors   []ValidationError   `json:"errors"`
	Warnings []ValidationWarning `json:"warnings"`
}

func (r *ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

func (r *ValidationResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}

func (r *ValidationResult) AddError(code ValidationErrorCode, nodeID, field, msg string) {
	r.Errors = append(r.Errors, ValidationError{
		Code: code, NodeID: nodeID, Field: field, Message: msg,
	})
}

func (r *ValidationResult) AddWarning(code ValidationErrorCode, nodeID, field, msg string) {
	r.Warnings = append(r.Warnings, ValidationWarning{
		Code: code, NodeID: nodeID, Field: field, Message: msg,
	})
}

type AgentExistenceChecker func(ctx context.Context, agentName string) bool

func ValidateGraph(ctx context.Context, def *GraphBuildConfig, agentChecker AgentExistenceChecker, reg *Registry) *ValidationResult {
	result := &ValidationResult{}

	validateTopology(def, result)
	validateNodeRefs(def, reg, result)
	validateAgentRefs(ctx, def, agentChecker, result)
	validateStateSchema(def, result)
	validateLoopExits(def, result)
	validateNodePolicies(def, result)
	validateSubgraphCycles(def, result)
	validateSubgraphDepth(def, result)

	return result
}

func validateSubgraphCycles(def *GraphBuildConfig, result *ValidationResult) {
	if def == nil {
		return
	}
	loading := map[string]struct{}{}
	for _, sub := range def.Subgraphs {
		validateSubgraphChain(sub.ID, sub.GraphID, sub.BuildConfig, loading, result)
	}
}

func validateSubgraphDepth(def *GraphBuildConfig, result *ValidationResult) {
	if def == nil {
		return
	}
	for i := range def.Subgraphs {
		checkSubgraphDepth(&def.Subgraphs[i], 1, result)
	}
}

func checkSubgraphDepth(sub *biz.SubgraphDef, depth int, result *ValidationResult) {
	if depth > maxSubgraphDepth {
		result.AddError(ValidationErrSubgraphDepth, sub.ID, "subgraph",
			fmt.Sprintf("子图嵌套深度 %d 超过上限 %d（节点 %q）", depth, maxSubgraphDepth, sub.ID))
		return
	}
	for i := range sub.BuildConfig.Subgraphs {
		checkSubgraphDepth(&sub.BuildConfig.Subgraphs[i], depth+1, result)
	}
}

func validateSubgraphChain(nodeID, graphID string, cfg GraphBuildConfig, loading map[string]struct{}, result *ValidationResult) {
	graphID = strings.TrimSpace(graphID)
	if graphID != "" {
		if _, ok := loading[graphID]; ok {
			result.AddError(ValidationErrSubgraphCycle, nodeID, "subgraph_id",
				fmt.Sprintf("子图引用循环 detected at %q", graphID))
			return
		}
		loading[graphID] = struct{}{}
		defer delete(loading, graphID)
	}
	for _, sub := range cfg.Subgraphs {
		validateSubgraphChain(sub.ID, sub.GraphID, sub.BuildConfig, loading, result)
	}
}

func validateNodePolicies(def *GraphBuildConfig, result *ValidationResult) {
	for _, n := range def.Nodes {
		validateMapperJSON(result, n.ID, "input_mapper_json", n.InputMapperJSON)
		validateMapperJSON(result, n.ID, "output_mapper_json", n.OutputMapperJSON)
		if n.RetryMaxAttempts < 0 {
			result.AddError(ValidationErrInvalidRetryPolicy, n.ID, "retry_max_attempts", "retry_max_attempts must be >= 0")
		}
		if n.CacheEnabled && n.CacheTTLSeconds < 0 {
			result.AddWarning(ValidationErrInvalidMapperJSON, n.ID, "cache_ttl_seconds", "cache_ttl_seconds must be >= 0")
		}
	}
}

func validateMapperJSON(result *ValidationResult, nodeID, field, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	if !json.Valid([]byte(raw)) {
		result.AddError(ValidationErrInvalidMapperJSON, nodeID, field, field+" must be valid JSON")
		return
	}
	var mapping map[string]string
	if err := json.Unmarshal([]byte(raw), &mapping); err != nil {
		result.AddError(ValidationErrInvalidMapperJSON, nodeID, field, field+" must be a JSON object of string keys")
	}
}

func validateTopology(def *GraphBuildConfig, result *ValidationResult) {
	if len(def.Nodes) == 0 && len(def.Subgraphs) == 0 {
		result.AddError(ValidationErrEmptyGraph, "", "", "Graph 必须包含至少一个节点")
		return
	}

	if def.EntryPoint == "" {
		result.AddError(ValidationErrNoEntryPoint, "", "entry_point", "Graph 必须指定入口点")
		return
	}

	nodeSet := make(map[string]bool, len(def.Nodes)+len(def.Subgraphs))
	nodeIDSet := make(map[string]bool)

	for _, n := range def.Nodes {
		if nodeIDSet[n.ID] {
			result.AddError(ValidationErrDuplicateNode, n.ID, "id", fmt.Sprintf("节点 ID %q 重复", n.ID))
			continue
		}
		nodeIDSet[n.ID] = true
		nodeSet[n.ID] = true
	}
	for _, sub := range def.Subgraphs {
		if nodeIDSet[sub.ID] {
			result.AddError(ValidationErrDuplicateNode, sub.ID, "id", fmt.Sprintf("节点 ID %q 重复", sub.ID))
			continue
		}
		nodeIDSet[sub.ID] = true
		nodeSet[sub.ID] = true
	}

	if !nodeSet[def.EntryPoint] {
		result.AddError(ValidationErrNoEntryPoint, def.EntryPoint, "entry_point",
			fmt.Sprintf("入口点 %q 不存在于节点列表中", def.EntryPoint))
		return
	}

	for _, e := range def.Edges {
		if !nodeSet[e.To] {
			result.AddError(ValidationErrEdgeTargetMissing, e.From, "to",
				fmt.Sprintf("边目标节点 %q 不存在", e.To))
		}
	}
	for _, ce := range def.ConditionalEdges {
		if !nodeSet[ce.From] {
			result.AddError(ValidationErrEdgeTargetMissing, ce.From, "from",
				fmt.Sprintf("条件边源节点 %q 不存在", ce.From))
		}
		for label, target := range ce.PathMap {
			if !nodeSet[target] {
				result.AddError(ValidationErrEdgeTargetMissing, ce.From, fmt.Sprintf("path_map[%s]", label),
					fmt.Sprintf("条件边目标节点 %q 不存在（标签 %q）", target, label))
			}
		}
	}

	reachable := make(map[string]bool)
	var walk func(nodeID string)
	walk = func(nodeID string) {
		if reachable[nodeID] {
			return
		}
		reachable[nodeID] = true
		for _, e := range def.Edges {
			if e.From == nodeID {
				walk(e.To)
			}
		}
		for _, ce := range def.ConditionalEdges {
			if ce.From == nodeID {
				for _, target := range ce.PathMap {
					walk(target)
				}
			}
		}
	}
	walk(def.EntryPoint)

	for id := range nodeSet {
		if !reachable[id] {
			result.AddWarning(ValidationErrUnreachableNode, id, "",
				fmt.Sprintf("节点 %q 不可达（从入口点 %q 无法到达）", id, def.EntryPoint))
		}
	}
}

func validateNodeRefs(def *GraphBuildConfig, reg *Registry, result *ValidationResult) {
	if reg == nil {
		return
	}
	for _, n := range def.Nodes {
		switch normalizeNodeType(n.Type) {
		case "llm", "tool", "tools", "task", "review", "agent", "router":
			continue
		}
		if n.FuncRef != "" {
			if _, err := reg.GetNodeFunc(n.FuncRef); err != nil {
				result.AddError(ValidationErrFuncRefNotFound, n.ID, "func_ref",
					fmt.Sprintf("节点 %q 引用的函数 %q 未注册", n.ID, n.FuncRef))
			}
		}
	}
	for _, ce := range def.ConditionalEdges {
		if ce.CondFuncRef != "" {
			if _, err := reg.GetCondFunc(ce.CondFuncRef); err != nil {
				result.AddError(ValidationErrCondFuncNotFound, ce.From, "cond_func_ref",
					fmt.Sprintf("条件边源 %q 引用的条件函数 %q 未注册", ce.From, ce.CondFuncRef))
			}
		}
	}
}

func validateAgentRefs(ctx context.Context, def *GraphBuildConfig, agentChecker AgentExistenceChecker, result *ValidationResult) {
	if agentChecker == nil {
		return
	}
	for _, n := range def.Nodes {
		if n.Type == "agent" && n.AgentName != "" {
			if !agentChecker(ctx, n.AgentName) {
				result.AddError(ValidationErrAgentNotFound, n.ID, "agent_name",
					fmt.Sprintf("节点 %q 引用的 Agent %q 不存在", n.ID, n.AgentName))
			}
		}
	}
}

func validateStateSchema(def *GraphBuildConfig, result *ValidationResult) {
	fieldSet := make(map[string]StateFieldDef, len(def.StateFields))
	for _, f := range def.StateFields {
		fieldSet[f.Name] = f
	}

	for _, n := range def.Nodes {
		if n.Type == "agent" && n.AgentName != "" {
			validateAgentNodeStateRefs(n, fieldSet, result)
		}
	}

	for _, f := range def.StateFields {
		switch f.Reducer {
		case ReducerAppend:
			if f.Type != "[]string" && f.Type != "[]int" && f.Type != "[]any" && f.Type != "[]float64" {
				result.AddWarning(ValidationErrReducerMismatch, "", f.Name,
					fmt.Sprintf("字段 %q 使用 AppendReducer 但类型为 %q（建议使用切片类型）", f.Name, f.Type))
			}
		case ReducerMerge:
			if f.Type != "map" {
				result.AddWarning(ValidationErrReducerMismatch, "", f.Name,
					fmt.Sprintf("字段 %q 使用 MergeReducer 但类型为 %q（建议使用 map 类型）", f.Name, f.Type))
			}
		}
	}
}

func validateAgentNodeStateRefs(n biz.NodeDef, fieldSet map[string]StateFieldDef, result *ValidationResult) {
	if strings.ToLower(strings.TrimSpace(n.Type)) != "agent" {
		return
	}
	validateMapperStateRefs(result, n.ID, "input_mapper_json", n.InputMapperJSON, fieldSet)
	validateMapperStateRefs(result, n.ID, "output_mapper_json", n.OutputMapperJSON, fieldSet)
}

func validateMapperStateRefs(result *ValidationResult, nodeID, field, raw string, fieldSet map[string]StateFieldDef) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	var mapping map[string]string
	if err := json.Unmarshal([]byte(raw), &mapping); err != nil {
		return
	}
	for _, target := range mapping {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if _, ok := fieldSet[target]; !ok {
			result.AddWarning(ValidationErrUndefinedField, nodeID, field,
				fmt.Sprintf("mapper references undefined state field %q", target))
		}
	}
}

func validateLoopExits(def *GraphBuildConfig, result *ValidationResult) {
	adj := make(map[string][]string)
	for _, e := range def.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	for _, ce := range def.ConditionalEdges {
		for _, target := range ce.PathMap {
			adj[ce.From] = append(adj[ce.From], target)
		}
	}

	// Find all strongly connected components (Tarjan's algorithm).
	// Nodes in SCCs of size > 1 are cycle nodes; single-node SCCs with a
	// self-loop are also cycle nodes.
	index := 0
	stack := make([]string, 0)
	onStack := make(map[string]bool)
	nodeIndex := make(map[string]int)
	nodeLowlink := make(map[string]int)
	cycleNodes := make(map[string]bool)

	var strongconnect func(v string)
	strongconnect = func(v string) {
		nodeIndex[v] = index
		nodeLowlink[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		for _, w := range adj[v] {
			if _, visited := nodeIndex[w]; !visited {
				strongconnect(w)
				if nodeLowlink[w] < nodeLowlink[v] {
					nodeLowlink[v] = nodeLowlink[w]
				}
			} else if onStack[w] {
				if nodeIndex[w] < nodeLowlink[v] {
					nodeLowlink[v] = nodeIndex[w]
				}
			}
		}

		if nodeLowlink[v] == nodeIndex[v] {
			// Pop SCC from stack.
			scc := make(map[string]bool)
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				scc[w] = true
				if w == v {
					break
				}
			}
			// Mark nodes in non-trivial SCCs as cycle nodes.
			if len(scc) > 1 {
				for n := range scc {
					cycleNodes[n] = true
				}
			} else if len(scc) == 1 {
				// Single-node SCC: cycle only if self-loop exists.
				for n := range scc {
					for _, next := range adj[n] {
						if next == n {
							cycleNodes[n] = true
							break
						}
					}
				}
			}
		}
	}

	for _, n := range def.Nodes {
		if _, visited := nodeIndex[n.ID]; !visited {
			strongconnect(n.ID)
		}
	}

	if len(cycleNodes) == 0 {
		return
	}

	finishPoint := def.FinishPoint

	// For each cycle node, check whether it has an exit path out of the cycle.
	for _, n := range def.Nodes {
		if !cycleNodes[n.ID] {
			continue
		}
		hasExit := false

		// Check conditional edges from this node that lead outside the cycle
		// and can reach the finish point.
		for _, ce := range def.ConditionalEdges {
			if ce.From != n.ID {
				continue
			}
			for _, target := range ce.PathMap {
				if !cycleNodes[target] && canReachFinish(target, finishPoint, adj) {
					hasExit = true
					break
				}
			}
			if hasExit {
				break
			}
		}

		// Check regular edges from this node that lead outside the cycle
		// and can reach the finish point.
		if !hasExit {
			for _, next := range adj[n.ID] {
				if !cycleNodes[next] && canReachFinish(next, finishPoint, adj) {
					hasExit = true
					break
				}
			}
		}

		if !hasExit {
			result.AddWarning(ValidationErrLoopNoExit, n.ID, "",
				fmt.Sprintf("节点 %q 可能处于无退出条件的循环中", n.ID))
		}
	}
}

// canReachFinish checks whether a path exists from nodeID to the finish point.
// A node with no outgoing edges (sink) is also considered reachable since it
// represents a terminal state in the graph.
func canReachFinish(nodeID, finishPoint string, adj map[string][]string) bool {
	visited := make(map[string]bool)
	var dfs func(id string) bool
	dfs = func(id string) bool {
		if id == finishPoint || id == "" {
			return true
		}
		if visited[id] {
			return false
		}
		visited[id] = true
		nexts := adj[id]
		if len(nexts) == 0 {
			// Sink node with no outgoing edges — treated as terminal.
			return true
		}
		for _, next := range nexts {
			if dfs(next) {
				return true
			}
		}
		return false
	}
	return dfs(nodeID)
}
