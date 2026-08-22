package agent

import "strings"

// orgDomainAliases maps a normalized top-level domain (from DomainLexicon)
// to department name / org_key aliases used by OrgPruner. Aliases are
// matched case-insensitively as substrings against department Name and Key.
//
// Empty / "其他" are not listed: OrgPruner falls back to the full catalog
// (NFR-78-06) rather than guessing a catch-all department.
var orgDomainAliases = map[string][]string{
	"软件": {"研发", "工程", "技术", "开发", "软件", "engineering", "rd", "dev"},
	"数据": {"数据", "分析", "data", "analytics"},
	"创作": {"内容", "创作", "媒体", "运营", "content", "media"},
	"设计": {"设计", "视觉", "design"},
	"研究": {"研究", "调研", "research"},
	"办公": {"行政", "办公", "综合", "人事", "admin", "hr"},
}

// DomainDepartmentAliases returns department name/key aliases for a
// (possibly nested) domain path. Unknown or empty paths return nil.
func DomainDepartmentAliases(domainPath string) []string {
	top := TopLevelDomain(NormalizeDomainPath(domainPath))
	if top == "" || top == domainLexiconOther {
		return nil
	}
	return append([]string(nil), orgDomainAliases[top]...)
}

func matchDepartmentAlias(deptName, deptKey string, aliases []string) bool {
	if len(aliases) == 0 {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(deptName))
	key := strings.ToLower(strings.TrimSpace(deptKey))
	for _, a := range aliases {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		if name != "" && (name == a || strings.Contains(name, a)) {
			return true
		}
		if key != "" && (key == a || strings.Contains(key, a)) {
			return true
		}
	}
	return false
}
