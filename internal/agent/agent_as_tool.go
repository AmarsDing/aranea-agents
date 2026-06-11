package agent

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	agenttool "trpc.group/trpc-go/trpc-agent-go/tool/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const maxAgentDelegationDepth = 3

type delegationDepthKey struct{}

// withDelegationDepth stores the current delegation depth in context.
func withDelegationDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, delegationDepthKey{}, depth)
}

// delegationDepthFromCtx returns the current delegation depth (0 if unset).
func delegationDepthFromCtx(ctx context.Context) int {
	if v, ok := ctx.Value(delegationDepthKey{}).(int); ok {
		return v
	}
	return 0
}

// BuildAgentAsTool creates an Agent-as-Tool for the moderate (single-agent) path.
// It uses the matcher to find the best agent, builds it via TRPCBuilderDeps,
// and wraps it with agenttool.NewTool so Spirit can invoke it directly.
func BuildAgentAsTool(ctx context.Context, matcher biz.AgentMatcherPort, deps TRPCBuilderDeps, lg loggateway.Logger, taskDesc string, capabilities []string) (trpctool.Tool, error) {
	depth := delegationDepthFromCtx(ctx)
	if depth >= maxAgentDelegationDepth {
		return nil, kerrors.BadRequest("SPIRIT", fmt.Sprintf("agent delegation depth %d exceeds limit %d, refusing recursive delegation", depth, maxAgentDelegationDepth))
	}

	match, err := matcher.MatchAgent(ctx, taskDesc, capabilities)
	if err != nil {
		return nil, kerrors.InternalServer("SPIRIT", "agent matching failed: "+err.Error())
	}
	if match == nil {
		return nil, kerrors.NotFound("SPIRIT", "no matching agent found for: "+taskDesc)
	}

	bizAg, err := resolveBizAgentByKey(ctx, deps, match.AgentKey)
	if err != nil {
		return nil, kerrors.NotFound("SPIRIT", fmt.Sprintf("resolve agent %s: %s", match.AgentKey, err.Error()))
	}

	// Increment delegation depth so nested agent-as-tool calls are bounded.
	buildCtx := withDelegationDepth(ctx, depth+1)
	ag, err := BuildTRPCAgentCached(buildCtx, bizAg, deps, lg)
	if err != nil {
		return nil, kerrors.InternalServer("SPIRIT", fmt.Sprintf("build agent %s: %s", match.AgentKey, err.Error()))
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
		return nil, kerrors.NotFound("SPIRIT", fmt.Sprintf("resolve agent %s: %s", agentKey, err.Error()))
	}
	return BuildTRPCAgentCached(ctx, bizAg, deps, lg)
}
