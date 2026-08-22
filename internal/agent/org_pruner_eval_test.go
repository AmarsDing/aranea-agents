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
		{AgentKey: "be-1", DepartmentID: "d-eng", DepartmentName: "研发部"},
		{AgentKey: "fe-1", DepartmentID: "d-eng", DepartmentName: "研发部"},
		{AgentKey: "copy-1", DepartmentID: "d-media", DepartmentName: "内容运营部"},
		{AgentKey: "writer-1", DepartmentID: "d-media", DepartmentName: "内容运营部"},
		{AgentKey: "design-1", DepartmentID: "d-design", DepartmentName: "设计部"},
		{AgentKey: "data-1", DepartmentID: "d-data", DepartmentName: "数据分析部"},
		{AgentKey: "research-1", DepartmentID: "d-research", DepartmentName: "研究部"},
		{AgentKey: "admin-1", DepartmentID: "d-admin", DepartmentName: "行政综合部"},
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
