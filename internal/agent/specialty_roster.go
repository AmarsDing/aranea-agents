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
}

func rosterMissError(subTask biz.SubTask) error {
	spec := NormalizeDomainPath(subTask.DomainPath)
	if spec == "" {
		spec = strings.TrimSpace(subTask.Name)
	}
	if spec == "" {
		spec = subTask.ID
	}
	return apierror.BadRequest(apierror.DomainSpirit,
		fmt.Sprintf("no roster specialist for %s; specify an existing agent or add one on the org roster", spec))
}

// bindRosterSpecialist picks a pre-built specialist from the pruned pool.
// taskText (subtask name/description) boosts platform tokens such as 小红书.
func bindRosterSpecialist(domainPath, taskText string, pool []biz.AgentCapability) (biz.AgentCapability, string, bool) {
	spec := NormalizeDomainPath(domainPath)
	if spec == "" || spec == domainLexiconOther {
		return biz.AgentCapability{}, "", false
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
