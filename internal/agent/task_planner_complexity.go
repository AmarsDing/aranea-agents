package agent

import (
	"regexp"
	"strings"

	"aranea-agents/internal/biz"
)

// assessComplexity computes the six-dimension complexity assessment.
func (impl *taskPlannerImpl) assessComplexity(input biz.PlanInput) biz.DimensionScores {
	dims := biz.DimensionScores{}

	// Semantic: estimate from message length and intent kind (weight 0.25)
	runeCount := len([]rune(input.UserMessage))
	if runeCount > 500 {
		dims.Semantic = 0.7
	} else if runeCount > 200 {
		dims.Semantic = 0.4
	} else {
		dims.Semantic = 0.2
	}
	if input.IntentArtifact != nil {
		switch input.IntentArtifact.IntentKind {
		case "research", "debug":
			dims.Semantic += 0.15
		case "code_change":
			dims.Semantic += 0.1
		}
		if dims.Semantic > 1.0 {
			dims.Semantic = 1.0
		}
	}

	// Structural: check if user message contains multiple questions/tasks (weight 0.15)
	dims.Structural = assessStructural(input.UserMessage)

	// Domain: evaluate from intent kind and risk flags (weight 0.15)
	dims.Domain = assessDomain(input.IntentArtifact)

	// Tool: evaluate from intent kind and search hints (weight 0.10)
	dims.Tool = assessTool(input.IntentArtifact)

	// Context: check message length and ambiguity count (weight 0.10)
	dims.Context = assessContext(input.UserMessage, input.IntentArtifact)

	// Historical: use HistoryDQScore from input (weight 0.25)
	dims.Historical = input.HistoryDQScore

	return dims
}

// 复杂度评估正则——包级预编译（F10：函数内 MustCompile 每次调用重复编译，
// Complexity-assessment regexes (F10): compiled once for assessStructural.
var (
	sentenceEndersPattern = regexp.MustCompile(`[。！？.!?\n]`)
	enumPattern           = regexp.MustCompile(`(?:\d+[.、)]\s|[-*]\s)`)
)

// assessStructural evaluates structural complexity (multiple tasks/questions).
func assessStructural(userMessage string) float64 {
	score := 0.0
	// Count sentence-ending punctuation as proxy for multiple tasks
	matches := sentenceEndersPattern.FindAllString(userMessage, -1)
	if len(matches) >= 5 {
		score += 0.5
	} else if len(matches) >= 3 {
		score += 0.3
	} else {
		score += 0.1
	}

	// Check for enumeration patterns (1. 2. 3. or - - -)
	enumMatches := enumPattern.FindAllString(userMessage, -1)
	if len(enumMatches) >= 3 {
		score += 0.4
	} else if len(enumMatches) >= 1 {
		score += 0.2
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// assessDomain evaluates domain complexity from intent kind and risk flags.
func assessDomain(artifact *biz.IntentArtifact) float64 {
	if artifact == nil {
		return 0.1
	}
	score := 0.0
	// Risk flags indicate cross-domain concerns
	if len(artifact.RiskFlags) >= 2 {
		score += 0.4
	} else if len(artifact.RiskFlags) >= 1 {
		score += 0.2
	}
	// Certain intent kinds imply cross-domain work
	switch artifact.IntentKind {
	case "research", "debug":
		score += 0.3
	case "code_change":
		score += 0.15
	}
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.1 {
		score = 0.1
	}
	return score
}

// assessTool evaluates tool complexity from intent kind and search hints.
func assessTool(artifact *biz.IntentArtifact) float64 {
	if artifact == nil {
		return 0.1
	}
	score := 0.0
	// Certain intent kinds imply tool usage
	switch artifact.IntentKind {
	case "code_change", "debug":
		score += 0.3
	case "research":
		score += 0.2
	}
	// More search hints suggest more tool lookups needed
	if len(artifact.SearchHints) >= 3 {
		score += 0.3
	} else if len(artifact.SearchHints) >= 1 {
		score += 0.15
	}
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.1 {
		score = 0.1
	}
	return score
}

// assessContext evaluates context complexity.
func assessContext(userMessage string, artifact *biz.IntentArtifact) float64 {
	score := 0.0
	// Message length factor
	runeCount := len([]rune(userMessage))
	if runeCount > 1000 {
		score += 0.3
	} else if runeCount > 500 {
		score += 0.2
	} else {
		score += 0.1
	}

	// Ambiguity count
	if artifact != nil {
		ambCount := len(artifact.Ambiguities)
		if ambCount >= 3 {
			score += 0.5
		} else if ambCount >= 1 {
			score += 0.3
		}
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// determineStrategy selects the orchestration strategy based on the explicit
// mode passed by the Spirit LLM. The three valid modes are: direct, parallel,
// dag (see DECISION.md). Empty/auto mode defaults to direct.
//
// decisionSource must match Plan()'s evidence-chain labels (llm_mode /
// keyword_fallback / route_commit / …). StrategyReason must not claim
// 「用户指定」when the mode came from keyword_fallback or route_commit —
// live44 orch probe 2026-10-29 attributed detectTeamIntent("组建一个团队")
// as 「用户指定 dag 模式」.
func (impl *taskPlannerImpl) determineStrategy(_ biz.ComplexityLevel, _ float64, input biz.PlanInput, decisionSource string) (biz.OrchestrationStrategy, string, biz.TopologyType) {
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	reasonFor := func(m string) string {
		switch strings.TrimSpace(decisionSource) {
		case "keyword_fallback":
			return "关键词回退：检测到组队意图，采用 " + m + " 模式"
		case "route_commit":
			return "路由提交 PlanTeam，采用 " + m + " 模式"
		case "evidence_gate_downgrade":
			return "组队证据闸降级后采用 " + m + " 模式"
		default:
			return "用户指定 " + m + " 模式"
		}
	}
	switch mode {
	case "direct":
		return biz.StrategyDirect, reasonFor("direct"), biz.TopologyDirect
	case "parallel":
		return biz.StrategyParallel, reasonFor("parallel"), biz.TopologyParallel
	case "dag":
		return biz.StrategyDAG, reasonFor("dag"), biz.TopologyHybrid
	}
	// Empty / auto / unknown mode: default to direct. The LLM is instructed
	// (DECISION.md) to always pass an explicit mode; reaching this branch means
	// the LLM did not comply — direct is the safest fallback.
	return biz.StrategyDirect, "未指定 mode，默认 direct 模式", biz.TopologyDirect
}

func complexityRank(level biz.ComplexityLevel) int {
	switch level {
	case biz.ComplexityComplex:
		return 3
	case biz.ComplexityModerate:
		return 2
	case biz.ComplexitySimple:
		return 1
	}
	return 0
}

// applyCommittedComplexity raises Plan()'s re-score to the gate commit.
// QuickAssess and Plan() used to disagree (0831-r2 S06: gate moderate 0.3,
// Plan simple 0.13 with decision_source=route_commit) — the later writer
// silently undid the commit.
func applyCommittedComplexity(committed biz.RouteDecision, level biz.ComplexityLevel, score float64) (biz.ComplexityLevel, float64) {
	if committed.Score > score {
		score = committed.Score
	}
	if complexityRank(committed.Level) > complexityRank(level) {
		level = committed.Level
	}
	return level, score
}

// shouldForceComplex returns true when the explicit mode requires subtask
// decomposition so the allocator has subtasks to assign agents to.
// Team-forming modes (parallel/dag) need decomposition regardless of the
// complexity score; otherwise a "simple" score would skip decomposition and
// the team would never be assembled.
func shouldForceComplex(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "parallel", "dag":
		return true
	}
	return false
}

// shouldFailClosedOnDecompose is true when a failed/empty/verify-failed
// decompose must not silently become StrategyDirect (medium+ or explicit
// team-forming mode). Simple unforced turns may still degrade to direct.
func shouldFailClosedOnDecompose(level biz.ComplexityLevel, mode string) bool {
	if shouldForceComplex(mode) {
		return true
	}
	switch level {
	case biz.ComplexityModerate, biz.ComplexityComplex:
		return true
	}
	return false
}

func applyDecomposeDowngrade(
	strategy biz.OrchestrationStrategy,
	topology biz.TopologyType,
	level biz.ComplexityLevel,
	mode, reason, directNote string,
) (biz.OrchestrationStrategy, string, biz.TopologyType, string) {
	if shouldFailClosedOnDecompose(level, mode) {
		if strategy == biz.StrategyDirect || strategy == biz.StrategySingleAgent {
			strategy = biz.StrategyParallel
			topology = biz.TopologyParallel
		}
		return strategy, "任务分解失败，禁止降级为 direct", topology, reason
	}
	return biz.StrategyDirect, directNote, biz.TopologyDirect, reason
}

// promoteStrategyForSubTasks upgrades an empty/auto Direct strategy to a
// team-forming strategy when decomposition (or sub_intents) produced slots
// and complexity is at least moderate. Explicit mode=direct is preserved.
func promoteStrategyForSubTasks(
	strategy biz.OrchestrationStrategy,
	reason string,
	topology biz.TopologyType,
	level biz.ComplexityLevel,
	mode string,
	linearDepends bool,
) (biz.OrchestrationStrategy, string, biz.TopologyType) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "direct" || mode == "single" || mode == "single_agent" {
		return strategy, reason, topology
	}
	if strategy != biz.StrategyDirect {
		return strategy, reason, topology
	}
	if linearDepends {
		return biz.StrategyDAG, "复合意图按执行顺序出槽，默认 dag", biz.TopologyHybrid
	}
	if level != biz.ComplexityModerate && level != biz.ComplexityComplex {
		return strategy, reason, topology
	}
	return biz.StrategyParallel, "分解产出子任务且复杂度≥moderate，默认 parallel", biz.TopologyParallel
}
