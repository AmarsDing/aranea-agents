package agent

import (
	"testing"

	"aranea-agents/internal/biz"
)

// 20 fixed Chinese tasks for M78 ORGFAST-14. DomainPath is the planner
// output (lexicon-normalized); OrgPruner must pick the expected department
// name as Top-1. Dept leads are in the catalog and must never appear in
// CandidateKeys.
func TestOrgPruner_EvalFixture_DepartmentTop1(t *testing.T) {
	caps := []biz.AgentCapability{
		{AgentKey: "be-1", DepartmentID: "d-eng", DepartmentKey: "backend_dev", DepartmentName: "研发部", DomainPath: "软件/后端"},
		{AgentKey: "fe-1", DepartmentID: "d-eng", DepartmentKey: "frontend_dev", DepartmentName: "研发部", DomainPath: "软件/前端"},
		{AgentKey: "copy-1", DepartmentID: "d-media", DepartmentKey: "media_operations", DepartmentName: "内容运营部", DomainPath: "创作/文案"},
		{AgentKey: "writer-1", DepartmentID: "d-media", DepartmentKey: "content_creation", DepartmentName: "内容运营部", DomainPath: "创作/文学"},
		{AgentKey: "design-1", DepartmentID: "d-design", DepartmentKey: "brand_design", DepartmentName: "设计部", DomainPath: "设计/视觉"},
		{AgentKey: "data-1", DepartmentID: "d-data", DepartmentName: "数据分析部", DomainPath: "数据/分析"},
		{AgentKey: "research-1", DepartmentID: "d-research", DepartmentName: "研究部", DomainPath: "研究/调研"},
		{AgentKey: "admin-1", DepartmentID: "d-admin", DepartmentName: "行政综合部", DomainPath: "办公/文档"},
		{AgentKey: biz.DeptLeadAgentKeyPrefix + "eng__", AgentVariant: "dept_lead", DepartmentID: "d-eng", DepartmentName: "研发部"},
		{AgentKey: biz.DeptLeadAgentKeyPrefix + "media__", AgentVariant: "dept_lead", DepartmentID: "d-media", DepartmentName: "内容运营部"},
	}

	cases := []struct {
		task       string
		domainPath string
		wantDept   string
	}{
		{"写一个 Go REST API", "软件/后端", "研发部"},
		{"给官网做 Vue 首页", "软件/前端", "研发部"},
		{"补集成测试", "软件/测试", "研发部"},
		{"把服务部署到 k8s", "软件/运维", "研发部"},
		{"分析用户留存漏斗", "数据/分析", "数据分析部"},
		{"写一篇品牌故事", "创作/文学", "内容运营部"},
		{"写小红书种草文案", "创作/文案", "内容运营部"},
		{"出一套活动主视觉", "设计/视觉", "设计部"},
		{"调研竞品定价", "研究/调研", "研究部"},
		{"整理会议纪要成文档", "办公/文档", "行政综合部"},
		{"后端接口鉴权改造", "软件/后端", "研发部"},
		{"前端表格筛选组件", "软件/前端", "研发部"},
		{"压测报告", "软件/测试", "研发部"},
		{"日志采集告警", "软件/运维", "研发部"},
		{"SQL 周报", "数据/分析", "数据分析部"},
		{"公众号长文", "创作/文学", "内容运营部"},
		{"促销 slogan", "创作/文案", "内容运营部"},
		{"APP 图标", "设计/视觉", "设计部"},
		{"用户访谈纪要分析", "研究/调研", "研究部"},
		{"员工手册修订", "办公/文档", "行政综合部"},
	}

	p := OrgPruner{}
	hit := 0
	for _, tc := range cases {
		got := p.Prune(tc.domainPath, caps)
		if got.FallbackAll || got.DepartmentName != tc.wantDept {
			t.Errorf("%q domain=%s: dept=%q fallback=%v reason=%s want %s",
				tc.task, tc.domainPath, got.DepartmentName, got.FallbackAll, got.Reason, tc.wantDept)
			continue
		}
		hit++
		for _, k := range got.CandidateKeys {
			if biz.IsDeptLeadAgent(biz.Agent{AgentKey: k}) {
				t.Errorf("%q selected dept_lead %s as candidate", tc.task, k)
			}
		}
	}
	if ratio := float64(hit) / float64(len(cases)); ratio < 0.90 {
		t.Fatalf("department Top-1 ratio=%.2f want >= 0.90 (%d/%d)", ratio, hit, len(cases))
	}
}

func TestMatchDepartmentAlias_ExactNotContains(t *testing.T) {
	aliases := []string{"运营", "媒体", "研发"}
	if matchDepartmentAlias("内容运营部", "", aliases) {
		t.Fatal("Chinese name Contains must not match 运营 inside 内容运营部")
	}
	if !matchDepartmentAlias("研发部", "", aliases) {
		t.Fatal("研发 + 部 must still match 研发部")
	}
	if !matchDepartmentAlias("", "media_operations", []string{"media_operations"}) {
		t.Fatal("org key exact match must hit")
	}
}

func TestOrgPruner_RealPackDepartmentKeys(t *testing.T) {
	caps := []biz.AgentCapability{
		{AgentKey: "xhs", DepartmentID: "d-media", DepartmentKey: "media_operations", DepartmentName: "媒体运营部", PositionKey: "xiaohongshu_specialist", DomainPath: "创作/文案", DisplayName: "小红书运营专家"},
		{AgentKey: "be", DepartmentID: "d-be", DepartmentKey: "backend_dev", DepartmentName: "后端开发部", PositionKey: "backend_architect", DomainPath: "软件/后端"},
		{AgentKey: "ops", DepartmentID: "d-ops", DepartmentKey: "ops", DepartmentName: "运维部", PositionKey: "sre", DomainPath: "软件/运维"},
		{AgentKey: "alert", DepartmentID: "d-alert", DepartmentKey: "alert_response", DepartmentName: "告警响应部", PositionKey: "alert_handler", DomainPath: "运维/告警"},
		{AgentKey: "fault", DepartmentID: "d-diag", DepartmentKey: "diagnostics", DepartmentName: "故障诊断部", PositionKey: "fault_diagnostician", DomainPath: "运维/诊断"},
	}
	p := OrgPruner{}
	cases := []struct {
		domain string
		dept   string
	}{
		{"创作/文案", "媒体运营部"},
		{"软件/后端", "后端开发部"},
		{"软件/运维", "运维部"},
		{"运维/告警", "告警响应部"},
		{"运维/诊断", "故障诊断部"},
	}
	for _, tc := range cases {
		got := p.Prune(tc.domain, caps)
		if got.FallbackAll || got.DepartmentName != tc.dept {
			t.Errorf("domain=%s dept=%q fallback=%v reason=%s want %s",
				tc.domain, got.DepartmentName, got.FallbackAll, got.Reason, tc.dept)
		}
	}
}
