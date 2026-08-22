package agent

import (
	"testing"

	"aranea-agents/internal/biz"
)

// Fixed specialists used by the 原话→专题→人 eval. dept_lead is present
// to prove it is never chosen.
var evalRoster = []biz.AgentCapability{
	{AgentKey: "spec-be", DisplayName: "后端专项", DomainPath: "软件/后端"},
	{AgentKey: "spec-fe", DisplayName: "前端专项", DomainPath: "软件/前端"},
	{AgentKey: "spec-qa", DisplayName: "测试专项", DomainPath: "软件/测试"},
	{AgentKey: "spec-ops", DisplayName: "运维专项", DomainPath: "软件/运维"},
	{AgentKey: "spec-da", DisplayName: "数据分析专项", DomainPath: "数据/分析"},
	{AgentKey: "spec-lit", DisplayName: "文学专项", DomainPath: "创作/文学"},
	{AgentKey: "spec-copy", DisplayName: "文案专项", DomainPath: "创作/文案"},
	{AgentKey: "spec-des", DisplayName: "视觉专项", DomainPath: "设计/视觉"},
	{AgentKey: "spec-rs", DisplayName: "调研专项", DomainPath: "研究/调研"},
	{AgentKey: "spec-doc", DisplayName: "文档专项", DomainPath: "办公/文档"},
	{AgentKey: biz.DeptLeadAgentKeyPrefix + "eng__", DisplayName: "研发主管", AgentVariant: "dept_lead", DomainPath: "软件/后端"},
}

func TestEval_OriginalTextToSpecialtyToPerson(t *testing.T) {
	cases := []struct {
		task     string
		wantSpec string
		wantKey  string
	}{
		{"用 Go 写 REST 接口并做鉴权", "软件/后端", "spec-be"},
		{"Vue 表格组件加筛选", "软件/前端", "spec-fe"},
		{"补接口压测用例", "软件/测试", "spec-qa"},
		{"k8s 部署加告警", "软件/运维", "spec-ops"},
		{"漏斗留存 SQL 分析", "数据/分析", "spec-da"},
		{"写一篇品牌故事长文", "创作/文学", "spec-lit"},
		{"小红书种草文案和 slogan", "创作/文案", "spec-copy"},
		{"做一套主视觉图标", "设计/视觉", "spec-des"},
		{"竞品访谈纪要", "研究/调研", "spec-rs"},
		{"整理会议纪要成文档", "办公/文档", "spec-doc"},
		{"官网首页前端改版", "软件/前端", "spec-fe"},
		{"Go 鉴权中间件", "软件/后端", "spec-be"},
		{"公众号品牌故事", "创作/文学", "spec-lit"},
		{"小红书种草文案", "创作/文案", "spec-copy"},
		{"主视觉设计", "设计/视觉", "spec-des"},
		{"用户访谈调研", "研究/调研", "spec-rs"},
		{"操作手册文档", "办公/文档", "spec-doc"},
		{"漏斗分析 SQL", "数据/分析", "spec-da"},
		{"日志告警部署", "软件/运维", "spec-ops"},
		{"写一首诗", "创作/文学", "spec-lit"},
	}
	if len(cases) != 20 {
		t.Fatalf("eval cases=%d want 20", len(cases))
	}
	for _, tc := range cases {
		spec := InferSpecialtyFromTask(tc.task)
		if spec != tc.wantSpec {
			t.Errorf("%q specialty=%q want %q", tc.task, spec, tc.wantSpec)
			continue
		}
		got, _, ok := bindRosterSpecialist(spec, evalRoster)
		if !ok {
			t.Errorf("%q roster miss for %s", tc.task, spec)
			continue
		}
		if got.AgentKey != tc.wantKey {
			t.Errorf("%q person=%q want %q", tc.task, got.AgentKey, tc.wantKey)
		}
		if biz.IsDeptLeadAgent(biz.Agent{AgentKey: got.AgentKey, AgentVariant: got.AgentVariant}) {
			t.Errorf("%q assigned dept_lead", tc.task)
		}
	}
}
