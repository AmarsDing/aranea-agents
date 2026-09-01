package agent

import "strings"

// orgDomainAliases maps a normalized top-level domain (from DomainLexicon)
// to department name / org_key aliases used by OrgPruner. Aliases match
// case-insensitively as exact key, exact name, or name == alias+"部".
// Substring Contains is intentionally not used (误伤「内容运营部」⊃「运营」).
//
// Empty / "其他" are not listed: OrgPruner falls back to the full catalog
// (NFR-78-06) rather than guessing a catch-all department.
var orgDomainAliases = map[string][]string{
	"软件": {"研发", "工程", "技术", "开发", "软件", "engineering", "rd", "dev"},
	"数据": {"数据", "分析", "gis", "空间", "data", "analytics"},
	"创作": {"内容", "创作", "媒体", "运营", "content", "media"},
	"设计": {"设计", "视觉", "品牌", "design"},
	"研究": {"研究", "调研", "策划", "research"},
	"办公": {"行政", "办公", "综合", "人事", "专项", "文档", "admin", "hr"},
	"运维": {"告警", "诊断", "巡检", "执行", "修复", "复盘", "运维", "alert", "diagnostic", "inspect"},
	"商务": {"销售", "财务", "客服", "电商", "推广", "跨境", "sales", "finance"},
	"医疗": {"临床", "医疗", "健康", "clinical", "health"},
}

// leafDepartmentAliases are more precise than the top-level map so
// "软件/运维" hits 运维部 without also pulling every 开发部 into 软件/后端.
var leafDepartmentAliases = map[string][]string{
	"软件/后端": {"后端", "backend", "研发", "工程", "golang", "api"},
	"软件/前端": {"前端", "frontend", "vue", "react", "研发", "工程"},
	"软件/测试": {"测试", "质量", "qa", "quality", "研发", "工程"},
	"软件/运维": {"运维", "ops", "sre", "devops", "研发", "工程"},
	"软件/产品": {"产品", "product"},
	"软件/项目": {"项目", "project", "jira"},
	"软件/架构": {"架构", "architect"},
	"软件/安全": {"安全", "security", "secops"},
	"软件/移动": {"移动", "mobile", "ios", "android"},
	"软件/游戏": {"游戏", "game", "unity"},
	"软件/空间": {"空间计算", "xr", "visionos"},
	"软件/合规": {"合规", "隐私", "privacy", "fedramp"},
	"创作/文案": {"文案", "运营", "媒体", "小红书", "抖音"},
	"创作/文学": {"文学", "内容", "出版", "公众号"},
	"设计/视觉": {"设计", "视觉", "品牌"},
	"研究/调研": {"研究", "调研", "策划"},
	"数据/分析": {"数据", "分析", "sql"},
	"数据/空间": {"gis", "空间", "地理"},
	"商务/推广": {"推广", "付费", "ppc"},
	"商务/电商": {"电商", "跨境"},
	"商务/销售": {"销售", "sales"},
	"商务/财务": {"财务", "finance"},
	"商务/客服": {"客服", "支持"},
	"医疗/临床": {"临床", "clinical"},
	"医疗/创新": {"创新", "innovation"},
	"医疗/公卫": {"公卫", "主权健康"},
	"办公/文档": {"文档", "纪要", "行政"},
	"办公/专项": {"专项", "参谋"},
	"运维/告警": {"告警", "alert", "incident"},
	"运维/诊断": {"诊断", "故障", "日志", "指标"},
	"运维/变更": {"变更", "执行", "runbook"},
	"运维/巡检": {"巡检", "inspect", "网络", "数据库"},
	"运维/复盘": {"复盘", "文档报告", "postmortem"},
	// 媒体/制作（2026-09-01 S06 种子修复）：playbook produce 阶段专用域。
	// 无本条目时 Prune 前置闸门 len(aliases)==0 fail-closed，media_studio
	// 部门（媒体制作部）内 agent 即使 domain_path 精确相等也进不了候选池。
	"媒体/制作": {"媒体制作", "media_studio"},
}

// DomainDepartmentAliases returns department name/key aliases for a
// (possibly nested) domain path. Unknown or empty paths return nil.
func DomainDepartmentAliases(domainPath string) []string {
	norm := NormalizeDomainPath(domainPath)
	if norm == "" || norm == domainLexiconOther {
		return nil
	}
	if leaf := leafDepartmentAliases[norm]; len(leaf) > 0 {
		return append([]string(nil), leaf...)
	}
	top := TopLevelDomain(norm)
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
	if i := strings.LastIndexAny(key, "/\\"); i >= 0 {
		key = strings.TrimSpace(key[i+1:])
	}
	for _, a := range aliases {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		if key != "" && key == a {
			return true
		}
		if name == "" {
			continue
		}
		if name == a || name == a+"部" || name == a+"dept" {
			return true
		}
	}
	return false
}
