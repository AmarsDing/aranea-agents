package biz

import "strings"

// TeamFailurePolicy is the team-level failure handling configuration (M53 Phase 4).
type TeamFailurePolicy struct {
	Default        string                             `json:"default"` // retry_then_block | skip | fail_fast
	Retry          TeamRetryPolicy                    `json:"retry"`
	NodeOverrides  map[string]TeamNodeFailureOverride `json:"node_overrides"`
	ParallelFail   string                             `json:"parallel_fail"` // continue | abort
	CircuitBreaker *CircuitBreakerPolicy              `json:"circuit_breaker,omitempty"`
	OnError        string                             `json:"on_error,omitempty"` // await_review | halt
}

// TeamRetryPolicy maps to graph.RetryPolicy at compile time.
type TeamRetryPolicy struct {
	MaxAttempts       int     `json:"max_attempts"`
	InitialIntervalMs int     `json:"initial_interval_ms"`
	BackoffFactor     float64 `json:"backoff_factor"`
	MaxIntervalMs     int     `json:"max_interval_ms"`
}

// TeamNodeFailureOverride per-node failure handling.
type TeamNodeFailureOverride struct {
	Policy        string           `json:"policy"`
	Retry         *TeamRetryPolicy `json:"retry"`
	FallbackAgent string           `json:"fallback_agent"`
}

// ApplyFailurePolicy annotates graph nodes with retry / skip / fallback metadata.
func ApplyFailurePolicy(cfg GraphBuildConfig, policy *TeamFailurePolicy) GraphBuildConfig {
	if policy == nil {
		return cfg
	}
	defaultPolicy := normalizeFailureDefault(policy.Default)
	defaultRetry := policy.Retry
	if defaultRetry.MaxAttempts <= 0 && defaultPolicy == FailureDefaultRetryThenBlock {
		defaultRetry.MaxAttempts = 3
	}
	for i := range cfg.Nodes {
		nodeID := cfg.Nodes[i].ID
		override, hasOverride := policy.NodeOverrides[nodeID]
		effective := defaultPolicy
		retry := defaultRetry
		fallback := ""
		if hasOverride {
			if p := strings.TrimSpace(override.Policy); p != "" {
				effective = normalizeFailureDefault(p)
			}
			if override.Retry != nil {
				retry = *override.Retry
			}
			fallback = strings.TrimSpace(override.FallbackAgent)
		}
		cfg.Nodes[i].FailureAction = effective
		cfg.Nodes[i].FallbackAgent = fallback
		if effective == FailureDefaultRetryThenBlock && retry.MaxAttempts > 0 {
			cfg.Nodes[i].RetryMaxAttempts = retry.MaxAttempts
		}
	}
	return cfg
}

// ApplyCircuitBreakerPolicy attaches CB policy to the build config and ensures
// executable nodes have a minimum retry floor so consecutive final-failures can
// be counted by the graph runtime Pre/Post node callbacks (FP-02).
func ApplyCircuitBreakerPolicy(cfg GraphBuildConfig, policy *CircuitBreakerPolicy) GraphBuildConfig {
	if policy == nil || policy.FailureThreshold <= 0 {
		return cfg
	}
	cfg.CircuitBreaker = policy
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Type != NodeTypeAgent && cfg.Nodes[i].Type != NodeTypeLLM && cfg.Nodes[i].Type != NodeTypeTool {
			continue
		}
		if cfg.Nodes[i].RetryMaxAttempts <= 0 {
			cfg.Nodes[i].RetryMaxAttempts = 1
		}
	}
	return cfg
}

// WithCircuitBreakerScope sets the namespace used for graph-node breaker keys.
func WithCircuitBreakerScope(cfg GraphBuildConfig, scope string) GraphBuildConfig {
	cfg.CircuitBreakerScope = strings.TrimSpace(scope)
	return cfg
}

func normalizeFailureDefault(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case FailureDefaultSkip, FailureDefaultFailFast, FailureDefaultRetryThenBlock, FailureOnFailureSkip:
		return strings.ToLower(strings.TrimSpace(raw))
	case "":
		return FailureDefaultRetryThenBlock
	default:
		return FailureDefaultRetryThenBlock
	}
}

func nodeNeedsFailureRecovery(n NodeDef) bool {
	if strings.EqualFold(strings.TrimSpace(n.FuncRef), SkipNodeFuncRef) {
		return false
	}
	if strings.TrimSpace(n.FallbackAgent) != "" {
		return true
	}
	return n.FailureAction == FailureOnFailureSkip
}

// EnsureFailureRecoveryStateFields adds shared state keys for runtime skip / fallback recovery.
func EnsureFailureRecoveryStateFields(cfg GraphBuildConfig) GraphBuildConfig {
	needSkipped := false
	for _, n := range cfg.Nodes {
		if nodeNeedsFailureRecovery(n) {
			needSkipped = true
			break
		}
	}
	if !needSkipped {
		return cfg
	}
	return ensureSkippedNodesStateField(cfg)
}

func ensureSkippedNodesStateField(cfg GraphBuildConfig) GraphBuildConfig {
	for _, sf := range cfg.StateFields {
		if sf.Name == SkippedNodesStateKey {
			return cfg
		}
	}
	cfg.StateFields = append(cfg.StateFields, StateFieldDef{
		Name:    SkippedNodesStateKey,
		Type:    "[]string",
		Reducer: ReducerAppend,
	})
	return cfg
}

// ApplySkipNodeSemantics converts policy=skip nodes into runtime skip function nodes and state tracking.
func ApplySkipNodeSemantics(cfg GraphBuildConfig) GraphBuildConfig {
	hasSkip := false
	for i := range cfg.Nodes {
		if cfg.Nodes[i].FailureAction != FailureDefaultSkip {
			continue
		}
		hasSkip = true
		cfg.Nodes[i].Type = NodeTypeFunction
		cfg.Nodes[i].FuncRef = SkipNodeFuncRef
		cfg.Nodes[i].AgentName = ""
	}
	if !hasSkip {
		return cfg
	}
	return ensureSkippedNodesStateField(cfg)
}

// FinalizeGraphFailurePolicy applies runtime failure semantics after base policy annotation.
func FinalizeGraphFailurePolicy(cfg GraphBuildConfig, policy *TeamFailurePolicy, parallelBranchIDs []string) GraphBuildConfig {
	cfg = ApplyParallelFailContinue(cfg, policy, parallelBranchIDs)
	cfg = ApplySkipNodeSemantics(cfg)
	cfg = EnsureFailureRecoveryStateFields(cfg)
	return cfg
}

// ApplyParallelFailContinue marks parallel join branch nodes with skip-on-failure when parallel_fail=continue.
func ApplyParallelFailContinue(cfg GraphBuildConfig, policy *TeamFailurePolicy, parallelBranchIDs []string) GraphBuildConfig {
	if policy == nil {
		return cfg
	}
	if !strings.EqualFold(strings.TrimSpace(policy.ParallelFail), ParallelFailContinue) {
		return cfg
	}
	branches := parallelBranchNodeIDs(cfg, parallelBranchIDs)
	if len(branches) == 0 {
		return cfg
	}
	finish := strings.TrimSpace(cfg.FinishPoint)
	overrides := policy.NodeOverrides
	for i := range cfg.Nodes {
		id := cfg.Nodes[i].ID
		if id == finish {
			continue
		}
		if _, ok := branches[id]; !ok {
			continue
		}
		if overrides != nil {
			if ov, ok := overrides[id]; ok {
				p := strings.ToLower(strings.TrimSpace(ov.Policy))
				if p == FailureDefaultFailFast || p == FailureDefaultSkip {
					continue
				}
			}
		}
		action := strings.ToLower(strings.TrimSpace(cfg.Nodes[i].FailureAction))
		if action == FailureDefaultSkip || action == FailureOnFailureSkip {
			continue
		}
		if strings.TrimSpace(cfg.Nodes[i].FallbackAgent) != "" {
			continue
		}
		cfg.Nodes[i].FailureAction = FailureOnFailureSkip
	}
	return EnsureFailureRecoveryStateFields(cfg)
}

func parallelBranchNodeIDs(cfg GraphBuildConfig, parallelBranchIDs []string) map[string]struct{} {
	finish := strings.TrimSpace(cfg.FinishPoint)
	// join feeders（分支尾节点）：显式 branchIDs 优先，否则回退为 finish 的
	// 直接前驱。feeder 数 < 2 不是并行拓扑，不标记。
	feeders := cleanBranchIDs(parallelBranchIDs)
	if len(feeders) < 2 {
		if finish == "" {
			return nil
		}
		feeders = feedersOfFinish(cfg, finish)
		if len(feeders) < 2 {
			return nil
		}
	}
	// Y8：分支集合 = 既有启发式结果 ∪ 分支内部祖先节点。祖先扩展沿边反向
	// 遍历：可同时到达 ≥2 个 feeder 的共享上游节点（fan-out 入口）排除——
	// 它承载所有分支，失败必须中止全局。
	branches := branchInternalNodes(cfg, feeders)
	if len(parallelBranchIDs) < 2 && finish != "" {
		// 无显式 branchIDs 时的既有启发式：feeder + fan-out 节点的非 finish
		// 目标。与祖先扩展取并集，保持既有标记语义不回归。
		for id := range feeders {
			branches[id] = struct{}{}
		}
		outCount := map[string]int{}
		for _, e := range cfg.Edges {
			if strings.EqualFold(strings.TrimSpace(e.Kind), EdgeKindTransfer) {
				continue
			}
			outCount[e.From]++
		}
		for from, count := range outCount {
			if count <= 1 {
				continue
			}
			for _, e := range cfg.Edges {
				if strings.EqualFold(strings.TrimSpace(e.Kind), EdgeKindTransfer) {
					continue
				}
				if e.From == from && e.To != finish {
					branches[e.To] = struct{}{}
				}
			}
		}
	}
	if len(branches) == 0 {
		return nil
	}
	return branches
}

func cleanBranchIDs(ids []string) map[string]struct{} {
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			out[id] = struct{}{}
		}
	}
	return out
}

func feedersOfFinish(cfg GraphBuildConfig, finish string) map[string]struct{} {
	feeders := map[string]struct{}{}
	for _, e := range cfg.Edges {
		if strings.EqualFold(strings.TrimSpace(e.Kind), EdgeKindTransfer) {
			continue
		}
		if e.To == finish {
			feeders[e.From] = struct{}{}
		}
	}
	return feeders
}

// branchInternalNodes 返回「分支内部」节点集合：从各 join feeder 沿
// 非 transfer 边反向遍历求祖先集，恰好属于 1 个 feeder 祖先集（含 feeder
// 自身）的节点即分支专属节点。共享上游（≥2 个 feeder 的公共祖先）排除。
func branchInternalNodes(cfg GraphBuildConfig, feeders map[string]struct{}) map[string]struct{} {
	// 反向邻接表
	rev := map[string][]string{}
	for _, e := range cfg.Edges {
		if strings.EqualFold(strings.TrimSpace(e.Kind), EdgeKindTransfer) {
			continue
		}
		rev[e.To] = append(rev[e.To], e.From)
	}
	// 统计每个节点被多少个 feeder 的祖先集覆盖
	cover := map[string]int{}
	for feeder := range feeders {
		seen := map[string]struct{}{feeder: {}}
		stack := []string{feeder}
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			cover[n]++
			for _, up := range rev[n] {
				if _, ok := seen[up]; ok {
					continue
				}
				seen[up] = struct{}{}
				stack = append(stack, up)
			}
		}
	}
	out := map[string]struct{}{}
	for id, n := range cover {
		if n == 1 {
			out[id] = struct{}{}
		}
	}
	return out
}

// FilterVisualizationEdges removes edges marked visualization-only (Kind=transfer).
func FilterVisualizationEdges(cfg GraphBuildConfig) GraphBuildConfig {
	if len(cfg.Edges) == 0 {
		return cfg
	}
	out := make([]EdgeDef, 0, len(cfg.Edges))
	for _, e := range cfg.Edges {
		if strings.EqualFold(strings.TrimSpace(e.Kind), EdgeKindTransfer) {
			continue
		}
		out = append(out, e)
	}
	cfg.Edges = out
	return cfg
}
