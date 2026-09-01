package agent

import (
	"fmt"
	"sort"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// specialtyRoleAliases maps a lexicon specialty (domain_path) to catalog
// Role / PositionKey tokens used by pre-built specialists.
var specialtyRoleAliases = map[string][]string{
	"软件/后端": {"backend", "be", "后端", "golang", "api"},
	"软件/前端": {"frontend", "fe", "前端", "vue", "react"},
	"软件/测试": {"qa", "test", "测试"},
	"软件/运维": {"sre", "ops", "运维", "devops"},
	"软件/产品": {"product", "prd", "产品"},
	"软件/项目": {"project", "jira", "项目"},
	"软件/架构": {"architect", "架构"},
	"软件/安全": {"secops", "security", "安全", "渗透"},
	"软件/移动": {"mobile", "ios", "android", "flutter", "移动"},
	"软件/游戏": {"game", "unity", "unreal", "游戏"},
	"软件/空间": {"xr", "visionos", "spatial", "空间"},
	"软件/合规": {"compliance", "privacy", "合规", "隐私"},
	"数据/分析": {"data", "analyst", "分析", "sql"},
	"创作/文学": {"writer", "literature", "文学", "文章"},
	"创作/文案": {"copy", "copywriter", "文案", "media", "xiaohongshu", "小红书", "douyin", "抖音", "weibo", "zhihu"},
	"设计/视觉": {"design", "visual", "设计", "ui"},
	"研究/调研": {"research", "调研", "research"},
	"办公/文档": {"docs", "admin", "文档", "办公", "ops_doc_generation", "document_generator"},
	"运维/告警": {"alert", "告警", "incident", "ops_alarm_handler"},
	"运维/诊断": {"fault", "diagnos", "诊断", "log_analyst", "ops_fault_diagnosis", "ops_log_analysis"},
	"运维/变更": {"change", "runbook", "变更", "ops_change_execution"},
	"运维/巡检": {"inspect", "巡检", "network_inspector", "ops_network_inspection", "ops_auto_inspection", "ops_system_inspection"},
	"运维/复盘": {"postmortem", "复盘"},
	"商务/销售": {"sales", "outbound", "销售"},
	"商务/财务": {"finance", "财务", "fpa"},
	"商务/客服": {"support", "客服"},
	// 媒体/制作（2026-09-01 S06 种子修复）：produce 阶段评分/兜底匹配。
	"媒体/制作": {"media", "video", "produce", "媒体", "制作", "剪辑", "视频"},
}

func rosterMissError(subTask biz.SubTask, roster []biz.AgentCapability) error {
	spec := NormalizeDomainPath(subTask.DomainPath)
	if spec == "" {
		spec = strings.TrimSpace(subTask.Name)
	}
	if spec == "" {
		spec = subTask.ID
	}
	// 包B B3b：挂载 biz.ErrRosterMiss 哨兵，plan_and_execute 捕获后降级为
	// 结构化 NextAction=build_orchestration_graph（authorize_playbook 先例），
	// 不再裸露 BAD_REQUEST。
	//
	// 2026-08-31 S06 根修：错误文案尾部附可点名名册摘要。改道
	// build_orchestration_graph 的 agents[].agent_key 必填真实 key，而 LLM
	// 此前从未见过名册（S06 实录「可用的只有三个管家」），只能瞎编被
	// registry NOT_FOUND 打回。把可点名清单随错误一起回传，改道才可执行。
	msg := fmt.Sprintf("no roster specialist for %s; specify an existing agent or add one on the org roster", spec)
	if names := nameableRosterKeys(roster); names != "" {
		msg += "; nameable agent_key: " + names
	}
	return apierror.BadRequest(apierror.DomainSpirit, msg).
		WithCause(biz.ErrRosterMiss)
}

// nameableRosterKeys 渲染可点名 agent_key 摘要（最多 20 个，key 字典序）。
// 只列 key 不带描述：该文案随工具结果回填主会话上下文，逐轮计费，必须紧凑。
func nameableRosterKeys(roster []biz.AgentCapability) string {
	if len(roster) == 0 {
		return ""
	}
	keys := make([]string, 0, len(roster))
	for _, cap := range roster {
		if k := strings.TrimSpace(cap.AgentKey); k != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) > 20 {
		keys = keys[:20]
	}
	return strings.Join(keys, ", ")
}

// genericRoleDomains 包B B3a（session-eval-20260825, P-ROSTER-GAP）通用角色
// 映射：planner LLM 按行业常识产出的通用角色词（技术/内容/运营等）经
// NormalizeDomainPath 归并落入「其他」，花名册零映射直接 roster miss
// （S07-A `[SPIRIT/BAD_REQUEST] no roster specialist for 其他`）。将通用词
// 映射到词表一级域，rosterSpecialtyMatch 即可命中该域下任意岗位。仅在词表
// 路径匹配失败后兜底——误映射代价是分到同域邻近岗位，远低于编排路径裸露
// 报错。键序即优先级（先行业技术词，后职能词）。
//
// 2026-08-31 S06 根修：补英文别名（分解 prompt 为英文指令，LLM 常产出
// "Marketing/Copywriting" 式英文 domain_path，纯中文词表零命中）；匹配
// 统一小写化。英文别名取 ≥4 字符，避免短词子串误命中（如 bi⊂bilibili）。
var genericRoleDomains = []struct {
	words  []string
	domain string
}{
	{[]string{"技术", "研发", "开发", "工程", "程序员", "编码", "代码", "backend", "develop", "engineer", "coding", "software"}, "软件"},
	{[]string{"内容", "文案", "写作", "编辑", "媒体", "公众号", "content", "copywriting", "writing", "editorial", "douyin", "xiaohongshu", "wechat"}, "创作"},
	{[]string{"运营", "推广", "市场", "营销", "增长", "marketing", "promotion", "growth", "branding"}, "商务"},
	{[]string{"数据", "分析", "报表", "bi", "analytics", "analyst"}, "数据"},
	{[]string{"设计", "视觉", "ui", "design", "visual"}, "设计"},
	{[]string{"运维", "巡检", "值班", "sre", "devops", "inspection"}, "运维"},
	{[]string{"调研", "研究", "竞品", "research", "survey"}, "研究"},
	{[]string{"行政", "文档", "纪要", "办公", "documentation"}, "办公"},
}

// inferGenericRoleDomain 从原始 domainPath（归一化前的 LLM 产出）推断通用
// 角色对应的一级域；domainPath 无命中时回退扫描任务文本。零命中返回 ""。
// 匹配前统一小写（LLM 英文产出大小写随意，S06 实证 "Marketing/Copy"）。
func inferGenericRoleDomain(domainPath, taskText string) string {
	lowerPath := strings.ToLower(domainPath)
	for _, g := range genericRoleDomains {
		for _, w := range g.words {
			if strings.Contains(lowerPath, w) {
				return g.domain
			}
		}
	}
	lowerTask := strings.ToLower(taskText)
	for _, g := range genericRoleDomains {
		for _, w := range g.words {
			if strings.Contains(lowerTask, w) {
				return g.domain
			}
		}
	}
	return ""
}

// bindRosterSpecialist picks a pre-built specialist from the pruned pool.
// taskText (subtask name/description) boosts platform tokens such as 小红书.
func bindRosterSpecialist(domainPath, taskText string, pool []biz.AgentCapability) (biz.AgentCapability, string, bool) {
	spec := NormalizeDomainPath(domainPath)
	if spec == "" || spec == domainLexiconOther {
		// 包B B3a：词表归并失败（LLM 按行业常识产出通用角色词）时，走通用
		// 角色映射兜底到一级域——避免直接 roster miss（P-ROSTER-GAP）。
		spec = inferGenericRoleDomain(domainPath, taskText)
		if spec == "" {
			return biz.AgentCapability{}, "", false
		}
	}
	var hits []biz.AgentCapability
	for _, cap := range pool {
		if !cap.IsHeuristicAssignable() {
			continue
		}
		if rosterSpecialtyMatch(spec, cap) {
			hits = append(hits, cap)
		}
	}
	if len(hits) == 0 {
		return biz.AgentCapability{}, "", false
	}
	sort.Slice(hits, func(i, j int) bool {
		si, sj := rosterScore(spec, taskText, hits[i]), rosterScore(spec, taskText, hits[j])
		if si != sj {
			return si > sj
		}
		return hits[i].AgentKey < hits[j].AgentKey
	})
	backup := ""
	if len(hits) > 1 {
		backup = hits[1].AgentKey
	}
	return hits[0], backup, true
}

func rosterScore(spec, taskText string, cap biz.AgentCapability) int {
	score := 0
	capPath := NormalizeDomainPath(cap.DomainPath)
	if capPath != "" && capPath == spec {
		score += 10
	} else if capPath != "" && specialtyPathCompatible(spec, capPath) {
		score += 5
	}
	leaf := spec
	if i := strings.LastIndex(spec, "/"); i >= 0 {
		leaf = spec[i+1:]
	}
	pos := strings.ToLower(strings.TrimSpace(cap.PositionKey))
	name := strings.ToLower(strings.TrimSpace(cap.DisplayName))
	if pos != "" && (pos == strings.ToLower(leaf) || strings.Contains(pos, strings.ToLower(leaf))) {
		score += 40
	}
	blob := pos + " " + name + " " + strings.ToLower(cap.Description) + " " + strings.ToLower(cap.Mission)
	specific, generic := false, false
	for _, tok := range specialtyRoleAliases[spec] {
		if !rosterTokenHit(blob, tok) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(tok), leaf) {
			generic = true
		} else {
			specific = true
		}
	}
	if rosterTokenHit(blob, leaf) {
		generic = true
	}
	task := strings.ToLower(strings.TrimSpace(taskText))
	taskSpecific := false
	if task != "" {
		for _, tok := range specialtyRoleAliases[spec] {
			if strings.EqualFold(strings.TrimSpace(tok), leaf) {
				continue
			}
			if rosterTokenHit(task, tok) && rosterTokenHit(blob, tok) {
				score += 60
				taskSpecific = true
				break
			}
		}
	}
	if specific && (task == "" || taskSpecific) {
		score += 45
	} else if generic {
		score += 15
	}
	return score
}

func rosterSpecialtyMatch(spec string, cap biz.AgentCapability) bool {
	capPath := NormalizeDomainPath(cap.DomainPath)
	if capPath != "" && specialtyPathCompatible(spec, capPath) {
		return true
	}
	leaf := spec
	if i := strings.LastIndex(spec, "/"); i >= 0 {
		leaf = spec[i+1:]
	}
	if strings.EqualFold(strings.TrimSpace(cap.PositionKey), leaf) {
		return true
	}
	aliases := specialtyRoleAliases[spec]
	tokens := append([]string{leaf}, aliases...)
	blob := strings.ToLower(strings.TrimSpace(cap.PositionKey) + " " + cap.DisplayName + " " + cap.Description)
	for _, role := range cap.Roles {
		r := strings.ToLower(strings.TrimSpace(role))
		for _, tok := range tokens {
			if r != "" && r == strings.ToLower(strings.TrimSpace(tok)) {
				return true
			}
		}
	}
	for _, tok := range tokens {
		if rosterTokenHit(blob, tok) {
			return true
		}
	}
	return false
}

// rosterTokenHit matches CJK as substring and ASCII as a whole identifier
// token so "writer" does not steal copywriter, and "be" does not match backend.
func rosterTokenHit(blob, tok string) bool {
	tok = strings.ToLower(strings.TrimSpace(tok))
	blob = strings.ToLower(blob)
	if tok == "" || blob == "" {
		return false
	}
	if containsCJK(tok) {
		return strings.Contains(blob, tok)
	}
	for _, part := range rosterASCIIParts(blob) {
		if part == tok {
			return true
		}
	}
	return false
}

func containsCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

func rosterASCIIParts(s string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func specialtyPathCompatible(a, b string) bool {
	if a == b {
		return true
	}
	return strings.HasPrefix(b, a+"/") || strings.HasPrefix(a, b+"/")
}

// InferSpecialtyFromTask maps a short Chinese task to a lexicon specialty.
// Used by eval and as a deterministic hint; unknown text returns "".
func InferSpecialtyFromTask(text string) string {
	s := strings.TrimSpace(text)
	if s == "" {
		return ""
	}
	rules := []struct {
		need []string
		path string
	}{
		{[]string{"REST", "API", "接口", "鉴权", "Go"}, "软件/后端"},
		{[]string{"Vue", "前端", "表格", "组件", "官网"}, "软件/前端"},
		{[]string{"测试", "压测", "用例"}, "软件/测试"},
		{[]string{"k8s", "部署"}, "软件/运维"},
		{[]string{"告警", "P1", "分诊"}, "运维/告警"},
		{[]string{"故障", "根因", "RCA"}, "运维/诊断"},
		{[]string{"巡检", "基线"}, "运维/巡检"},
		{[]string{"漏斗", "SQL", "留存"}, "数据/分析"},
		{[]string{"品牌故事", "公众号", "长文", "写诗", "诗"}, "创作/文学"},
		{[]string{"小红书", "文案", "slogan", "种草"}, "创作/文案"},
		{[]string{"主视觉", "图标", "设计"}, "设计/视觉"},
		{[]string{"竞品", "访谈", "调研"}, "研究/调研"},
		{[]string{"纪要", "手册", "文档"}, "办公/文档"},
	}
	bestPath, bestHits := "", 0
	for _, r := range rules {
		hits := 0
		for _, n := range r.need {
			if strings.Contains(s, n) {
				hits++
			}
		}
		if hits > bestHits {
			bestHits = hits
			bestPath = r.path
		}
	}
	if bestHits == 0 {
		return ""
	}
	return bestPath
}
