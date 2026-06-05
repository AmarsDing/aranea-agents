package agent

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	agenttool "trpc.group/trpc-go/trpc-agent-go/tool/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// BuildAgentAsTool creates an Agent-as-Tool for the moderate (single-agent) path.
// It uses the matcher to find the best agent, builds it via TRPCBuilderDeps,
// and wraps it with agenttool.NewTool so Spirit can invoke it directly.
func BuildAgentAsTool(ctx context.Context, matcher biz.AgentMatcherPort, deps TRPCBuilderDeps, lg loggateway.Logger, taskDesc string, capabilities []string) (trpctool.Tool, error) {
	match, err := matcher.MatchAgent(ctx, taskDesc, capabilities)
	if err != nil {
		return nil, fmt.Errorf("agent matching failed: %w", err)
	}
	if match == nil {
		return nil, fmt.Errorf("no matching agent found for: %s", taskDesc)
	}

	bizAg, err := resolveBizAgentByKey(ctx, deps, match.AgentKey)
	if err != nil {
		return nil, fmt.Errorf("resolve agent %s: %w", match.AgentKey, err)
	}

	ag, err := BuildTRPCAgentCached(ctx, bizAg, deps, lg)
	if err != nil {
		return nil, fmt.Errorf("build agent %s: %w", match.AgentKey, err)
	}

	tool := agenttool.NewTool(
		ag,
		agenttool.WithSkipSummarization(true),
		agenttool.WithDescription(fmt.Sprintf("Delegate task to %s: %s", match.DisplayName, match.MatchReason)),
	)

	return tool, nil
}

// ResolveAndBuildAgent resolves an agent key to a trpc-agent-go Agent instance.
// This is a convenience function for callers that already know which agent to use.
func ResolveAndBuildAgent(ctx context.Context, agentKey string, deps TRPCBuilderDeps, lg loggateway.Logger) (trpcagent.Agent, error) {
	bizAg, err := resolveBizAgentByKey(ctx, deps, agentKey)
	if err != nil {
		return nil, fmt.Errorf("resolve agent %s: %w", agentKey, err)
	}
	return BuildTRPCAgentCached(ctx, bizAg, deps, lg)
}
