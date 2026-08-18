package agent

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// agentMatcherImpl implements biz.AgentMatcherPort using the agent catalog.
type agentMatcherImpl struct {
	agents biz.AgentReader
	lg     loggateway.Logger
}

// NewAgentMatcher creates an AgentMatcherPort backed by the agent catalog.
func NewAgentMatcher(agents biz.AgentReader, lg loggateway.Logger) biz.AgentMatcherPort {
	return &agentMatcherImpl{agents: agents, lg: lg}
}

func (m *agentMatcherImpl) MatchAgent(ctx context.Context, taskDesc string, capabilities []string) (*biz.AgentMatch, error) {
	result, err := m.agents.SearchAgents(ctx, biz.AgentListQuery{Status: string(biz.AgentStatusActive), Limit: 200})
	if err != nil {
		return nil, apierror.Internal(apierror.DomainAgent, "search agents").WithCause(err)
	}

	var bestMatch *biz.AgentMatch
	var bestScore float64

	taskTokens := NewUnicodeTokenizer(DefaultTokenizerOptions()).Tokenize(strings.ToLower(taskDesc))
	weights := DefaultMatchScoreWeights()

	for i := range result.Items {
		ag := &result.Items[i]
		if !biz.IsCatalogAgentAssignable(*ag) {
			continue
		}
		capScore := JaccardCapability(capabilities, ag.Roles)
		agentTokens := NewUnicodeTokenizer(DefaultTokenizerOptions()).Tokenize(strings.ToLower(ag.AgentDescription))
		semScore := TFSemantic(taskTokens, agentTokens)
		score := BlendMatchScore(capScore, semScore, weights)
		if score > bestScore {
			bestScore = score
			bestMatch = &biz.AgentMatch{
				AgentKey:    ag.AgentKey,
				DisplayName: ag.DisplayName,
				Score:       score,
				MatchReason: fmt.Sprintf("Capability=%.2f Semantic=%.2f Blended=%.2f", capScore, semScore, score),
			}
		}
	}

	// Minimum score threshold for a valid match.
	const minMatchScore = 0.3

	if bestMatch != nil && bestScore > minMatchScore {
		m.lg.Info("Agent 匹配成功",
			loggateway.StepID("agent.match"),
			loggateway.Str("matched_agent", bestMatch.AgentKey),
			loggateway.Float64("score", bestMatch.Score),
		)
		return bestMatch, nil
	}

	// Fallback: first assignable catalog agent.
	for i := range result.Items {
		ag := &result.Items[i]
		if !biz.IsCatalogAgentAssignable(*ag) {
			continue
		}
		m.lg.Info("Agent 匹配降级为首个可用 Agent",
			loggateway.StepID("agent.match"),
			loggateway.Str("fallback_agent", ag.AgentKey),
		)
		return &biz.AgentMatch{
			AgentKey:    ag.AgentKey,
			DisplayName: ag.DisplayName,
			Score:       0.1,
			MatchReason: "Fallback: no exact match, using first available agent",
		}, nil
	}

	m.lg.Warn("无可用 Agent 匹配",
		loggateway.StepID("agent.match"),
		loggateway.Str("task", taskDesc),
	)
	return nil, nil
}
