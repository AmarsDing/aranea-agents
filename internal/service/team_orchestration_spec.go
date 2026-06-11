package service

import (
	"strings"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/team"
)

func toProtoOrchestrationSpec(raw string) *v1.OrchestrationSpec {
	spec, err := biz.ParseOrchestrationSpec(raw)
	if err != nil {
		return nil
	}
	out := &v1.OrchestrationSpec{
		Version:             int32(spec.Version),
		Mode:                spec.Mode,
		LinkedGraphId:       spec.LinkedGraphID,
		RuntimeEngine:       spec.RuntimeEngine,
		TurnTimeoutSec:      int32(spec.TurnTimeoutSec),
		FirstByteTimeoutSec: int32(spec.FirstByteTimeoutSec),
		IntentAnchorAgentId: spec.IntentAnchorAgentID,
		Description:         spec.Description,
		MaxConcurrency:      int32(spec.MaxConcurrency),
		TimeoutSeconds:      int32(spec.TimeoutSeconds),
		LoopMaxIterations:   int32(spec.LoopMaxIterations),
		EnableCheckpoint:    spec.EnableCheckpoint,
	}
	for _, m := range spec.Members {
		out.Members = append(out.Members, &v1.OrchestrationMember{
			AgentId:    m.AgentID,
			Role:       m.Role,
			Name:       m.Name,
			TaskPrompt: m.TaskPrompt,
			Enabled:    m.Enabled(),
			SortOrder:  int32(m.SortOrder),
		})
	}
	if spec.Graph != nil {
		g := &v1.EmbeddedGraph{Version: int32(spec.Graph.Version), Layout: spec.Graph.Layout}
		for _, n := range spec.Graph.Nodes {
			g.Nodes = append(g.Nodes, &v1.EmbeddedGraphNode{
				Id:              n.ID,
				Type:            n.Type,
				Label:           n.Label,
				AgentId:         n.AgentID,
				Role:            n.Role,
				TaskPrompt:      n.TaskPrompt,
				Enabled:         n.Enabled,
				InterruptBefore: n.InterruptBefore,
				InterruptAfter:  n.InterruptAfter,
				Destinations:    append([]string(nil), n.Destinations...),
			})
		}
		for _, e := range spec.Graph.Edges {
			g.Edges = append(g.Edges, &v1.EmbeddedGraphEdge{
				Id: e.ID, Source: e.Source, Target: e.Target, Label: e.Label, Condition: e.Condition,
			})
		}
		out.Graph = g
	}
	if spec.FailurePolicy != nil {
		out.FailurePolicy = toProtoFailurePolicy(spec.FailurePolicy)
	}
	return out
}

func toProtoFailurePolicy(p *biz.TeamFailurePolicy) *v1.FailurePolicySpec {
	if p == nil {
		return nil
	}
	out := &v1.FailurePolicySpec{
		DefaultPolicy: p.Default,
		ParallelFail:  p.ParallelFail,
		OnError:       p.OnError,
		Retry: &v1.TeamRetryPolicySpec{
			MaxAttempts:       int32(p.Retry.MaxAttempts),
			InitialIntervalMs: int32(p.Retry.InitialIntervalMs),
			BackoffFactor:     p.Retry.BackoffFactor,
			MaxIntervalMs:     int32(p.Retry.MaxIntervalMs),
		},
	}
	if p.CircuitBreaker != nil {
		out.CircuitBreaker = &v1.CircuitBreakerSpec{
			FailureThreshold:    int32(p.CircuitBreaker.FailureThreshold),
			ResetTimeoutSeconds: int32(p.CircuitBreaker.ResetTimeoutSeconds),
			FallbackNode:        p.CircuitBreaker.FallbackNode,
		}
	}
	if len(p.NodeOverrides) > 0 {
		out.NodeOverrides = map[string]*v1.TeamNodeFailureOverrideSpec{}
		for k, ov := range p.NodeOverrides {
			item := &v1.TeamNodeFailureOverrideSpec{Policy: ov.Policy, FallbackAgent: ov.FallbackAgent}
			if ov.Retry != nil {
				item.Retry = &v1.TeamRetryPolicySpec{
					MaxAttempts:       int32(ov.Retry.MaxAttempts),
					InitialIntervalMs: int32(ov.Retry.InitialIntervalMs),
					BackoffFactor:     ov.Retry.BackoffFactor,
					MaxIntervalMs:     int32(ov.Retry.MaxIntervalMs),
				}
			}
			out.NodeOverrides[k] = item
		}
	}
	return out
}

func fromProtoOrchestrationSpec(pb *v1.OrchestrationSpec) biz.OrchestrationSpec {
	if pb == nil {
		return biz.DefaultOrchestrationSpec()
	}
	spec := biz.OrchestrationSpec{
		Version:             int(pb.GetVersion()),
		Mode:                pb.GetMode(),
		LinkedGraphID:       pb.GetLinkedGraphId(),
		RuntimeEngine:       pb.GetRuntimeEngine(),
		TurnTimeoutSec:      int(pb.GetTurnTimeoutSec()),
		FirstByteTimeoutSec: int(pb.GetFirstByteTimeoutSec()),
		IntentAnchorAgentID: pb.GetIntentAnchorAgentId(),
		Description:         pb.GetDescription(),
		MaxConcurrency:      int(pb.GetMaxConcurrency()),
		TimeoutSeconds:      int(pb.GetTimeoutSeconds()),
		LoopMaxIterations:   int(pb.GetLoopMaxIterations()),
		EnableCheckpoint:    pb.GetEnableCheckpoint(),
	}
	for _, m := range pb.GetMembers() {
		spec.Members = append(spec.Members, biz.OrchestrationMember{
			AgentID:    m.GetAgentId(),
			Role:       m.GetRole(),
			Name:       m.GetName(),
			TaskPrompt: m.GetTaskPrompt(),
			EnabledPtr: boolPtr(m.GetEnabled()),
			SortOrder:  int(m.GetSortOrder()),
		})
	}
	if g := pb.GetGraph(); g != nil {
		eg := &biz.EmbeddedGraphSpec{Version: int(g.GetVersion()), Layout: g.GetLayout()}
		for _, n := range g.GetNodes() {
			eg.Nodes = append(eg.Nodes, biz.EmbeddedGraphNodeSpec{
				ID: n.GetId(), Type: n.GetType(), Label: n.GetLabel(),
				AgentID: n.GetAgentId(), Role: n.GetRole(),
				TaskPrompt: n.GetTaskPrompt(), Enabled: n.Enabled,
				InterruptBefore: n.GetInterruptBefore(), InterruptAfter: n.GetInterruptAfter(),
				Destinations: append([]string(nil), n.GetDestinations()...),
			})
		}
		for _, e := range g.GetEdges() {
			eg.Edges = append(eg.Edges, biz.EmbeddedGraphEdgeSpec{
				ID: e.GetId(), Source: e.GetSource(), Target: e.GetTarget(),
				Label: e.GetLabel(), Condition: e.GetCondition(),
			})
		}
		spec.Graph = eg
	}
	if fp := pb.GetFailurePolicy(); fp != nil {
		spec.FailurePolicy = fromProtoFailurePolicy(fp)
	}
	biz.NormalizeOrchestrationSpec(&spec)
	return spec
}

func fromProtoFailurePolicy(fp *v1.FailurePolicySpec) *biz.TeamFailurePolicy {
	if fp == nil {
		return nil
	}
	out := &biz.TeamFailurePolicy{
		Default:      fp.GetDefaultPolicy(),
		ParallelFail: fp.GetParallelFail(),
		OnError:      fp.GetOnError(),
		Retry: biz.TeamRetryPolicy{
			MaxAttempts:       int(fp.GetRetry().GetMaxAttempts()),
			InitialIntervalMs: int(fp.GetRetry().GetInitialIntervalMs()),
			BackoffFactor:     fp.GetRetry().GetBackoffFactor(),
			MaxIntervalMs:     int(fp.GetRetry().GetMaxIntervalMs()),
		},
	}
	if cb := fp.GetCircuitBreaker(); cb != nil {
		out.CircuitBreaker = &biz.CircuitBreakerPolicy{
			FailureThreshold:    int(cb.GetFailureThreshold()),
			ResetTimeoutSeconds: int(cb.GetResetTimeoutSeconds()),
			FallbackNode:        cb.GetFallbackNode(),
		}
	}
	if len(fp.GetNodeOverrides()) > 0 {
		out.NodeOverrides = map[string]biz.TeamNodeFailureOverride{}
		for k, ov := range fp.GetNodeOverrides() {
			item := biz.TeamNodeFailureOverride{Policy: ov.GetPolicy(), FallbackAgent: ov.GetFallbackAgent()}
			if r := ov.GetRetry(); r != nil {
				item.Retry = &biz.TeamRetryPolicy{
					MaxAttempts:       int(r.GetMaxAttempts()),
					InitialIntervalMs: int(r.GetInitialIntervalMs()),
					BackoffFactor:     r.GetBackoffFactor(),
					MaxIntervalMs:     int(r.GetMaxIntervalMs()),
				}
			}
			out.NodeOverrides[k] = item
		}
	}
	return out
}

func mergeTeamDefinitionFromRequest(base string, pb *v1.Team) (string, error) {
	if pb == nil {
		return base, nil
	}
	if specPB := pb.GetOrchestrationSpec(); specPB != nil {
		spec := fromProtoOrchestrationSpec(specPB)
		merged, err := biz.MergeOrchestrationSpecIntoDefinition(base, spec)
		if err != nil {
			return base, err
		}
		base = merged
	}
	if linked := strings.TrimSpace(pb.GetLinkedGraphId()); linked != "" {
		var err error
		base, err = team.MergeLinkedGraphID(base, linked)
		if err != nil {
			return base, err
		}
	}
	return base, nil
}
