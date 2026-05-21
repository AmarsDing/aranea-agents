package agent

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

// BizAgentFactoryOptions registers per-agent-key factories on a Runner so
// transfer / swarm resolution can build agents dynamically from the catalog.
func BizAgentFactoryOptions(deps TRPCBuilderDeps, agentKeys ...string) []trpcrunner.Option {
	var opts []trpcrunner.Option
	seen := make(map[string]struct{}, len(agentKeys))
	for _, key := range agentKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		opts = append(opts, trpcrunner.WithAgentFactory(key, bizAgentFactoryForKey(deps, key)))
	}
	return opts
}

func bizAgentFactoryForKey(deps TRPCBuilderDeps, agentKey string) trpcrunner.AgentFactory {
	key := strings.TrimSpace(agentKey)
	return func(ctx context.Context, _ trpcagent.RunOptions) (trpcagent.Agent, error) {
		ag, err := resolveBizAgentByKey(ctx, deps, key)
		if err != nil {
			return nil, err
		}
		return BuildTRPCAgentCached(ctx, ag, deps)
	}
}

func resolveBizAgentByKey(ctx context.Context, deps TRPCBuilderDeps, agentKey string) (biz.Agent, error) {
	key := strings.TrimSpace(agentKey)
	if key == "" {
		return biz.Agent{}, errors.New("agent key is required")
	}
	if deps.Agents != nil {
		ag, err := deps.Agents.GetAgentByAgentKey(ctx, key)
		if err == nil {
			event.CtxFlowLogDone(ctx, "system.agent.db_resolve", "Agent 数据库解析成功", event.P("agent_key", key), event.P("agent_id", ag.ID))
			if deps.AgentUC != nil {
				return deps.AgentUC.Get(ctx, ag.ID)
			}
			return ag, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			event.CtxFlowLogError(ctx, "system.agent.db_resolve", "Agent 数据库解析失败", event.P("agent_key", key), event.P("error", err))
			return biz.Agent{}, err
		}
	}
	event.CtxFlowLogWarn(ctx, "system.agent.db_resolve", "Agent 未找到", event.P("agent_key", key))
	return biz.Agent{}, errors.New("agent not found: " + key)
}
