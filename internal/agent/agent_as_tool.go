package agent

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	agenttool "trpc.group/trpc-go/trpc-agent-go/tool/agent"
)

const (
	// envMaxDelegateDepth overrides the delegation depth limit at startup
	// (P1-4). Process-lifetime fixed, matching the subagent maxConcurrent
	// env semantics (ARANEA_SUBAGENT_MAX_CONCURRENCY).
	envMaxDelegateDepth = "ARANEA_MAX_DELEGATE_DEPTH"
	// defaultMaxDelegateDepth bounds agent-as-tool / transfer delegation
	// nesting. Spirit→member→agent-tool chains stay ≤3 deep; anything
	// deeper is almost always a model loop and must fail loud.
	defaultMaxDelegateDepth = 3
)

// maxDelegateDepth resolves the effective delegation depth limit once per
// process (env override, else default). Shared by BuildAgentAsTool and the
// transfer controller so both channels enforce the same bound.
var maxDelegateDepth = resolveMaxDelegateDepth

// resolveMaxDelegateDepth reads ARANEA_MAX_DELEGATE_DEPTH; falls back to
// defaultMaxDelegateDepth when unset, unparsable, or < 1.
func resolveMaxDelegateDepth() int {
	v := strings.TrimSpace(os.Getenv(envMaxDelegateDepth))
	if v == "" {
		return defaultMaxDelegateDepth
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return defaultMaxDelegateDepth
	}
	return n
}

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
	if limit := maxDelegateDepth(); depth >= limit {
		return nil, apierror.BadRequest(apierror.DomainSpirit, "agent delegation depth %d exceeds limit %d, refusing recursive delegation", depth, limit)
	}

	match, err := matcher.MatchAgent(ctx, taskDesc, capabilities)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainSpirit, "agent matching failed").WithCause(err)
	}
	if match == nil {
		return nil, apierror.NotFound(apierror.DomainSpirit, "no matching agent found for: %s", taskDesc)
	}

	bizAg, err := resolveBizAgentByKey(ctx, deps, match.AgentKey)
	if err != nil {
		return nil, apierror.NotFound(apierror.DomainSpirit, "resolve agent %s", match.AgentKey).WithCause(err)
	}

	// Increment delegation depth so nested agent-as-tool calls are bounded.
	buildCtx := withDelegationDepth(ctx, depth+1)
	ag, err := BuildTRPCAgentCached(buildCtx, bizAg, deps, lg)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainSpirit, "build agent %s", match.AgentKey).WithCause(err)
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
		return nil, apierror.NotFound(apierror.DomainSpirit, "resolve agent %s", agentKey).WithCause(err)
	}
	return BuildTRPCAgentCached(ctx, bizAg, deps, lg)
}
