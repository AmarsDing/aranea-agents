package agent

import (
	"context"
	"strconv"

	"aranea-agents/internal/biz"

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
	subs := make([]trpcagent.Agent, 0, len(keys))
	for _, key := range keys {
		ag, err := deps.Agents.GetAgentByAgentKey(ctx, key)
		if err != nil {
			return nil, kerrors.NotFound("AGENT", "sub agent "+key+" not found: "+err.Error())
		}
		built, err := BuildTRPCLLMAgentCached(ctx, ag, deps)
		if err != nil {
			return nil, kerrors.NotFound("AGENT", "sub agent "+key+" build failed: "+err.Error())
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
