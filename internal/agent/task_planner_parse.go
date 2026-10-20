package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"

	"github.com/google/uuid"
)

func parseClarificationRequest(text string) ([]string, bool) {
	t := strings.TrimSpace(text)
	if t == "" || !strings.Contains(t, "needs_clarification") {
		return nil, false
	}
	// 澄清输出是单个 JSON object（subtasks 是 array——此处提取的首尾 {}
	// 跨多元素时 unmarshal 必失败、单元素时 NeedsClarification=false，均安全
	// 落到未命中分支）。
	start := strings.Index(t, "{")
	end := strings.LastIndex(t, "}")
	if start < 0 || end <= start {
		return nil, false
	}
	var raw struct {
		NeedsClarification bool     `json:"needs_clarification"`
		Questions          []string `json:"questions"`
	}
	if err := json.Unmarshal([]byte(t[start:end+1]), &raw); err != nil {
		return nil, false
	}
	if !raw.NeedsClarification {
		return nil, false
	}
	qs := make([]string, 0, len(raw.Questions))
	for _, q := range raw.Questions {
		if s := strings.TrimSpace(q); s != "" {
			qs = append(qs, s)
		}
	}
	if len(qs) == 0 {
		return nil, false
	}
	return qs, true
}

// decomposeClarificationError 是分解 LLM 的澄清出口信号（Q7）——不是故障：
// 任务存在阻塞性信息缺失，需先向用户澄清。重试分类判为永久性（同一 prompt
// 重试只会得到同一结论，重试纯粹烧 LLM 调用）。
type decomposeClarificationError struct {
	questions []string
}

func (e *decomposeClarificationError) Error() string {
	return fmt.Sprintf("decomposition requests user clarification (%d questions)", len(e.questions))
}

// parseDecompositionOutput parses the LLM output into SubTask slice.
func parseDecompositionOutput(text string) ([]biz.SubTask, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}

	// Extract JSON array
	if i := strings.Index(text, "["); i >= 0 {
		if j := strings.LastIndex(text, "]"); j > i {
			text = text[i : j+1]
		}
	}

	var rawTasks []struct {
		ID                   string                    `json:"id"`
		Name                 string                    `json:"name"`
		Description          string                    `json:"description"`
		DependsOn            []string                  `json:"depends_on"`
		RequiredCapabilities []string                  `json:"required_capabilities"`
		Priority             int                       `json:"priority"`
		EstimatedComplexity  float64                   `json:"estimated_complexity"`
		Deliverables         []biz.DeliverableContract `json:"deliverables"`
		InputContract        []biz.DeliverableContract `json:"input_contract"`
		DomainPath           string                    `json:"domain_path"`
	}

	if err := json.Unmarshal([]byte(text), &rawTasks); err != nil {
		return nil, apierror.Internal(apierror.DomainSpirit, "json unmarshal").WithCause(err)
	}

	// 2026-07-05 修复：LLM prompt 指定生成 st_1/st_2/... 格式的 ID，
	// 跨 session 会冲突（plan_steps_v2 表 id 字段全局 UNIQUE）。这里将
	// LLM 返回的 ID 重写为 st_<uuid>，同步重写 DependsOn 中的引用。
	// 所有后端/前端代码都不解析 ID 内容，仅作不透明字符串使用，因此
	// 重写不会破坏引用链。
	idRemap := make(map[string]string, len(rawTasks))
	for _, rt := range rawTasks {
		if strings.TrimSpace(rt.ID) == "" || strings.TrimSpace(rt.Name) == "" {
			continue
		}
		if _, exists := idRemap[rt.ID]; !exists {
			idRemap[rt.ID] = "st_" + uuid.NewString()
		}
	}

	subTasks := make([]biz.SubTask, 0, len(rawTasks))
	for _, rt := range rawTasks {
		if strings.TrimSpace(rt.ID) == "" || strings.TrimSpace(rt.Name) == "" {
			continue
		}
		if rt.DependsOn == nil {
			rt.DependsOn = []string{}
		}
		if rt.RequiredCapabilities == nil {
			rt.RequiredCapabilities = []string{}
		}
		if rt.Priority == 0 {
			rt.Priority = 3
		}
		if rt.EstimatedComplexity == 0 {
			rt.EstimatedComplexity = 0.5
		}
		// 重写 DependsOn 中的旧 ID 引用为新 UUID 格式
		rewiredDeps := make([]string, 0, len(rt.DependsOn))
		for _, depID := range rt.DependsOn {
			if newID, ok := idRemap[depID]; ok {
				rewiredDeps = append(rewiredDeps, newID)
			} else {
				// 未知引用保留原值，validateSubTaskDAG 会报错
				rewiredDeps = append(rewiredDeps, depID)
			}
		}
		subTasks = append(subTasks, biz.SubTask{
			ID:                   idRemap[rt.ID],
			Name:                 rt.Name,
			Description:          rt.Description,
			DependsOn:            rewiredDeps,
			RequiredCapabilities: rt.RequiredCapabilities,
			Priority:             rt.Priority,
			EstimatedComplexity:  rt.EstimatedComplexity,
			Deliverables:         sanitizeContracts(rt.Deliverables),
			InputContract:        sanitizeContracts(rt.InputContract),
			DomainPath:           NormalizeDomainPath(rt.DomainPath),
		})
	}

	// P1 形式契约（B.10.15.2）兜底派生：LLM 未输出契约但存在 DAG 依赖时，
	// 从 subtask 确定性派生（{step_id}_output, document/markdown），保证注入
	// 提示可引用、验证器有事可验。派生名在 DAG 内构造即匹配。
	deriveFallbackContracts(subTasks)

	return subTasks, nil
}

// sanitizeContracts drops contract entries with a blank name (advisory
// contract — malformed entries must not break planning) and entries named
// after the reserved deliverable state keys ("summary"/"cognition"): those
// names become MDC topics 1:1, but set_deliverable rejects writes under
// reserved keys, so keeping them would create unsatisfiable member contracts
// (TS9-BUG-4: planner authored a contract literally named "summary").
func sanitizeContracts(in []biz.DeliverableContract) []biz.DeliverableContract {
	if len(in) == 0 {
		return nil
	}
	out := make([]biz.DeliverableContract, 0, len(in))
	for _, c := range in {
		name := strings.TrimSpace(c.Name)
		if name == "" || name == "summary" || name == "cognition" {
			continue
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// deriveFallbackContracts deterministically derives contracts for DAG edges
// the LLM left undeclared:
//   - producer (subtask with dependents, no LLM deliverables) gets
//     {name: "<step_id>_output", type: document, format: markdown};
//   - consumer (subtask with depends_on, no LLM input_contract) gets one
//     derived entry per upstream referencing the same name.
//
// Derived names match by construction, so the advisory validator only flags
// genuine LLM inconsistencies. Subtasks without any dependency edge (neither
// producer nor consumer) get nothing.
func deriveFallbackContracts(subTasks []biz.SubTask) {
	hasDependent := make(map[string]bool, len(subTasks))
	for _, st := range subTasks {
		for _, depID := range st.DependsOn {
			hasDependent[depID] = true
		}
	}
	for i := range subTasks {
		st := &subTasks[i]
		if len(st.Deliverables) == 0 && hasDependent[st.ID] {
			st.Deliverables = []biz.DeliverableContract{{
				Name:        st.ID + "_output",
				Type:        "document",
				Format:      "markdown",
				Description: "derived fallback deliverable for " + st.Name,
			}}
		}
	}
	for i := range subTasks {
		st := &subTasks[i]
		if len(st.InputContract) > 0 || len(st.DependsOn) == 0 {
			continue
		}
		for _, depID := range st.DependsOn {
			st.InputContract = append(st.InputContract, biz.DeliverableContract{
				Name:        depID + "_output",
				Type:        "document",
				Format:      "markdown",
				Description: "derived fallback input from upstream " + depID,
			})
		}
	}
}

// closedLoopStrongIncident 匹配真实事故现场，不含单独的「故障」——
// 「故障诊断思路 / 巡检与故障处置说明」是知识或文档任务，不是闭环处置。
var closedLoopStrongIncident = regexp.MustCompile(`本次事故|生产事故|事故处置|告警分诊|告警处置|宕机|停机|outage|incident|postmortem|事故复盘`)

// closedLoopHandlePattern 是较弱的处置用语，仅在非文档/知识问法时生效。
var closedLoopHandlePattern = regexp.MustCompile(`故障处置|故障处理`)

// closedLoopKnowledgeSkip 排除方法论、说明、三句话要点等知识问答。
var closedLoopKnowledgeSkip = regexp.MustCompile(`故障诊断的?(基本)?思路|故障排查方法|说明.{0,16}故障|常见故障判断|三句话说明故障|故障诊断思路|巡检与故障`)

// closedLoopDocPattern 把「写说明 / 三句话 / 要点」判成文档任务，避免
// 「机房巡检与故障处置说明」被当成事故闭环。
var closedLoopDocPattern = regexp.MustCompile(`说明文档|撰写.{0,12}说明|写一份|三句话|要点清单|基本思路`)

// looksLikeClosedLoopIncident reports whether the user is asking to handle a
// real incident (alert triage / outage / postmortem), not to explain故障.
func looksLikeClosedLoopIncident(userMessage string) bool {
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return false
	}
	if closedLoopKnowledgeSkip.MatchString(msg) {
		return false
	}
	if closedLoopStrongIncident.MatchString(msg) {
		return true
	}
	if closedLoopHandlePattern.MatchString(msg) && !closedLoopDocPattern.MatchString(msg) {
		return true
	}
	return false
}

// appendClosedLoopPostmortem 实现 TS9-GAP-1：闭环类任务（事故/告警/故障处置）
// 在 LLM 分解未产出复盘节点时，由引擎确定性追加一个「事故复盘」subtask，
// 依赖当前 DAG 的全部叶子节点（所有处置完成后才复盘）。与
// deriveFallbackContracts 同模式：引擎补齐 LLM 之不足，流程控制权在引擎。
//
// 跳过条件：subtask < 2（非编排任务）、消息无闭环信号、LLM 已产出复盘节点。
// 返回新切片，不修改入参。
func appendClosedLoopPostmortem(userMessage string, subTasks []biz.SubTask) ([]biz.SubTask, *biz.SubTask, bool) {
	if len(subTasks) < 2 {
		return nil, nil, false
	}
	if !looksLikeClosedLoopIncident(userMessage) {
		return nil, nil, false
	}
	for _, st := range subTasks {
		name := strings.ToLower(st.Name)
		if strings.Contains(name, "复盘") || strings.Contains(name, "postmortem") {
			return nil, nil, false
		}
	}
	// 叶子节点：没有任何其他节点依赖它（与 buildDAGFromSubTasks LeafIDs 同义）。
	depended := make(map[string]bool, len(subTasks))
	for _, st := range subTasks {
		for _, depID := range st.DependsOn {
			depended[depID] = true
		}
	}
	leaves := make([]string, 0, 1)
	for _, st := range subTasks {
		if !depended[st.ID] {
			leaves = append(leaves, st.ID)
		}
	}
	if len(leaves) == 0 {
		return nil, nil, false
	}
	pm := biz.SubTask{
		ID:   "st_" + uuid.NewString(),
		Name: "事故复盘",
		// description 自包含（执行团队只能看到自己的 description）：说明输入
		// 来源（上游交付物）与产出要求（复盘报告结构与交付 topic）。
		Description:          "对本次事故处置全流程进行复盘：基于上游团队的交付物（告警分诊、根因定位、修复方案、恢复执行与验证结果），产出标准事故复盘报告，包含事故时间线、根因分析（直接/间接/系统层面）、处置过程评估、可执行的改进项清单。通过 set_deliverable 将复盘报告写入 postmortem-report。",
		DependsOn:            leaves,
		RequiredCapabilities: []string{"documentation", "research"},
		Priority:             5,
		EstimatedComplexity:  0.3,
		Deliverables: []biz.DeliverableContract{{
			Name:        "postmortem-report",
			Type:        "document",
			Format:      "markdown",
			Description: "事故复盘报告（时间线/根因/处置评估/改进项）",
		}},
		DomainPath: "办公/文档",
	}
	updated := make([]biz.SubTask, 0, len(subTasks)+1)
	updated = append(updated, subTasks...)
	updated = append(updated, pm)
	return updated, &pm, true
}

// validateSubTaskDAG checks for duplicate IDs, cycles and invalid references.
var fenceRE = regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*?)```")

// stripDecompositionFences removes optional markdown code fences from model output.
func stripDecompositionFences(s string) string {
	s = strings.TrimSpace(s)
	if m := fenceRE.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return s
}
