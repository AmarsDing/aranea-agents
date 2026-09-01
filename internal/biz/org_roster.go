package biz

import (
	"strings"
)

// DepartmentDomainPaths returns the specialty slots a department can cover.
// Keys are organization department keys from agency-pack / it-ops-pack.
// Unknown departments return nil — callers fall back to name aliases.
func DepartmentDomainPaths(deptKey, deptName string) []string {
	key := rosterLeaf(deptKey)
	if paths, ok := departmentDomainPaths[key]; ok {
		return append([]string(nil), paths...)
	}
	name := strings.ToLower(strings.TrimSpace(deptName))
	for k, paths := range departmentDomainPaths {
		if name != "" && strings.Contains(name, strings.ToLower(k)) {
			return append([]string(nil), paths...)
		}
	}
	return nil
}

// InferDomainPath picks the most specific specialty for a position in a department.
// Empty when nothing can be inferred (caller should leave the DB field empty).
func InferDomainPath(positionKey, departmentKey, displayName string) string {
	if p := positionDomainPath[rosterLeaf(positionKey)]; p != "" {
		return p
	}
	blob := strings.ToLower(rosterLeaf(positionKey) + " " + displayName)
	// Specific tokens beat the department default (纪要岗挂在项目管理部等)。
	if p := inferDomainOverride(blob); p != "" {
		return p
	}
	if paths := DepartmentDomainPaths(departmentKey, ""); len(paths) > 0 {
		return paths[0]
	}
	return inferDomainWeak(blob)
}

func inferDomainOverride(blob string) string {
	switch {
	case containsAny(blob, "xiaohongshu", "小红书", "douyin", "抖音", "kuaishou", "快手", "weibo", "微博", "zhihu", "知乎", "bilibili", "b站"):
		return "创作/文案"
	case containsAny(blob, "meeting_notes", "纪要", "document_generator"):
		return "办公/文档"
	case containsAny(blob, "alert_handler", "ops_alarm", "告警", "incident_commander"):
		return "运维/告警"
	case containsAny(blob, "fault_diagnostician", "ops_fault", "fault_diagnosis", "根因", "log_analyst", "ops_log_analysis", "metric_analyst"):
		return "运维/诊断"
	case containsAny(blob, "ops_network", "ops_auto_inspection", "ops_system_inspection", "network_inspector"):
		return "运维/巡检"
	case containsAny(blob, "ops_doc_generation", "document_generator"):
		return "办公/文档"
	case containsAny(blob, "book_co_author", "代笔"):
		return "创作/文学"
	case containsAny(blob, "ux_researcher", "trend_researcher"):
		return "研究/调研"
	case containsAny(blob, "data_engineer", "spatial_data"):
		return "数据/分析"
	}
	return ""
}

func inferDomainWeak(blob string) string {
	switch {
	case containsAny(blob, "backend", "后端", "golang"):
		return "软件/后端"
	case containsAny(blob, "frontend", "前端", "vue", "react"):
		return "软件/前端"
	case containsAny(blob, "测试", "压测"):
		return "软件/测试"
	case containsAny(blob, "sre", "devops"):
		return "软件/运维"
	case containsAny(blob, "文案", "copy", "种草"):
		return "创作/文案"
	}
	return ""
}

// InferToolsProfile chooses a legal tools profile from the specialty.
// Content/research roles get research; engineering/ops get coding; unknown stays coding.
func InferToolsProfile(domainPath string) string {
	top := domainPath
	if i := strings.Index(domainPath, "/"); i > 0 {
		top = domainPath[:i]
	}
	switch top {
	case "软件", "运维":
		return "coding"
	case "创作", "设计", "研究", "商务", "医疗", "数据", "办公":
		return "research"
	default:
		return "coding"
	}
}

// InferMissionStatement copies catalog description into mission_statement.
// DisplayName alone is not enough — a synthetic one-liner would make L1
// mission match fire on default success-rate (0.5) with near-zero similarity.
func InferMissionStatement(displayName, description string) string {
	desc := strings.TrimSpace(description)
	if desc == "" {
		return ""
	}
	if runes := []rune(desc); len(runes) > 160 {
		return string(runes[:160])
	}
	return desc
}

func rosterLeaf(path string) string {
	s := strings.TrimSpace(path)
	if s == "" {
		return ""
	}
	if i := strings.LastIndexAny(s, "/\\"); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(s)
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if n != "" && strings.Contains(s, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

var departmentDomainPaths = map[string][]string{
	"creative_planning":      {"研究/调研"},
	"brand_design":           {"设计/视觉"},
	"content_creation":       {"创作/文案", "创作/文学"},
	"media_operations":       {"创作/文案"},
	// media_studio（2026-09-01 S06 种子修复）：数字内容媒体公司媒体制作部。
	"media_studio":           {"媒体/制作"},
	"paid_promotion":         {"商务/推广"},
	"cross_border_ecommerce": {"商务/电商"},
	"sales_dept":             {"商务/销售"},
	"finance_dept":           {"商务/财务"},
	"customer_support":       {"商务/客服"},
	"special_services":       {"办公/专项"},
	"product_dept":           {"软件/产品"},
	"project_management":     {"软件/项目"},
	"backend_dev":            {"软件/后端"},
	"frontend_dev":           {"软件/前端"},
	"mobile_dev":             {"软件/移动"},
	"game_dev":               {"软件/游戏"},
	"spatial_computing":      {"软件/空间"},
	"quality_assurance":      {"软件/测试"},
	"ops":                    {"软件/运维"},
	"architecture":           {"软件/架构"},
	"security_dept":          {"软件/安全"},
	"compliance_audit":       {"软件/合规"},
	"gis_solutions":          {"数据/空间"},
	"clinical_evidence":      {"医疗/临床"},
	"medical_innovation":     {"医疗/创新"},
	"sovereign_health":       {"医疗/公卫"},
	"alert_response":         {"运维/告警"},
	"diagnostics":            {"运维/诊断"},
	"execution":              {"运维/变更"},
	"inspection":             {"运维/巡检"},
	"docs":                   {"运维/复盘", "办公/文档"},
}

var positionDomainPath = map[string]string{
	"xiaohongshu_specialist":         "创作/文案",
	"douyin_strategist":              "创作/文案",
	"tiktok_strategist":              "创作/文案",
	"weibo_strategist":               "创作/文案",
	"wechat_official_account":        "创作/文案",
	"zhihu_strategist":               "创作/文案",
	"bilibili_content_strategist":    "创作/文案",
	"kuaishou_strategist":            "创作/文案",
	"instagram_curator":              "创作/文案",
	"linkedin_content_creator":       "创作/文案",
	"content_creator":                "创作/文案",
	"social_media_strategist":        "创作/文案",
	"seo_specialist":                 "创作/文案",
	"baidu_seo_specialist":           "创作/文案",
	"book_co_author":                 "创作/文学",
	"podcast_strategist":             "创作/文学",
	"global_podcast_strategist":      "创作/文学",
	"ui_designer":                    "设计/视觉",
	"visual_storyteller":             "设计/视觉",
	"brand_guardian":                 "设计/视觉",
	"ux_researcher":                  "研究/调研",
	"trend_researcher":               "研究/调研",
	"anthropologist":                 "研究/调研",
	"statistician":                   "研究/调研",
	"backend_architect":              "软件/后端",
	"api_platform_engineer":          "软件/后端",
	"ai_engineer":                    "软件/后端",
	"data_engineer":                  "数据/分析",
	"frontend_developer":             "软件/前端",
	"wechat_mini_program_developer":  "软件/前端",
	"mobile_app_builder":             "软件/移动",
	"mobile_release_engineer":        "软件/移动",
	"game_designer":                  "软件/游戏",
	"xr_immersive_developer":         "软件/空间",
	"visionos_spatial_engineer":      "软件/空间",
	"test_automation_engineer":       "软件/测试",
	"api_tester":                     "软件/测试",
	"sre":                            "软件/运维",
	"devops_automator":               "软件/运维",
	"incident_response_commander":    "软件/运维",
	"network_engineer":               "软件/运维",
	"software_architect":             "软件/架构",
	"appsec_engineer":                "软件/安全",
	"penetration_tester":             "软件/安全",
	"data_privacy_officer":           "软件/合规",
	"manager":                        "软件/产品",
	"project_manager_senior":         "软件/项目",
	"meeting_notes_specialist":       "办公/文档",
	"document_generator":             "办公/文档",
	"technical_writer":               "办公/文档",
	"alert_handler":                  "运维/告警",
	"incident_commander":             "运维/告警",
	"fault_diagnostician":            "运维/诊断",
	"log_analyst":                    "运维/诊断",
	"metric_analyst":                 "运维/诊断",
	"change_executor":                "运维/变更",
	"runbook_engineer":               "运维/变更",
	"system_inspector":               "运维/巡检",
	"network_inspector":              "运维/巡检",
	"db_operator":                    "运维/巡检",
	"postmortem_writer":              "运维/复盘",
	// Live it-ops pack keys (ops_*). Without these InferDomainPath stays
	// empty and plan_and_execute cannot bind 运维/* specialists.
	"ops_network_inspection": "运维/巡检",
	"ops_auto_inspection":    "运维/巡检",
	"ops_system_inspection":  "运维/巡检",
	"ops_fault_diagnosis":    "运维/诊断",
	"ops_log_analysis":       "运维/诊断",
	"ops_alarm_handler":      "运维/告警",
	"ops_change_execution":   "运维/变更",
	"ops_doc_generation":     "办公/文档",
	"ops_database":           "运维/巡检",
	"ops_compliance_check":   "软件/合规",
	"ops_command_expert":     "软件/运维",
	"ops_server_command":     "软件/运维",
	"financial_analyst":              "商务/财务",
	"fpa_analyst":                    "商务/财务",
	"outbound_strategist":            "商务/销售",
	"sales_outreach":                 "商务/销售",
	"support_responder":              "商务/客服",
	"customer_service":               "商务/客服",
	"ppc_strategist":                 "商务/推广",
	"paid_social_strategist":         "商务/推广",
	"cross_border_specialist":        "商务/电商",
	"china_ecommerce_operator":       "商务/电商",
	"clinical_evidence_agent":        "医疗/临床",
	"innovation_strategist":          "医疗/创新",
	"sovereign_health_systems_agent": "医疗/公卫",
	"spatial_data_scientist":         "数据/空间",
	"spatial_data_engineer":          "数据/空间",
	"web_gis_developer":              "数据/空间",
	"analytics_reporter":             "数据/分析",
	// 媒体/制作（2026-09-01 S06 种子修复）：media_studio 部门岗位。
	"demo_video_producer":            "媒体/制作",
	"video_editor":                   "媒体/制作",
}
