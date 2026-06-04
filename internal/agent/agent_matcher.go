package agent

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
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
	result, err := m.agents.SearchAgents(ctx, biz.AgentListQuery{Limit: 200})
	if err != nil {
		return nil, fmt.Errorf("search agents: %w", err)
	}

	var bestMatch *biz.AgentMatch
	var bestScore float64

	for i := range result.Items {
		ag := &result.Items[i]
		if ag.AgentKey == biz.SpiritAgentKey {
			continue
		}
		score := calculateMatchScore(*ag, capabilities, taskDesc)
		if score > bestScore {
			bestScore = score
			bestMatch = &biz.AgentMatch{
				AgentKey:    ag.AgentKey,
				DisplayName: ag.DisplayName,
				Score:       score,
				MatchReason: fmt.Sprintf("Role/capability match score: %.2f", score),
			}
		}
	}

	if bestMatch != nil && bestScore > 0.3 {
		m.lg.Info("Agent 匹配成功",
			loggateway.StepID("agent.match"),
			loggateway.Str("matched_agent", bestMatch.AgentKey),
			loggateway.Float64("score", bestMatch.Score),
		)
		return bestMatch, nil
	}

	// Fallback: return the first non-Spirit agent.
	// Full LLM-based matching will be implemented in Phase 2 (T2.1).
	for i := range result.Items {
		ag := &result.Items[i]
		if ag.AgentKey != biz.SpiritAgentKey {
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
	}

	m.lg.Warn("无可用 Agent 匹配",
		loggateway.StepID("agent.match"),
		loggateway.Str("task", taskDesc),
	)
	return nil, nil
}

// calculateMatchScore scores an agent against required capabilities and task description.
// It checks overlap between the agent's Roles and the required capabilities,
// and also checks if the task description keywords appear in the agent's description.
func calculateMatchScore(ag biz.Agent, capabilities []string, taskDesc string) float64 {
	var totalScore float64
	var maxScore float64

	// Dimension 1: Role/capability overlap
	if len(capabilities) > 0 && len(ag.Roles) > 0 {
		maxScore += 1.0
		roleSet := make(map[string]bool, len(ag.Roles))
		for _, r := range ag.Roles {
			roleSet[strings.ToLower(r)] = true
		}
		matches := 0
		for _, cap := range capabilities {
			if roleSet[strings.ToLower(cap)] {
				matches++
			}
		}
		totalScore += float64(matches) / float64(len(capabilities))
	}

	// Dimension 2: Task description keyword overlap with agent description
	if taskDesc != "" && ag.AgentDescription != "" {
		maxScore += 0.5
		descLower := strings.ToLower(ag.AgentDescription)
		taskWords := tokenize(strings.ToLower(taskDesc))
		if len(taskWords) > 0 {
			hits := 0
			for _, w := range taskWords {
				if strings.Contains(descLower, w) {
					hits++
				}
			}
			totalScore += 0.5 * float64(hits) / float64(len(taskWords))
		}
	}

	if maxScore == 0 {
		return 0
	}
	return totalScore / maxScore
}

// tokenize splits a string into lowercase word tokens for matching.
func tokenize(s string) []string {
	fields := strings.Fields(s)
	var tokens []string
	for _, f := range fields {
		f = strings.Trim(f, ".,;:!?()[]{}\"'")
		if len(f) >= 2 {
			tokens = append(tokens, f)
		}
	}
	return tokens
}
