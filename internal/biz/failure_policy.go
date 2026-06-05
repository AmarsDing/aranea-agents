package biz

import "strings"

// TeamFailurePolicy is the team-level failure handling configuration (M53 Phase 4).
type TeamFailurePolicy struct {
	Default        string                         `json:"default"` // retry_then_block | skip | fail_fast
	Retry          TeamRetryPolicy                `json:"retry"`
	NodeOverrides  map[string]TeamNodeFailureOverride `json:"node_overrides"`
	ParallelFail   string                         `json:"parallel_fail"` // continue | abort
	CircuitBreaker *CircuitBreakerPolicy          `json:"circuit_breaker,omitempty"`
	OnError        string                         `json:"on_error,omitempty"` // await_review | halt
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

// ApplyCircuitBreakerPolicy annotates nodes with circuit breaker metadata for runtime (FP-02).
func ApplyCircuitBreakerPolicy(cfg GraphBuildConfig, policy *CircuitBreakerPolicy) GraphBuildConfig {
	if policy == nil || policy.FailureThreshold <= 0 {
		return cfg
	}
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
	if len(parallelBranchIDs) >= 2 {
		branches := make(map[string]struct{}, len(parallelBranchIDs))
		for _, id := range parallelBranchIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			branches[id] = struct{}{}
		}
		if len(branches) >= 2 {
			return branches
		}
	}
	finish := strings.TrimSpace(cfg.FinishPoint)
	if finish == "" {
		return nil
	}
	feeders := map[string]struct{}{}
	outCount := map[string]int{}
	for _, e := range cfg.Edges {
		if strings.EqualFold(strings.TrimSpace(e.Kind), EdgeKindTransfer) {
			continue
		}
		outCount[e.From]++
		if e.To == finish {
			feeders[e.From] = struct{}{}
		}
	}
	if len(feeders) < 2 {
		return nil
	}
	branches := map[string]struct{}{}
	for id := range feeders {
		branches[id] = struct{}{}
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
	return branches
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
