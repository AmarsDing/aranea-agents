package agent

import (
	"regexp"
	"strconv"
	"strings"

	"aranea-agents/internal/biz"
)

// Team-count detection regexes (F10): compiled once. Complexity patterns live in task_planner_complexity.go.
var (
	teamCountDigitPattern = regexp.MustCompile(`(\d+)\s*(?:个|支)?\s*(?:teams?|团队)`)
	teamCountCNPattern    = regexp.MustCompile(`(一|两|二|三|四|五|六|七|八|九|十)\s*(?:个|支)?\s*(?:teams?|团队)`)
	teamCountENPattern    = regexp.MustCompile(`\b(one|two|three|four|five|six|seven|eight|nine|ten)\s+teams?\b`)
)

// detectTeamCount extracts the user's explicit team count from the message.
// Returns 0 when no count is specified (caller should use default range).
// Recognized patterns:
//   - "2个团队", "3支团队", "两个team", "三个团队"
//   - "组建3个团队", "分派两个团队", "创建2个团队"
//   - "two teams", "3 teams", "five teams"
//
// 2026-07-04 问题 2 修复：原 detectTeamIntent 只返回 mode（"dag"），
// 丢弃数量约束。这导致 LLM decomposition 自由产生 2-6 个 subtask，
// orchestrateDAG 为每个 subtask 创建一个 team，最终 team 数量与用户
// 请求不符（用户要 2 个 team，可能多出 3-5 个）。
func detectTeamCount(message string) int {
	lower := strings.ToLower(message)

	if m := teamCountDigitPattern.FindStringSubmatch(lower); len(m) >= 2 {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 && n <= 20 {
			return n
		}
	}

	cnNumMap := map[string]int{
		"一": 1, "两": 2, "二": 2, "三": 3, "四": 4, "五": 5,
		"六": 6, "七": 7, "八": 8, "九": 9, "十": 10,
	}
	if m := teamCountCNPattern.FindStringSubmatch(lower); len(m) >= 2 {
		if n, ok := cnNumMap[m[1]]; ok && n > 0 {
			return n
		}
	}

	enNumMap := map[string]int{
		"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
		"six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
	}
	if m := teamCountENPattern.FindStringSubmatch(lower); len(m) >= 2 {
		if n, ok := enNumMap[m[1]]; ok && n > 0 {
			return n
		}
	}
	return 0
}

// explicitToolRequestPattern matches snake_case tool identifiers with at least
// two underscores (e.g. cli_admin_skill_install_from_url, plan_and_execute).
// Single-underscore everyday tools (exec_command, web_search) deliberately do
// not match — mentioning them is a normal direct request, not an orchestration
// demand.
var explicitToolRequestPattern = regexp.MustCompile(`[a-z][a-z0-9]*(?:_[a-z0-9]+){2,}`)

// teamFormationFlexPattern matches team-formation verbs with a short filler
// (quantity / measure word) before 团队/team, e.g. "组建几个团队".
var teamFormationFlexPattern = regexp.MustCompile(`(?:组建|创建|成立|编排|调度|分派).{0,8}(?:团队|team)`)

// detectTeamIntent scans the user message for explicit team-formation keywords
// and returns the recommended mode ("parallel" or "dag"). Returns "" if no
// team intent is detected.
//
// This is a backend-side fallback for when the Spirit LLM passes mode="" or
// "auto" despite the user explicitly asking for teams. The DECISION.md prompt
// instructs the LLM to pass explicit mode, but LLMs don't always comply —
// this ensures teams are created when the user clearly requests them.
//
// Mode selection rationale (aligned with DECISION.md three-mode system):
//   - "分派N个团队"/"组建团队"/"团队协作" (team formation keywords) → dag:
//     user expects one or more multi-member teams (≥2 members each).
//   - "并行"/"同时执行" (parallel keywords) → parallel: each subtask gets 1
//     agent, no multi-member teams formed. Use when subtasks are independent.
//   - Team keywords take precedence over parallel keywords: if the user
//     mentions "团队" they want dag (multi-member collaboration), not parallel.
func detectTeamIntent(message string) string {
	lower := strings.ToLower(message)

	teamKeywords := []string{

		"两个team", "2个team", "两支team", "2支team",
		"两个团队", "2个团队", "两支团队", "2支团队",
		"三个team", "3个team", "三支team", "3支team",
		"三个团队", "3个团队", "三支团队", "3支团队",
		"多个团队", "多个team", "多支团队", "多支team",
		"几个团队", "几支团队", "几个team", "几支team",
		"分派两个", "分派2个", "分派三个", "分派3个",
		"分派团队", "分派team",

		"组建团队", "组建队", "组建team", "form a team", "build a team",
		"团队协作", "团队a", "团队b", "团队c",
		"组建一个团队", "组建一支团队", "组建1个团队", "组建1支团队",
		"组建两个团队", "组建两支团队", "组建2个团队", "组建2支团队",
		"组建三个团队", "组建三支团队", "组建3个团队", "组建3支团队",
		"组建多个团队", "组建多支团队",
		"组建几个团队", "组建几支团队",
		"创建团队", "创建一个团队", "创建一支团队",
		"成立团队", "成立一个团队",
		"编排团队", "编排一个团队",
		"调度团队", "调度一个团队",
	}
	for _, kw := range teamKeywords {
		if strings.Contains(lower, kw) {
			return "dag"
		}
	}

	if teamFormationFlexPattern.MatchString(lower) {
		return "dag"
	}

	parallelKeywords := []string{"并行", "同时执行", "并行处理", "parallel", "同时工作", "concurrently"}
	for _, kw := range parallelKeywords {
		if strings.Contains(lower, kw) {
			return "parallel"
		}
	}
	return ""
}

// HasTeamModeEvidence reports whether the message carries structural evidence
// for team-forming modes (parallel/dag). Exported for PrePlanningGate so
// ForcePlanning and Plan() share one discriminator.
func HasTeamModeEvidence(msg string) bool {
	return hasTeamModeEvidence(msg)
}

// hasTeamModeEvidence 报告消息是否携带组队模式（parallel/dag）的结构性
// 证据（包C Q2-C2 组队证据闸）。证据集：
//   - 组队意图词（detectTeamIntent：组建团队/并行处理…）
//   - 团队数量约束（detectTeamCount：两个团队/3 teams…）
//   - 组织链措辞（HasOrgChainIntent：按组织链/走编制…）
//   - 组织实体派发句式（biz.HasDispatchSignal：让市场部出方案/技术侧+
//     内容侧并列——S06/S07 型合法组队诉求）
//   - 模式字面量（用户显式写「dag 模式/组队模式」）
//
// 鉴别边界（战役语料钉住）：S09-t1「我们来规划…渠道、节奏、预算三大块」
// 是自我规划 + 内容板块枚举，无证据 → 降级 direct；S07「组织一次新产品
// 上线方案：技术侧…内容侧…运营侧…」多实体派发，有证据 → 放行。
func hasTeamModeEvidence(msg string) bool {
	if detectTeamIntent(msg) != "" || detectTeamCount(msg) > 0 || HasOrgChainIntent(msg) {
		return true
	}
	if biz.HasDispatchSignal(msg) {
		return true
	}
	lower := strings.ToLower(msg)
	for _, lit := range teamModeLiterals {
		if strings.Contains(lower, lit) {
			return true
		}
	}
	return false
}

// teamModeLiterals 用户显式指定组队模式的字面量（证据闸放行用，防误伤
// 「用 dag 模式跑这个任务」型显式诉求）。
var teamModeLiterals = []string{
	"dag模式", "dag 模式", "并行模式", "组队模式", "团队模式", "多智能体模式",
}
