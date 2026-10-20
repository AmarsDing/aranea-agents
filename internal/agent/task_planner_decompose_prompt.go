package agent

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// decomposeCapabilityTags 返回注入分解 prompt 的真实能力标签并集（与校验门
// R2 同一数据源 capabilityRoleUnion）。capBuilder 缺失 / BuildAll 失败 /
// 并集为空时返回 nil——调用方回退静态预定义清单（fail-open，与校验门对空
// 并集跳过 R2 的语义一致；旧测试构造路径行为不变）。
func (impl *taskPlannerImpl) decomposeCapabilityTags(ctx context.Context) []string {
	tags, _ := impl.decomposeRosterContext(ctx)
	return tags
}

// decomposeRosterContext 返回 (能力标签并集, 可分配名册行)。两者同源
// （capBuilder.BuildAll 只调一次）：能力标签供 required_capabilities 校验门
// R2 使用；名册行注入分解 prompt，让分解 LLM 看到真实可点名/可匹配的
// agent_key 与 domain_path（session-eval-20260831 S06 roster_miss 根修：
// 此前 prompt 只有 33 域抽象词表，LLM 产出词表外/英文 domain_path 必
// NormalizeDomainPath→其他→roster_miss，改道 build_orchestration_graph
// 又因手里没有 agent_key 无法填参）。fail-open：任一构建失败两者皆 nil。
func (impl *taskPlannerImpl) decomposeRosterContext(ctx context.Context) ([]string, []string) {
	if impl.capBuilder == nil {
		return nil, nil
	}
	caps, err := impl.capBuilder.BuildAll(ctx)
	if err != nil {
		impl.lg.Warn("分解 prompt 能力清单构建失败，回退静态标签清单（fail-open）",
			loggateway.StepID(biz.SpiritStepPlannerDecompose),
			loggateway.Err(err),
		)
		return nil, nil
	}
	return capabilityRoleUnion(caps), rosterPromptLines(filterHeuristicAssignable(caps))
}

// rosterPromptLines 把可分配名册渲染为分解 prompt 用的紧凑行：
// "- agent_key | DisplayName | DomainPath"。与 allocate 的可分配池
// （filterHeuristicAssignable）保持一致，避免 LLM 点名不可分配 agent。
func rosterPromptLines(caps []biz.AgentCapability) []string {
	if len(caps) == 0 {
		return nil
	}
	lines := make([]string, 0, len(caps))
	for _, cap := range caps {
		domain := NormalizeDomainPath(cap.DomainPath)
		if domain == domainLexiconOther {
			domain = ""
		}
		lines = append(lines, fmt.Sprintf("- %s | %s | %s", cap.AgentKey, strings.TrimSpace(cap.DisplayName), domain))
	}
	sort.Strings(lines)
	return lines
}

// buildDecompositionPrompt creates the system prompt for task decomposition.
// teamCount > 0 时强制要求 LLM 生成恰好 N 个 subtask（用户明确请求 N 个 team）；
// teamCount = 0 时使用默认 2-6 范围。
//
// 2026-07-04 问题 2 修复：原 prompt 固定 "2-6 subtasks"，未传递用户的数量
// 约束。当用户说"派出2个team"时，LLM 可能产生 5 个 subtask，orchestrateDAG
// 为每个 subtask 创建一个 team，导致最终多出 3 个 team。
func buildDecompositionPrompt(userMessage string, artifact *biz.IntentArtifact, teamCount int, capTags []string, rosterLines ...string) string {
	intentContext := ""
	if artifact != nil {
		intentContext = fmt.Sprintf("\nIntent analysis:\n- Refined goal: %s\n- Intent kind: %s\n- Risk flags: %v",
			artifact.RefinedGoal,
			artifact.IntentKind,
			artifact.RiskFlags,
		)

		if len(artifact.SuccessCriteria) > 0 {
			intentContext += fmt.Sprintf("\n- Success criteria: %v", artifact.SuccessCriteria)
		}
		if len(artifact.SearchHints) > 0 {
			intentContext += fmt.Sprintf("\n- Search hints: %v", artifact.SearchHints)
		}
	}

	countRule := "Break down complex tasks into 2-6 subtasks. Never produce more than 12 subtasks under any circumstance."
	if teamCount > 0 {

		countRule = fmt.Sprintf(`The user has explicitly requested EXACTLY %d teams.
You MUST produce EXACTLY %d subtasks — no more, no less.
Each subtask will be assigned to one dedicated team, so the subtask count MUST equal the requested team count.
Before finalizing, COUNT your subtasks; if the count differs from %d, merge or split subtasks until it matches exactly.`, teamCount, teamCount, teamCount)
	}

	capRule := "- required_capabilities must use these predefined tags: go-backend, go-kratos, vue3-frontend, quasar-ui, devops, database, architecture, testing, security, research, documentation, api-design, system-admin"
	if len(capTags) > 0 {
		capRule = fmt.Sprintf("- required_capabilities must ONLY use capability tags from the current agent roster: %s. NEVER invent tags outside this list — a subtask tagged with capabilities no agent possesses cannot be scheduled", strings.Join(capTags, ", "))
	}

	rosterSection := ""
	if len(rosterLines) > 0 {
		rosterSection = fmt.Sprintf(`
Current assignable agent roster (agent_key | display_name | domain_path):
%s
Pick each subtask's domain_path to match a domain that EXISTS in this roster whenever the task allows it — a subtask whose domain matches no roster entry cannot be scheduled. When you already know which roster agent fits a subtask, you MAY name that agent_key in the subtask description so the orchestrator can route it directly.
`, strings.Join(rosterLines, "\n"))
	}

	return fmt.Sprintf(`You are a task decomposition specialist. %s

Rules:
- Each subtask must have: id (st_1, st_2, etc.), name, description, depends_on (array of other subtask IDs), required_capabilities (from the predefined list), priority (1-5, 1=highest), estimated_complexity (0.0-1.0), domain_path (specialty slot from the lexicon — assign topics only, never person names or agent_keys)
- The "name" field MUST be a short noun-phrase suitable for displaying as a team name (e.g. "Code Analysis Team", "Data Pipeline Builder"), NOT a sentence-length task description. The "name" will be shown to the user as the team's display name; "id" is internal-only and never shown.
- Output ONLY a JSON array, no markdown fences, no commentary
%s
- System administration subtasks (Skill/MCP/package installation, system resource management) MUST be tagged "system-admin", and their "description" MUST be intent-based: state the outcome to achieve (what), the source URL, and the exact cli_admin_* tool name to use (e.g. "使用 cli_admin_skill_install_from_url 从 https://github.com/example/xlsx-skill 安装 xlsx skill，完成后用 cli_admin_skill_get 确认 enabled=true"). NEVER put shell command text (pip install, git clone, etc.) into the description — the system butler has no shell/exec tools and shell text induces hallucinated calls to non-existent tools.
- domain_path is the specialty slot. Classify into this lexicon (most specific fit; if none fits, use a top-level domain or "其他"). Do NOT pick a person: %s
- depends_on must only reference IDs of other subtasks in the array
- No circular dependencies allowed
- Dependency discipline: declare depends_on ONLY when a subtask genuinely consumes another subtask's output (its input_contract references the predecessor's deliverables). Parallel same-role teams (e.g. "two teams each write a poem, then a third team judges") MUST have empty depends_on; only the final merging/judging subtask depends on ALL of the parallel subtasks. NEVER chain parallel teams serially, and NEVER make every subtask depend on a "understand the current state / statistics" subtask unless that output is a real input to the work.
- Subtasks should be independently executable where possible
- CRITICAL: Each subtask "description" must be fully self-contained. The executing team sees ONLY its own description — it cannot see the user message or other subtasks. Every concrete parameter the executor needs (URLs, file paths, branch/tag names, subpaths, skill/agent names, numeric values, flags) MUST be copied verbatim into the description. NEVER use context references such as "the given URL", "the above parameters", "使用给定的/上述的/前文提到的" — always inline the actual values.
- CLARIFICATION EXIT: If the user message is missing BLOCKING concrete parameters whose absence would materially change the plan's direction (e.g. product/brand name, target date or deadline, channel/platform, budget, target system/URL), DO NOT fabricate them. Output a single JSON object instead of the subtask array: {"needs_clarification": true, "questions": ["<short question 1>", "<short question 2>"]} (2-5 questions, each answerable in one line). NEVER invent product names, dates, URLs, channel names, budget figures, or any concrete fact not present in the user message — when such a fact is essential and missing, asking is mandatory and guessing is forbidden. Use this exit ONLY for truly blocking gaps; for non-essential details, proceed and state reasonable generic assumptions explicitly in the description (e.g. "a mainstream social platform") rather than specific invented names.
- Each subtask MAY include "deliverables" (output contract array) and "input_contract" (input contract array). Contract element: {"name": string, "type": "document"|"code"|"data", "format": "markdown"|"json"|"zip", "description": string}. The contract "name" becomes the deliverable topic namespace that team members write via set_deliverable — keep it short (letters/digits in any language plus '_'/'-', no spaces or punctuation; a concise slug like "root-cause-report" or "根因报告" works) and NEVER use the reserved names "summary" or "cognition" (writes under them are rejected/overwritten).
- If subtask B depends_on subtask A, B's input_contract SHOULD declare references to A's deliverables using the SAME "name" values`, countRule, capRule, DomainLexiconPromptList()) + intentContext + rosterSection
}

// decomposeConfigError 表示分解前置条件不满足（catalog/http client 缺失、
// provider/model 未配置等）——重试相同请求无意义，属永久性错误。
type decomposeConfigError struct {
	err error
}

func (e *decomposeConfigError) Error() string {
	if e == nil || e.err == nil {
		return "decompose config error"
	}
	return e.err.Error()
}

func (e *decomposeConfigError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// decomposeFatalMarkers 标记 LLM 返回的永久性错误——重试相同请求不会成功。
var decomposeFatalMarkers = []string{
	"invalid_api_key",
	"unauthorized",
	"forbidden",
	"context length exceeded",
	"context_length_exceeded",
	"content_filter",
	"content filter",
}

// decomposeRetriableMarkers 标记瞬时故障——重试有意义。
// *StreamIdleError 的 Error() 输出必含 "llm stream idle"。
var decomposeRetriableMarkers = []string{
	"llm stream idle",
}

// isRetriableDecomposeError 对分解调用错误做保险丝分类：
//   - nil / 父 ctx 取消 / 永久性错误 → 不重试
//   - 其他（含被包装的 *StreamIdleError、io.EOF、网络抖动、超时）→ 无限重试
//
// 调用方必须在返回 Retriable 前检查 ctx 是否已取消——退避期间的穿透由调用方负责。
func isRetriableDecomposeError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// 配置级错误（catalog/http 缺失、provider/model 未配置）——重试无意义。
	var cfgErr *decomposeConfigError
	if errors.As(err, &cfgErr) {
		return false
	}
	// Q7 澄清出口：LLM 的有效响应而非故障——同一 prompt 重试只会得到同一
	// 结论，判永久性立即熔断（由 Plan() 透传澄清问题）。
	var clarifyErr *decomposeClarificationError
	if errors.As(err, &clarifyErr) {
		return false
	}
	msg := strings.ToLower(err.Error())

	for _, m := range decomposeFatalMarkers {
		if strings.Contains(msg, m) {
			return false
		}
	}

	for _, m := range decomposeRetriableMarkers {
		if strings.Contains(msg, m) {
			return true
		}
	}

	return true
}

// defaultMaxDecomposeAttempts 是分解瞬时故障的默认重试上限（含首次尝试）。
// F8/Y3：上限存在是为了让持续性故障最终走 decompose_failed 降级，而非无限重试。
const defaultMaxDecomposeAttempts = 5

// defaultDecomposeBackoff 是生产默认的指数退避——attempt 从 1 起。
// 100ms → 200ms → 400ms → 800ms → 1.6s → 3.2s → 6.4s → 10s（封顶）。
func defaultDecomposeBackoff(attempt int) time.Duration {
	const cap = 10 * time.Second
	d := 100 * time.Millisecond * (1 << (attempt - 1))
	if d > cap {
		d = cap
	}
	return d
}
