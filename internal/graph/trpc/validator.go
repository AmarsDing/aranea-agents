package graph

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ValidationErrorCode string

const (
	ValidationErrEmptyGraph        ValidationErrorCode = "empty_graph"
	ValidationErrNoEntryPoint      ValidationErrorCode = "no_entry_point"
	ValidationErrUnreachableNode   ValidationErrorCode = "unreachable_node"
	ValidationErrOrphanNode        ValidationErrorCode = "orphan_node"
	ValidationErrAgentNotFound     ValidationErrorCode = "agent_not_found"
	ValidationErrFuncRefNotFound   ValidationErrorCode = "func_ref_not_found"
	ValidationErrCondFuncNotFound  ValidationErrorCode = "cond_func_not_found"
	ValidationErrUndefinedField    ValidationErrorCode = "undefined_state_field"
	ValidationErrReducerMismatch   ValidationErrorCode = "reducer_type_mismatch"
	ValidationErrLoopNoExit        ValidationErrorCode = "loop_no_exit"
	ValidationErrDuplicateNode     ValidationErrorCode = "duplicate_node"
	ValidationErrEdgeTargetMissing ValidationErrorCode = "edge_target_missing"
	ValidationErrInvalidMapperJSON ValidationErrorCode = "invalid_mapper_json"
	ValidationErrInvalidRetryPolicy ValidationErrorCode = "invalid_retry_policy"
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

type AgentExistenceChecker func(agentName string) bool

func ValidateGraph(def *GraphBuildConfig, agentChecker AgentExistenceChecker, reg *Registry) *ValidationResult {
	result := &ValidationResult{}

	validateTopology(def, result)
	validateNodeRefs(def, reg, result)
	validateAgentRefs(def, agentChecker, result)
	validateStateSchema(def, result)
	validateLoopExits(def, result)
	validateNodePolicies(def, result)

	return result
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
		case "llm", "tool", "tools":
			continue
		}
		if n.Func == nil && n.FuncRef != "" {
			if _, err := reg.GetNodeFunc(n.FuncRef); err != nil {
				result.AddError(ValidationErrFuncRefNotFound, n.ID, "func_ref",
					fmt.Sprintf("节点 %q 引用的函数 %q 未注册", n.ID, n.FuncRef))
			}
		}
	}
	for _, ce := range def.ConditionalEdges {
		if ce.CondFunc == nil && ce.CondFuncRef != "" {
			if _, err := reg.GetCondFunc(ce.CondFuncRef); err != nil {
				result.AddError(ValidationErrCondFuncNotFound, ce.From, "cond_func_ref",
					fmt.Sprintf("条件边源 %q 引用的条件函数 %q 未注册", ce.From, ce.CondFuncRef))
			}
		}
	}
}

func validateAgentRefs(def *GraphBuildConfig, agentChecker AgentExistenceChecker, result *ValidationResult) {
	if agentChecker == nil {
		return
	}
	for _, n := range def.Nodes {
		if n.Type == "agent" && n.AgentName != "" {
			if !agentChecker(n.AgentName) {
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

func validateAgentNodeStateRefs(n NodeDef, fieldSet map[string]StateFieldDef, result *ValidationResult) {
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

	visited := make(map[string]color)
	inStack := make(map[string]bool)

	var dfs func(nodeID string) bool
	dfs = func(nodeID string) bool {
		visited[nodeID] = colorGray
		inStack[nodeID] = true

		for _, next := range adj[nodeID] {
			if inStack[next] {
				return true
			}
			if visited[next] == colorWhite {
				if dfs(next) {
					return true
				}
			}
		}

		inStack[nodeID] = false
		visited[nodeID] = colorBlack
		return false
	}

	for _, n := range def.Nodes {
		if visited[n.ID] == colorWhite {
			if dfs(n.ID) {
				hasExit := false
				for _, ce := range def.ConditionalEdges {
					if ce.From == n.ID || isInPath(ce.From, n.ID, adj) {
						hasExit = true
						break
					}
				}
				if !hasExit {
					for _, e := range def.Edges {
						if e.From == n.ID {
							nexts := adj[n.ID]
							for _, next := range nexts {
								if next != n.ID && canReachEnd(next, adj) {
									hasExit = true
									break
								}
							}
							if hasExit {
								break
							}
						}
					}
				}
				if !hasExit {
					result.AddWarning(ValidationErrLoopNoExit, n.ID, "",
						fmt.Sprintf("节点 %q 可能处于无退出条件的循环中", n.ID))
				}
			}
		}
	}
}

const (
	colorWhite color = 0
	colorGray  color = 1
	colorBlack color = 2
)

type color int

func isInPath(from, target string, adj map[string][]string) bool {
	visited := make(map[string]bool)
	var dfs func(nodeID string) bool
	dfs = func(nodeID string) bool {
		if nodeID == target {
			return true
		}
		if visited[nodeID] {
			return false
		}
		visited[nodeID] = true
		for _, next := range adj[nodeID] {
			if dfs(next) {
				return true
			}
		}
		return false
	}
	return dfs(from)
}

func canReachEnd(nodeID string, adj map[string][]string) bool {
	visited := make(map[string]bool)
	var dfs func(id string) bool
	dfs = func(id string) bool {
		if id == "__end__" || id == "" {
			return true
		}
		if visited[id] {
			return false
		}
		visited[id] = true
		nexts := adj[id]
		if len(nexts) == 0 {
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
