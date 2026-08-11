package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"aranea-agents/internal/biz"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// ── P3 M4: Agent Case → Skill 蒸馏器 ───────────────────────────────────────
//
// EverOS 蒸馏链的最后一环：M2 提取的 Agent Case 积累到阈值后（M4 Trigger 判定），
// 把最近一批高质量任务经验交给 LLM 蒸馏成一份 SKILL.md 草稿，作为 create_skill
// 建议汇入统一进化漏斗（人工审批后才落库，不在此自动创建技能）。
//
// 与 AgentCaseLLMExtractor 并列：复用 MemoryLLMExtractor 的 callModel 通道
// （provider 路由/超时/错误语义），用独立 prompt 做"多 case → 单技能"聚合。

// caseDistillSystemPrompt 要求 LLM 从一批任务经验中提炼可复用的通用技能，
// 而非逐条复述。name 约定英文 kebab-case（skill slug 语义），body 为完整
// SKILL.md 草稿（审批界面直接预览）。
const caseDistillSystemPrompt = `你是技能蒸馏器。输入是同一 Agent 近期完成真实任务的经验记录（goal/approach/outcome/pitfalls/tools）。从中提炼这些经验反复用到的通用做法，固化为一份可复用技能。

只输出一个 JSON 对象，不要输出任何其他内容：
{
  "name": "英文 kebab-case 技能名（如 batch-data-import），概括共性模式",
  "body": "完整 SKILL.md 正文（markdown）"
}

body 结构要求：
# <技能中文名>
## 何时使用
<什么场景下应该使用此技能，1-2 句>
## 步骤
<从成功案例提炼的通用步骤，编号列表，写可复用方法而非某次任务的流水账>
## 注意事项
<从 pitfalls/失败案例提炼的教训；若无失败案例可省略本节>

规则：
- 只固化在多条经验中重复出现的模式；单次偶然做法不要写入
- 步骤必须可迁移到新任务，禁止出现具体会话的日期/ID/一次性数据
- 经验之间若无共性（目标领域完全不同），只输出 {"name":"","body":""}
- 全文使用中文（name 除外）`

// caseDistillMinBodyRunes 是蒸馏正文的最低长度：低于此长度视为 LLM 敷衍输出，
// 按提取失败处理（Trigger 本轮跳过，下轮重试）。
const caseDistillMinBodyRunes = 10

// AgentCaseSkillDistiller implements biz.CaseSkillDistiller via the shared
// memory-worker LLM channel.
type AgentCaseSkillDistiller struct {
	llm *MemoryLLMExtractor
}

// NewAgentCaseSkillDistiller wraps the shared memory LLM extractor. Returns nil
// when llm is nil (Trigger then no-ops, same as legacy behavior).
func NewAgentCaseSkillDistiller(llm *MemoryLLMExtractor) *AgentCaseSkillDistiller {
	if llm == nil {
		return nil
	}
	return &AgentCaseSkillDistiller{llm: llm}
}

var _ biz.CaseSkillDistiller = (*AgentCaseSkillDistiller)(nil)

// DistillSkillFromCases runs one LLM call over the case digest and parses the
// (name, body) draft. All errors are best-effort: the Trigger skips this tick
// and retries on the next orchestrator scan.
func (d *AgentCaseSkillDistiller) DistillSkillFromCases(ctx context.Context, agentID string, cases []biz.AgentCase) (string, string, error) {
	if d == nil || d.llm == nil {
		return "", "", biz.ErrLLMExtractorUnavailable
	}
	digest := buildCaseDistillDigest(cases)
	if digest == "" {
		return "", "", biz.ErrLLMExtractionFailed
	}
	req := &trpcmodel.Request{
		Messages: []trpcmodel.Message{
			trpcmodel.NewSystemMessage(caseDistillSystemPrompt),
			trpcmodel.NewUserMessage("Agent task experience cases:\n\n" + digest),
		},
	}
	// callModel 仅用 ConsolidateInput 做 provider 路由（AgentID → MemoryWorker
	// 配置）；蒸馏无会话上下文，SessionID 留空。
	text, _, err := d.llm.callModel(ctx, biz.ConsolidateInput{AgentID: agentID}, req)
	if err != nil {
		return "", "", err
	}
	return parseCaseDistillResponse(text)
}

// caseDistillLLMResponse 是蒸馏 prompt 约定的 JSON 结构。
type caseDistillLLMResponse struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

// parseCaseDistillResponse parses the LLM JSON output into (name, body).
// Markdown code fences are tolerated. name 归一化为 slug（仅 [a-z0-9-]）；
// 空 name / 过短 body / 垃圾输出 → ErrLLMExtractionFailed（本轮跳过）。
func parseCaseDistillResponse(text string) (string, string, error) {
	s := stripCaseJSONFence(strings.TrimSpace(text))
	if s == "" {
		return "", "", biz.ErrLLMExtractionFailed
	}
	var r caseDistillLLMResponse
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return "", "", biz.ErrLLMExtractionFailed
	}
	name := slugifyDistilledSkillName(r.Name)
	body := strings.TrimSpace(r.Body)
	if name == "" {
		return "", "", biz.ErrLLMExtractionFailed
	}
	if len([]rune(body)) < caseDistillMinBodyRunes {
		return "", "", biz.ErrLLMExtractionFailed
	}
	return name, body, nil
}

// buildCaseDistillDigest renders cases as the LLM digest. Outcome 大写呈现
// （SUCCESS/PARTIAL/FAILURE），让成败信号在摘要中一眼可分；工具名与失败教训
// 是蒸馏质量的关键信号，必须保留。
func buildCaseDistillDigest(cases []biz.AgentCase) string {
	var sb strings.Builder
	n := 0
	for _, c := range cases {
		if strings.TrimSpace(c.Goal) == "" {
			continue
		}
		n++
		fmt.Fprintf(&sb, "### Case %d [%s] quality=%.2f\n", n, strings.ToUpper(strings.TrimSpace(c.Outcome)), c.Quality)
		sb.WriteString("- goal: " + strings.TrimSpace(c.Goal) + "\n")
		if v := strings.TrimSpace(c.Approach); v != "" {
			sb.WriteString("- approach: " + v + "\n")
		}
		if v := strings.TrimSpace(c.OutcomeSummary); v != "" {
			sb.WriteString("- outcome_summary: " + v + "\n")
		}
		if v := strings.TrimSpace(c.Pitfalls); v != "" {
			sb.WriteString("- pitfalls: " + v + "\n")
		}
		if len(c.ToolsUsed) > 0 {
			sb.WriteString("- tools: " + strings.Join(c.ToolsUsed, ", ") + "\n")
		}
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

// slugifyDistilledSkillName normalizes an LLM-proposed name to the skill slug
// charset [a-z0-9-]：非法字符折叠为单个 "-"，去首尾 "-"。纯非 ASCII 名称
// （如全中文）折叠后为空，由调用方按提取失败处理。
func slugifyDistilledSkillName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var sb strings.Builder
	lastDash := false
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			sb.WriteRune(r)
			lastDash = false
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			// 非 ASCII 字母/数字（如中文）：skill slug 仅收 ASCII，折叠为分隔符。
			if !lastDash {
				sb.WriteByte('-')
				lastDash = true
			}
			continue
		}
		if !lastDash {
			sb.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(sb.String(), "-")
}
