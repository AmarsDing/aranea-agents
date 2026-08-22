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
	"数据/分析": {"data", "analyst", "分析", "sql"},
	"创作/文学": {"writer", "literature", "文学", "文章"},
	"创作/文案": {"copy", "copywriter", "文案", "media"},
	"设计/视觉": {"design", "visual", "设计", "ui"},
	"研究/调研": {"research", "调研", "research"},
	"办公/文档": {"docs", "admin", "文档", "办公"},
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
// Primary is the first stable (AgentKey-sorted) hit; backup is the next key.
func bindRosterSpecialist(domainPath string, pool []biz.AgentCapability) (biz.AgentCapability, string, bool) {
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
		return hits[i].AgentKey < hits[j].AgentKey
	})
	backup := ""
	if len(hits) > 1 {
		backup = hits[1].AgentKey
	}
	return hits[0], backup, true
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
	for _, role := range cap.Roles {
		r := strings.ToLower(strings.TrimSpace(role))
		for _, tok := range tokens {
			if r != "" && r == strings.ToLower(strings.TrimSpace(tok)) {
				return true
			}
		}
	}
	return false
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
		{[]string{"k8s", "部署", "告警", "日志"}, "软件/运维"},
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
