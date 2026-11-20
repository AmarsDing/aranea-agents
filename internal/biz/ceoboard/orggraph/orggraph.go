// Package orggraph 把组织树、Agent 名册、收件箱、交办登记、定时任务、今日用量
// 收敛成看板总览那张图：我 → 公司（含总经理 Agent）→ 部门 → Agent。
//
// 口径与前端 web/src/features/company/tree.ts（buildBoard）一一对应，前端在
// 接口不可用时仍用 tree.ts 回落拼装，两边改一处必须同步另一处。
// 输出字段名也与 tree.ts 的 Board 相同，前端可以直接使用。
//
// 每个节点讲同一个闭环（设计稿 03-看板 §13.2）：
//
//	waiting 等你拍板：收件箱里由这个节点的 Agent 提出的事
//	running 在做：交给这个节点 Agent 的事，执行中
//	stuck   卡住：交办的事停在等待 / 失败 / 取消
//	review  待验收：交办的事做完了，等你验收
//
// 本包是纯函数，不读库、不依赖 proto。
package orggraph

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"
)

// 节点状态，按严重程度取最重的一种。
const (
	StateAttention = "attention" // 有事等我（拍板 / 验收），或有事卡住、有 Agent 最近一次运行失败
	StateOK        = "ok"        // 今天有 Agent 跑过，或有事在做
	StateIdle      = "idle"      // 有 Agent 在岗，但今天没动静
	StateEmpty     = "empty"     // 没有 Agent 在岗
)

// 交办三态。
const (
	WorkRunning = "running"
	WorkStuck   = "stuck"
	WorkReview  = "review"
)

// OrgNode 是组织树的一个节点（company / department / position）。
type OrgNode struct {
	ID              string
	Name            string
	Level           string
	Enabled         bool
	SortOrder       int
	DeletedAt       string
	DeptLeadAgentID string
	MetadataJSON    string
	Children        []OrgNode
}

// Agent 是名册里的一个 Agent。时间均为 RFC3339 文本。
type Agent struct {
	ID            string
	Key           string
	Name          string
	PositionID    string // 所挂岗位（部门负责人可能直接挂部门节点）
	LastRunStatus string
	LastRunAt     string
	DeletedAt     string
}

// InboxItem 是一条等我拍板的事（字段名同 tree.ts 的 InboxInput）。
type InboxItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	AgentKey  string `json:"agent_key"`
	DueAt     string `json:"due_at"`
	RiskLevel string `json:"risk_level"`
	Link      string `json:"link,omitempty"`
}

// WorkItem 是一条已归成三态的交办（字段名同 tree.ts 的 WorkInput）。
type WorkItem struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	AssigneeKind string `json:"assignee_kind"`
	AssigneeID   string `json:"assignee_id"`
	State        string `json:"state"`
	At           string `json:"at"`
	To           string `json:"to"`
}

// Cron 是一条启用中的定时任务的下次运行时间。
type Cron struct {
	AgentID   string
	NextRunAt string
}

// Usage 是一个 Agent 的今日用量。
type Usage struct {
	AgentID   string
	Calls     int64
	CostMicro int64
}

// Input 是 Build 的全部输入。
type Input struct {
	Orgs       []OrgNode
	Agents     []Agent
	Inbox      []InboxItem
	InboxTotal int
	Work       []WorkItem
	Cron       []Cron
	Usage      []Usage
	Now        time.Time
}

// Loop 是闭环四格。
type Loop struct {
	Waiting int `json:"waiting"`
	Running int `json:"running"`
	Stuck   int `json:"stuck"`
	Review  int `json:"review"`
}

// BoardAgent 是图上的一个 Agent。
type BoardAgent struct {
	ID           string      `json:"id"`
	Key          string      `json:"key"`
	Name         string      `json:"name"`
	Position     string      `json:"position"`
	Failed       bool        `json:"failed"`
	RanToday     bool        `json:"ranToday"`
	LastRunAt    string      `json:"lastRunAt"`
	NextRunAt    string      `json:"nextRunAt"`
	Calls        int64       `json:"calls"`
	CostMicro    int64       `json:"costMicro"`
	State        string      `json:"state"`
	Loop         Loop        `json:"loop"`
	WaitingItems []InboxItem `json:"waitingItems"`
	WorkItems    []WorkItem  `json:"workItems"`
}

// Rollup 是一组 Agent 汇总成的节点口径。
type Rollup struct {
	AgentCount   int         `json:"agentCount"`
	ActiveToday  int         `json:"activeToday"`
	Failed       int         `json:"failed"`
	Calls        int64       `json:"calls"`
	CostMicro    int64       `json:"costMicro"`
	Loop         Loop        `json:"loop"`
	State        string      `json:"state"`
	LastRunAt    string      `json:"lastRunAt"`
	NextRunAt    string      `json:"nextRunAt"`
	WaitingItems []InboxItem `json:"waitingItems"`
	WorkItems    []WorkItem  `json:"workItems"`
}

// DeptNode 是一个部门。
type DeptNode struct {
	ID   string      `json:"id"`
	Name string      `json:"name"`
	Lead *BoardAgent `json:"lead"`
	// LeadMissing 配置了负责人 id 但名册里找不到（被删或无权查看）。
	LeadMissing    bool `json:"leadMissing"`
	Positions      int  `json:"positions"`
	EmptyPositions int  `json:"emptyPositions"`
	// Agents 有问题的排前面，其次今天跑过的；同档保持组织树里的顺序。
	Agents []BoardAgent `json:"agents"`
	Rollup
}

// CompanyNode 是一家公司。
type CompanyNode struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	CEO           *BoardAgent `json:"ceo"`
	CEOConfigured bool        `json:"ceoConfigured"`
	Depts         []DeptNode  `json:"depts"`
	// GM 是总经理的巡视与汇报（由装载层按定时任务记录填入；Build 不填）。
	GM *GMStatus `json:"gm,omitempty"`
	Rollup
}

// Me 是「我」这个顶层节点。
type Me struct {
	Loop         Loop        `json:"loop"`
	HighRisk     int         `json:"highRisk"`
	EarliestDue  string      `json:"earliestDue"`
	WaitingItems []InboxItem `json:"waitingItems"`
	WorkItems    []WorkItem  `json:"workItems"`
	// Unattributed 归不到任何公司的待办（来源不带 Agent，或 Agent 不在组织里）。
	Unattributed int `json:"unattributed"`
	// Unassigned 名册里有、但没挂到任何公司的 Agent。
	Unassigned  int   `json:"unassigned"`
	AgentCount  int   `json:"agentCount"`
	ActiveToday int   `json:"activeToday"`
	Calls       int64 `json:"calls"`
	CostMicro   int64 `json:"costMicro"`
}

// Board 是整张图。
type Board struct {
	Companies []CompanyNode `json:"companies"`
	Me        Me            `json:"me"`
}

// GMStatus 是一家公司总经理的巡视与汇报状态。
type GMStatus struct {
	// Enabled 巡视与汇报的定时任务至少有一条在启用。
	Enabled      bool      `json:"enabled"`
	LastPatrolAt string    `json:"lastPatrolAt"`
	NextPatrolAt string    `json:"nextPatrolAt"`
	LastReport   *GMReport `json:"lastReport,omitempty"`
	NextReportAt string    `json:"nextReportAt"`
}

// GMReport 是最近一次汇报。
type GMReport struct {
	RunID      string   `json:"runId"`
	At         string   `json:"at"`
	Conclusion string   `json:"conclusion"`
	Reasons    []string `json:"reasons"`
	// Writer model = 总经理的模型写的；rule = 模型没回答，按规则生成。
	Writer string `json:"writer"`
	Acked  bool   `json:"acked"`
}

var stateRank = map[string]int{StateAttention: 0, StateOK: 1, StateIdle: 2, StateEmpty: 3}

var failedRe = regexp.MustCompile(`(?i)fail|error`)

// ParseTime 解析接口里出现的几种时间文本；无效返回零值。
func ParseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05Z07:00", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func later(a, b string) string {
	if ParseTime(b).After(ParseTime(a)) {
		return b
	}
	return a
}

func earlier(a, b string) string {
	tb := ParseTime(b)
	if tb.IsZero() {
		return a
	}
	ta := ParseTime(a)
	if ta.IsZero() || tb.Before(ta) {
		return b
	}
	return a
}

// SameDay 报告 iso 是否与 now 在 now 所在时区的同一天。
func SameDay(iso string, now time.Time) bool {
	t := ParseTime(iso)
	if t.IsZero() {
		return false
	}
	t = t.In(now.Location())
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

// CompanyLeadID 取公司节点 metadata_json.company_lead_agent_id。
func CompanyLeadID(metadataJSON string) string {
	if strings.TrimSpace(metadataJSON) == "" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(metadataJSON), &m); err != nil {
		return ""
	}
	id, _ := m["company_lead_agent_id"].(string)
	return strings.TrimSpace(id)
}

func stateOf(agentCount, activeToday, failed int, loop Loop) string {
	needsMe := loop.Waiting+loop.Stuck+loop.Review > 0
	if agentCount == 0 {
		if needsMe {
			return StateAttention
		}
		return StateEmpty
	}
	if failed > 0 || needsMe {
		return StateAttention
	}
	if activeToday > 0 || loop.Running > 0 {
		return StateOK
	}
	return StateIdle
}

func alive(n OrgNode) bool {
	return n.ID != "" && n.DeletedAt == "" && n.Enabled
}

func sortNodes(list []OrgNode) {
	// shortcut: 同序号按名字的字节序排，前端 localeCompare 对中文的次序可能不同；要严格一致时两边改成同一把比较器。
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].SortOrder != list[j].SortOrder {
			return list[i].SortOrder < list[j].SortOrder
		}
		return list[i].Name < list[j].Name
	})
}

// collect 取某层级的直接后代；容忍公司下直接挂岗位、部门下再套部门的树。
func collect(root OrgNode, level string) []OrgNode {
	var out []OrgNode
	var walk func(n OrgNode)
	walk = func(n OrgNode) {
		for _, c := range n.Children {
			if !alive(c) {
				continue
			}
			if c.Level == level {
				out = append(out, c)
			} else {
				walk(c)
			}
		}
	}
	walk(root)
	sortNodes(out)
	return out
}

func rollup(members []*BoardAgent) Rollup {
	r := Rollup{AgentCount: len(members), WaitingItems: []InboxItem{}, WorkItems: []WorkItem{}}
	for _, a := range members {
		r.Loop.Waiting += a.Loop.Waiting
		r.Loop.Running += a.Loop.Running
		r.Loop.Stuck += a.Loop.Stuck
		r.Loop.Review += a.Loop.Review
		r.WaitingItems = append(r.WaitingItems, a.WaitingItems...)
		r.WorkItems = append(r.WorkItems, a.WorkItems...)
		if a.RanToday {
			r.ActiveToday++
		}
		if a.Failed {
			r.Failed++
		}
		r.Calls += a.Calls
		r.CostMicro += a.CostMicro
		r.LastRunAt = later(r.LastRunAt, a.LastRunAt)
		r.NextRunAt = earlier(r.NextRunAt, a.NextRunAt)
	}
	r.State = stateOf(r.AgentCount, r.ActiveToday, r.Failed, r.Loop)
	return r
}

func uniq(list ...*BoardAgent) []*BoardAgent {
	seen := map[string]bool{}
	out := make([]*BoardAgent, 0, len(list))
	for _, a := range list {
		if a == nil || seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		out = append(out, a)
	}
	return out
}

func countWork(work []WorkItem, state string) int {
	n := 0
	for _, w := range work {
		if w.State == state {
			n++
		}
	}
	return n
}

// Build 把输入收敛成整张图。
func Build(in Input) Board {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	waitingByKey := map[string][]InboxItem{}
	highRisk := 0
	earliestDue := ""
	for _, item := range in.Inbox {
		if item.AgentKey != "" {
			waitingByKey[item.AgentKey] = append(waitingByKey[item.AgentKey], item)
		}
		if item.RiskLevel == "high" {
			highRisk++
		}
		earliestDue = earlier(earliestDue, item.DueAt)
	}
	workByID := map[string][]WorkItem{}
	for _, w := range in.Work {
		if w.AssigneeKind != "agent" || w.AssigneeID == "" {
			continue
		}
		workByID[w.AssigneeID] = append(workByID[w.AssigneeID], w)
	}
	nextRunByID := map[string]string{}
	for _, job := range in.Cron {
		t := ParseTime(job.NextRunAt)
		if job.AgentID == "" || t.IsZero() || t.Before(now) {
			continue
		}
		nextRunByID[job.AgentID] = earlier(nextRunByID[job.AgentID], job.NextRunAt)
	}
	usageByID := map[string]Usage{}
	for _, u := range in.Usage {
		usageByID[u.AgentID] = u
	}

	// 岗位 id → 岗位名；部门负责人可能直接挂部门节点（名为空）。
	positionName := map[string]string{}
	var index func(n OrgNode)
	index = func(n OrgNode) {
		if n.Level == "position" && n.ID != "" {
			positionName[n.ID] = n.Name
		}
		for _, c := range n.Children {
			index(c)
		}
	}
	for _, n := range in.Orgs {
		index(n)
	}

	agentByID := map[string]*BoardAgent{}
	var agentOrder []string
	byPosition := map[string][]*BoardAgent{}
	for _, a := range in.Agents {
		if a.ID == "" || a.DeletedAt != "" {
			continue
		}
		waiting := append([]InboxItem{}, waitingByKey[a.Key]...)
		work := append([]WorkItem{}, workByID[a.ID]...)
		use := usageByID[a.ID]
		failed := failedRe.MatchString(a.LastRunStatus)
		loop := Loop{
			Waiting: len(waiting),
			Running: countWork(work, WorkRunning),
			Stuck:   countWork(work, WorkStuck),
			Review:  countWork(work, WorkReview),
		}
		ranToday := SameDay(a.LastRunAt, now) || use.Calls > 0
		pos := strings.TrimSpace(a.PositionID)
		name := a.Name
		if name == "" {
			name = a.Key
		}
		active, fail := 0, 0
		if ranToday {
			active = 1
		}
		if failed {
			fail = 1
		}
		agent := &BoardAgent{
			ID:           a.ID,
			Key:          a.Key,
			Name:         name,
			Position:     positionName[pos],
			Failed:       failed,
			RanToday:     ranToday,
			LastRunAt:    a.LastRunAt,
			NextRunAt:    nextRunByID[a.ID],
			Calls:        use.Calls,
			CostMicro:    use.CostMicro,
			State:        stateOf(1, active, fail, loop),
			Loop:         loop,
			WaitingItems: waiting,
			WorkItems:    work,
		}
		if _, dup := agentByID[a.ID]; !dup {
			agentOrder = append(agentOrder, a.ID)
		}
		agentByID[a.ID] = agent
		if pos != "" {
			byPosition[pos] = append(byPosition[pos], agent)
		}
	}

	placed := map[string]bool{}
	var companyRoots []OrgNode
	for _, n := range in.Orgs {
		if alive(n) && n.Level == "company" {
			companyRoots = append(companyRoots, n)
		}
	}
	sortNodes(companyRoots)

	companies := make([]CompanyNode, 0, len(companyRoots))
	var inCompany []*BoardAgent
	for _, root := range companyRoots {
		depts := []DeptNode{}
		var deptMembers []*BoardAgent
		for _, dept := range collect(root, "department") {
			positions := collect(dept, "position")
			var onPositions []*BoardAgent
			empty := 0
			for _, p := range positions {
				list := byPosition[p.ID]
				if len(list) == 0 {
					empty++
				}
				onPositions = append(onPositions, list...)
			}
			// 有的 Agent（如自动建的部门负责人）直接挂在部门节点上，不经过岗位。
			onDept := byPosition[dept.ID]
			leadID := strings.TrimSpace(dept.DeptLeadAgentID)
			var lead *BoardAgent
			if leadID != "" {
				lead = agentByID[leadID]
			}
			members := uniq(append(append([]*BoardAgent{lead}, onDept...), onPositions...)...)
			order := map[string]int{}
			for i, a := range members {
				placed[a.ID] = true
				order[a.ID] = i
			}
			sorted := append([]*BoardAgent{}, members...)
			sort.SliceStable(sorted, func(i, j int) bool {
				ri, rj := stateRank[sorted[i].State], stateRank[sorted[j].State]
				if ri != rj {
					return ri < rj
				}
				return order[sorted[i].ID] < order[sorted[j].ID]
			})
			agents := make([]BoardAgent, 0, len(sorted))
			for _, a := range sorted {
				agents = append(agents, *a)
			}
			deptMembers = append(deptMembers, sorted...)
			depts = append(depts, DeptNode{
				ID:             dept.ID,
				Name:           dept.Name,
				Lead:           copyAgent(lead),
				LeadMissing:    leadID != "" && lead == nil,
				Positions:      len(positions),
				EmptyPositions: empty,
				Agents:         agents,
				Rollup:         rollup(members),
			})
		}
		ceoID := CompanyLeadID(root.MetadataJSON)
		var ceo *BoardAgent
		if ceoID != "" {
			ceo = agentByID[ceoID]
		}
		if ceo != nil {
			placed[ceo.ID] = true
		}
		all := uniq(append([]*BoardAgent{ceo}, deptMembers...)...)
		inCompany = append(inCompany, all...)
		companies = append(companies, CompanyNode{
			ID:            root.ID,
			Name:          root.Name,
			CEO:           copyAgent(ceo),
			CEOConfigured: ceoID != "",
			Depts:         depts,
			Rollup:        rollup(all),
		})
	}

	inCompany = uniq(inCompany...)
	all := rollup(inCompany)
	attributed := map[string]bool{}
	for _, a := range inCompany {
		for _, item := range a.WaitingItems {
			attributed[item.ID] = true
		}
	}
	unattributed := 0
	for _, item := range in.Inbox {
		if !attributed[item.ID] {
			unattributed++
		}
	}
	unassigned := 0
	for _, id := range agentOrder {
		if !placed[id] {
			unassigned++
		}
	}
	waiting := in.InboxTotal
	if len(in.Inbox) > waiting {
		waiting = len(in.Inbox)
	}
	return Board{
		Companies: companies,
		Me: Me{
			Loop: Loop{
				Waiting: waiting,
				Running: countWork(in.Work, WorkRunning),
				Stuck:   countWork(in.Work, WorkStuck),
				Review:  countWork(in.Work, WorkReview),
			},
			HighRisk:     highRisk,
			EarliestDue:  earliestDue,
			WaitingItems: append([]InboxItem{}, in.Inbox...),
			WorkItems:    append([]WorkItem{}, in.Work...),
			Unattributed: unattributed,
			Unassigned:   unassigned,
			AgentCount:   all.AgentCount,
			ActiveToday:  all.ActiveToday,
			Calls:        all.Calls,
			CostMicro:    all.CostMicro,
		},
	}
}

func copyAgent(a *BoardAgent) *BoardAgent {
	if a == nil {
		return nil
	}
	c := *a
	return &c
}
