package orggraph

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// 与 web/src/features/company/__tests__/tree.spec.ts 同一组数据：两边的口径钉在一起。

var cst = time.FixedZone("CST", 8*3600)
var now = time.Date(2026, 11, 16, 10, 0, 0, 0, cst)

func at(h int, day ...int) string {
	d := 16
	if len(day) > 0 {
		d = day[0]
	}
	return time.Date(2026, 11, d, h, 0, 0, 0, cst).Format(time.RFC3339)
}

func node(id, level string, mod func(*OrgNode), children ...OrgNode) OrgNode {
	n := OrgNode{ID: id, Name: id, Level: level, Enabled: true, Children: children}
	if mod != nil {
		mod(&n)
	}
	return n
}

func agent(id, pos string, mod func(*Agent)) Agent {
	a := Agent{ID: id, Key: "key-" + id, Name: "名-" + id, PositionID: pos}
	if mod != nil {
		mod(&a)
	}
	return a
}

func work(id, assignee, state, kind string) WorkItem {
	return WorkItem{ID: id, Title: id, AssigneeKind: kind, AssigneeID: assignee, State: state, At: at(9), To: "/chat"}
}

func fixture() Input {
	orgs := []OrgNode{
		node("co-1", "company", func(n *OrgNode) {
			n.Name = "一号公司"
			n.MetadataJSON = `{"company_lead_agent_id":"ceo"}`
		},
			node("d-content", "department", func(n *OrgNode) { n.Name, n.DeptLeadAgentID, n.SortOrder = "内容部", "lead-c", 2 },
				node("p-write", "position", func(n *OrgNode) { n.Name, n.SortOrder = "写手", 1 }),
				node("p-review", "position", func(n *OrgNode) { n.Name, n.SortOrder = "评审", 2 }),
				node("p-off", "position", func(n *OrgNode) { n.Name, n.Enabled = "停用岗", false }),
			),
			node("d-trade", "department", func(n *OrgNode) { n.Name, n.DeptLeadAgentID, n.SortOrder = "交易部", "gone", 1 },
				node("p-quant", "position", nil),
			),
			node("d-empty", "department", func(n *OrgNode) { n.Name, n.SortOrder = "空部门", 3 }),
		),
		node("co-2", "company", func(n *OrgNode) { n.Name, n.SortOrder = "二号公司", 1 },
			node("d-x", "department", nil, node("p-x", "position", nil)),
		),
	}
	agents := []Agent{
		agent("ceo", "", nil),
		agent("lead-c", "d-content", func(a *Agent) { a.LastRunAt = at(8) }),
		agent("w1", "p-write", func(a *Agent) { a.LastRunAt = at(9) }),
		agent("w2", "p-write", func(a *Agent) { a.LastRunAt = at(9, 15) }),
		agent("r1", "p-review", func(a *Agent) { a.LastRunStatus, a.LastRunAt = "failed", at(7) }),
		agent("q1", "p-quant", func(a *Agent) { a.LastRunAt = at(9, 14) }),
		agent("dead", "p-quant", func(a *Agent) { a.DeletedAt = at(1) }),
		agent("x1", "p-x", nil),
		agent("loose", "", nil),
	}
	return Input{
		Orgs:   orgs,
		Agents: agents,
		Inbox: []InboxItem{
			{ID: "i1", Title: "调仓", AgentKey: "key-w1", DueAt: at(18), RiskLevel: "high"},
			{ID: "i2", Title: "选题", AgentKey: "key-w1", DueAt: at(12), RiskLevel: "low"},
			{ID: "i3", Title: "没归属", RiskLevel: "medium"},
		},
		InboxTotal: 5,
		Work: []WorkItem{
			work("t1", "w2", WorkRunning, "agent"),
			work("t2", "lead-c", WorkReview, "agent"),
			work("t3", "x1", WorkStuck, "agent"),
			work("t4", "w2", WorkRunning, "team"),
		},
		Cron:  []Cron{{AgentID: "w1", NextRunAt: at(14)}, {AgentID: "w2", NextRunAt: at(12)}, {AgentID: "w2", NextRunAt: at(6)}},
		Usage: []Usage{{AgentID: "q1", Calls: 3, CostMicro: 120_000}},
		Now:   now,
	}
}

func ids(list []BoardAgent) []string {
	out := []string{}
	for _, a := range list {
		out = append(out, a.ID)
	}
	return out
}

func TestBuild_CompaniesAndCEO(t *testing.T) {
	b := Build(fixture())
	if len(b.Companies) != 2 || b.Companies[0].Name != "一号公司" || b.Companies[1].Name != "二号公司" {
		t.Fatalf("companies = %+v", b.Companies)
	}
	if b.Companies[0].CEO == nil || b.Companies[0].CEO.ID != "ceo" {
		t.Fatalf("ceo = %+v", b.Companies[0].CEO)
	}
	if b.Companies[1].CEO != nil || b.Companies[1].CEOConfigured {
		t.Fatalf("co-2 ceo should be empty")
	}
}

func TestBuild_DeptMembers(t *testing.T) {
	co := Build(fixture()).Companies[0]
	var names []string
	for _, d := range co.Depts {
		names = append(names, d.Name)
	}
	if !reflect.DeepEqual(names, []string{"交易部", "内容部", "空部门"}) {
		t.Fatalf("dept order = %v", names)
	}
	content := co.Depts[1]
	got := ids(content.Agents)
	want := map[string]bool{"lead-c": true, "r1": true, "w1": true, "w2": true}
	if len(got) != 4 {
		t.Fatalf("content agents = %v", got)
	}
	for _, id := range got {
		if !want[id] {
			t.Fatalf("unexpected member %s", id)
		}
	}
	if content.Positions != 2 || content.EmptyPositions != 0 || content.Lead == nil || content.Lead.ID != "lead-c" {
		t.Fatalf("content = %+v", content)
	}
	trade := co.Depts[0]
	if !trade.LeadMissing || !reflect.DeepEqual(ids(trade.Agents), []string{"q1"}) {
		t.Fatalf("trade = %+v", trade)
	}
	if co.Depts[2].State != StateEmpty {
		t.Fatalf("empty dept state = %s", co.Depts[2].State)
	}
}

func TestBuild_LoopRollup(t *testing.T) {
	b := Build(fixture())
	co1, co2 := b.Companies[0], b.Companies[1]
	if want := (Loop{Waiting: 2, Running: 1, Review: 1}); co1.Depts[1].Loop != want || co1.Loop != want {
		t.Fatalf("co1 loop = %+v / %+v", co1.Depts[1].Loop, co1.Loop)
	}
	if want := (Loop{Stuck: 1}); co2.Loop != want {
		t.Fatalf("co2 loop = %+v", co2.Loop)
	}
	if want := (Loop{Waiting: 5, Running: 2, Stuck: 1, Review: 1}); b.Me.Loop != want {
		t.Fatalf("me loop = %+v", b.Me.Loop)
	}
	if b.Me.HighRisk != 1 || b.Me.EarliestDue != at(12) || b.Me.Unattributed != 1 || b.Me.Unassigned != 1 {
		t.Fatalf("me = %+v", b.Me)
	}
}

func TestBuild_States(t *testing.T) {
	co1 := Build(fixture()).Companies[0]
	if co1.Depts[1].State != StateAttention {
		t.Fatalf("content state = %s", co1.Depts[1].State)
	}
	trade := co1.Depts[0]
	if trade.State != StateOK || trade.ActiveToday != 1 || trade.Calls != 3 || co1.CostMicro != 120_000 {
		t.Fatalf("trade = %+v", trade)
	}
	content := co1.Depts[1]
	if content.Agents[0].State != StateAttention {
		t.Fatalf("first agent should need attention: %+v", content.Agents[0])
	}
	for _, a := range content.Agents {
		if a.ID == "w2" && a.State != StateOK {
			t.Fatalf("w2 state = %s", a.State)
		}
	}
}

func TestBuild_LastNextAndPosition(t *testing.T) {
	content := Build(fixture()).Companies[0].Depts[1]
	if content.LastRunAt != at(9) || content.NextRunAt != at(12) {
		t.Fatalf("last/next = %s / %s", content.LastRunAt, content.NextRunAt)
	}
	for _, a := range content.Agents {
		if a.ID == "w1" && a.Position != "写手" {
			t.Fatalf("w1 position = %q", a.Position)
		}
	}
}

func TestBuild_Empty(t *testing.T) {
	b := Build(Input{Now: now})
	if len(b.Companies) != 0 || b.Me.Loop != (Loop{}) {
		t.Fatalf("empty board = %+v", b)
	}
	if b.Me.WaitingItems == nil || b.Me.WorkItems == nil || b.Companies == nil {
		t.Fatal("slices must encode as [] not null")
	}
}

func TestCompanyLeadID(t *testing.T) {
	if CompanyLeadID(`{"company_lead_agent_id":" a "}`) != "a" || CompanyLeadID("not json") != "" || CompanyLeadID("") != "" {
		t.Fatal("CompanyLeadID")
	}
}

func TestPatrolAndReport(t *testing.T) {
	b := Build(fixture())
	p := PatrolCompany(b.Companies[0], now)
	if p.State != StateAttention || p.Loop.Waiting != 2 || p.Failed != 1 {
		t.Fatalf("patrol = %+v", p)
	}
	var texts []string
	for _, f := range p.Findings {
		texts = append(texts, f.Text)
	}
	joined := strings.Join(texts, "\n")
	for _, want := range []string{"名-r1 最近一次运行失败", "等你拍板（高风险）：调仓", "等你拍板（12:00 到期）：选题"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("findings missing %q:\n%s", want, joined)
		}
	}
	if c := Conclusion(p); c != "要看一眼：等你拍板 2 件、待验收 1 件、失败 1 个" {
		t.Fatalf("conclusion = %q", c)
	}
	prev := p
	prev.Loop.Waiting = 0
	reasons := Reasons(p, &prev, at(8), now)
	if len(reasons) == 0 || len(reasons) > maxReasons || reasons[len(reasons)-1] != "和上次汇报（08:00）比：等你拍板 0 → 2" {
		t.Fatalf("reasons = %v", reasons)
	}
	prompt := ReportPrompt(p, Conclusion(p), reasons, now)
	if !strings.Contains(prompt, "【定时汇报 · 一号公司 · 10:00】") || !strings.Contains(prompt, "结论：<一句话>") {
		t.Fatalf("prompt = %s", prompt)
	}

	// 二号公司：只有一件卡住的交办。
	p2 := PatrolCompany(b.Companies[1], now)
	if len(p2.Findings) != 1 || !strings.Contains(p2.Findings[0].Text, "交办「t3」给名-x1，卡住了（从 09:00 起）") {
		t.Fatalf("co2 findings = %+v", p2.Findings)
	}
}

func TestConclusionNoIssues(t *testing.T) {
	cases := []struct {
		p    Patrol
		want string
	}{
		{Patrol{AgentCount: 14, ActiveToday: 5, Loop: Loop{Running: 2}, CostMicro: 515_000}, "正常：今天 5/14 跑过，2 件在做，花费 $0.52"},
		{Patrol{AgentCount: 14}, "今天没动静：14 个 Agent 在岗，今天都没跑过"},
		{Patrol{}, "没人在岗"},
	}
	for _, c := range cases {
		if got := Conclusion(c.p); got != c.want {
			t.Fatalf("Conclusion = %q, want %q", got, c.want)
		}
	}
	if r := Reasons(Patrol{AgentCount: 1}, nil, "", now); len(r) != 1 {
		t.Fatalf("no findings still gives one reason: %v", r)
	}
}

func TestParseReport(t *testing.T) {
	reply := "**结论：** 要看一眼，内容部有两件等你拍板\n\n理由：\n- 调仓是高风险\n2. 选题 12:00 到期\n• 名-r1 早上失败了\n随便一句不是列表"
	c, r, ok := ParseReport(reply)
	if !ok || c != "要看一眼，内容部有两件等你拍板" {
		t.Fatalf("conclusion = %q ok=%v", c, ok)
	}
	if !reflect.DeepEqual(r, []string{"调仓是高风险", "选题 12:00 到期", "名-r1 早上失败了"}) {
		t.Fatalf("reasons = %v", r)
	}
	if _, _, ok := ParseReport("好的，我来看看"); ok {
		t.Fatal("reply without 结论 must not parse")
	}
	if c, _, ok := ParseReport("结论: 正常"); !ok || c != "正常" {
		t.Fatalf("ascii colon: %q", c)
	}
}
