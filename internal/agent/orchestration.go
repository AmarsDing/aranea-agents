package agent

import (
	"context"
	"strconv"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcchainagent "trpc.group/trpc-go/trpc-agent-go/agent/chainagent"
	trpccycleagent "trpc.group/trpc-go/trpc-agent-go/agent/cycleagent"
	trpcparallelagent "trpc.group/trpc-go/trpc-agent-go/agent/parallelagent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func BuildChainAgent(ctx context.Context, name string, subAgentKeys []string, deps TRPCBuilderDeps) (trpcagent.Agent, error) {
	if len(subAgentKeys) == 0 {
		return nil, kerrors.BadRequest("AGENT", "chain agent requires at least one sub_agent_key")
	}
	subs, err := buildSubAgents(ctx, subAgentKeys, deps)
	if err != nil {
		return nil, err
	}
	return trpcchainagent.New(name, trpcchainagent.WithSubAgents(subs)), nil
}

func BuildCycleAgent(ctx context.Context, name string, cfg biz.OrchestrationConfig, deps TRPCBuilderDeps) (trpcagent.Agent, error) {
	if len(cfg.SubAgentKeys) == 0 {
		return nil, kerrors.BadRequest("AGENT", "cycle agent requires at least one sub_agent_key")
	}
	subs, err := buildSubAgents(ctx, cfg.SubAgentKeys, deps)
	if err != nil {
		return nil, err
	}
	opts := []trpccycleagent.Option{trpccycleagent.WithSubAgents(subs)}
	if cfg.MaxIterations != nil {
		opts = append(opts, trpccycleagent.WithMaxIterations(*cfg.MaxIterations))
	}
	if cfg.EscalationRule != "" {
		escFunc := buildEscalationFunc(cfg.EscalationRule)
		if escFunc != nil {
			opts = append(opts, trpccycleagent.WithEscalationFunc(escFunc))
		}
	}
	return trpccycleagent.New(name, opts...), nil
}

func BuildParallelAgent(ctx context.Context, name string, subAgentKeys []string, deps TRPCBuilderDeps) (trpcagent.Agent, error) {
	if len(subAgentKeys) == 0 {
		return nil, kerrors.BadRequest("AGENT", "parallel agent requires at least one sub_agent_key")
	}
	subs, err := buildSubAgents(ctx, subAgentKeys, deps)
	if err != nil {
		return nil, err
	}
	return trpcparallelagent.New(name, trpcparallelagent.WithSubAgents(subs)), nil
}

func buildSubAgents(ctx context.Context, keys []string, deps TRPCBuilderDeps) ([]trpcagent.Agent, error) {
	// Phase 1: Resolve all agent keys to bare catalog rows (single query per key).
	bareAgents := make([]biz.Agent, 0, len(keys))
	for _, key := range keys {
		ag, err := deps.Agents.GetAgentByAgentKey(ctx, key)
		if err != nil {
			return nil, kerrors.NotFound("AGENT", "sub agent "+key+" not found: "+err.Error())
		}
		bareAgents = append(bareAgents, ag)
	}
	// Phase 2: Batch hydrate all agents (settings + files, no extras).
	// This avoids the N+1 query pattern of calling Get() per agent.
	var hydrated []biz.Agent
	if deps.AgentUC != nil {
		var err error
		hydrated, err = deps.AgentUC.BatchHydrateForBuild(ctx, bareAgents)
		if err != nil {
			return nil, kerrors.NotFound("AGENT", "sub agent batch hydration failed: "+err.Error())
		}
	} else {
		deps.Logger().Warn("AgentUC not injected, sub agents will not be hydrated; runtime errors may occur",
			loggateway.StepID("agent.orchestration"))
		hydrated = bareAgents
	}
	// Phase 3: Build each hydrated agent into a trpc-agent-go Agent.
	subs := make([]trpcagent.Agent, 0, len(hydrated))
	for i, ag := range hydrated {
		built, err := BuildTRPCLLMAgentCached(ctx, ag, deps, deps.Logger())
		if err != nil {
			return nil, kerrors.NotFound("AGENT", "sub agent "+keys[i]+" build failed: "+err.Error())
		}
		subs = append(subs, built)
	}
	return subs, nil
}

func buildEscalationFunc(rule string) trpccycleagent.EscalationFunc {
	switch rule {
	case "error_free":
		return func(evt *trpcevent.Event) bool {
			return evt != nil && evt.Object == trpcmodel.ObjectTypeError
		}
	case "completion":
		return func(evt *trpcevent.Event) bool {
			return evt != nil && evt.Object == trpcmodel.ObjectTypeRunnerCompletion
		}
	default:
		if n, err := strconv.Atoi(rule); err == nil && n > 0 {
			return nil
		}
		return nil
	}
}
