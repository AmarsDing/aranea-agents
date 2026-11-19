// Package bizview 聚合电商经营业务数据，供 CEO 经营驾驶舱使用。
//
// 设计约束：
//   - 只读：仅使用 Ent 查询，不写入任何业务表。
//   - 固定查询数：每张表一次分组/计数，避免逐行 N+1。
//   - 诚实空态：无数据的指标返回 pending/“—”，不编造 0 或估算值。
//   - 交易域（订单/支付/库存/履约/售后/流量/营销/客服/财务）当前系统不存在，
//     仅在 Coverage 中声明未接入，不输出数字。
package bizview

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/contentitem"
	"aranea-agents/internal/data/ent/decisionrecord"
	"aranea-agents/internal/data/ent/modeltokenusagehourly"
	"aranea-agents/internal/data/ent/opportunity"
	"aranea-agents/internal/data/ent/organization"
	"aranea-agents/internal/data/ent/publishjob"
	"aranea-agents/internal/data/ent/reviewtask"
	"aranea-agents/internal/data/ent/session"
	"aranea-agents/internal/data/ent/suppliercandidate"
	"aranea-agents/internal/data/ent/team"
	"aranea-agents/internal/data/ent/teamrun"
)

// 业务日按中国时区（UTC+8）切分，与经营习惯一致。
var bizLoc = time.FixedZone("Asia/Shanghai", 8*3600)

// Snapshot 是 CEO 经营驾驶舱的数据快照。
type Snapshot struct {
	AsOf      string     `json:"as_of"`
	Window    string     `json:"window"`
	ScopeNote string     `json:"scope_note"`
	Company   Company    `json:"company"`
	KPIs      []Metric   `json:"kpis"`
	Modules   []Module   `json:"modules"`
	Anomalies []Anomaly  `json:"anomalies"`
	Coverage  []Coverage `json:"coverage"`
	Notice    string     `json:"notice,omitempty"`
}

// Company 是公司级规模与成本事实。
type Company struct {
	Name              string `json:"name"`
	Health            string `json:"health"`
	Departments       int    `json:"departments"`
	Positions         int    `json:"positions"`
	Agents            int    `json:"agents"`
	Teams             int    `json:"teams"`
	ActiveSessions    int    `json:"active_sessions_24h"`
	RunningRuns       int    `json:"running_runs"`
	TodayCostMicroUSD int64  `json:"today_cost_micro_usd"`
	TodayTokens       int64  `json:"today_tokens"`
}

// Drill 描述指标/实体的下钻入口。
type Drill struct {
	Label    string `json:"label"`
	LabelKey string `json:"label_key,omitempty"`
	Path     string `json:"path"`
}

// Metric 是统一的经营指标视图。
type Metric struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	NameKey string  `json:"name_key,omitempty"`
	Value   float64 `json:"value"`
	Display string  `json:"display"`
	Unit    string  `json:"unit"`
	Window  string  `json:"window"`
	Health  string  `json:"health"`
	Source  string  `json:"source"`
	Rule    string  `json:"rule,omitempty"`
	Note    string  `json:"note,omitempty"`
	Pending bool    `json:"pending"`
	Drill   *Drill  `json:"drill,omitempty"`
}

// Entity 是公司运转图上的业务实体卡。
type Entity struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameKey string `json:"name_key,omitempty"`
	Kind    string `json:"kind"`
	Count   int    `json:"count"`
	Status  string `json:"status,omitempty"`
	Health  string `json:"health"`
	Note    string `json:"note,omitempty"`
	Drill   *Drill `json:"drill,omitempty"`
}

// Module 是公司运转图上的业务模块（岗位/职能）。
type Module struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	NameKey    string   `json:"name_key,omitempty"`
	Tone       string   `json:"tone"`
	Job        string   `json:"job"`
	JobKey     string   `json:"job_key,omitempty"`
	Scope      string   `json:"scope"`
	ScopeKey   string   `json:"scope_key,omitempty"`
	Cadence    string   `json:"cadence"`
	CadenceKey string   `json:"cadence_key,omitempty"`
	Health     string   `json:"health"`
	Working    bool     `json:"working"`
	Metrics    []Metric `json:"metrics"`
	Entities   []Entity `json:"entities"`
}

// Anomaly 是业务异常，供 CEO 决策区直接展示。
type Anomaly struct {
	ID     string         `json:"id"`
	Code   string         `json:"code"`
	Level  string         `json:"level"`
	Module string         `json:"module"`
	Text   string         `json:"text"`
	Params map[string]any `json:"params,omitempty"`
	Since  string         `json:"since,omitempty"`
	Drill  *Drill         `json:"drill,omitempty"`
}

// Coverage 声明尚未接入的经营域。
type Coverage struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameKey string `json:"name_key,omitempty"`
	Ready   bool   `json:"ready"`
	Note    string `json:"note"`
	NoteKey string `json:"note_key,omitempty"`
}

// Builder 负责把 OLTP 表聚合成经营快照，并做短 TTL 缓存。
type Builder struct {
	client *ent.Client
	now    func() time.Time
	ttl    time.Duration

	mu       sync.Mutex
	cached   *Snapshot
	cachedAt time.Time
}

// NewBuilder 创建聚合器。client 为空时 Build 返回错误。
func NewBuilder(client *ent.Client) *Builder {
	return &Builder{
		client: client,
		now:    func() time.Time { return time.Now().UTC() },
		ttl:    10 * time.Second,
	}
}

// Build 返回经营快照。10s 内重复请求直接复用缓存，避免多端轮询压库。
func (b *Builder) Build(ctx context.Context) (*Snapshot, error) {
	if b == nil || b.client == nil {
		return nil, fmt.Errorf("bizview: ent client is nil")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	if b.cached != nil && now.Sub(b.cachedAt) < b.ttl {
		return b.cached, nil
	}
	snap, err := b.build(ctx, now)
	if err != nil {
		return nil, err
	}
	b.cached = snap
	b.cachedAt = now
	return snap, nil
}

type statusCount struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

func (b *Builder) build(ctx context.Context, now time.Time) (*Snapshot, error) {
	client := b.client
	todayStart := bizDayStart(now)
	since24h := now.Add(-24 * time.Hour)
	since72h := now.Add(-72 * time.Hour)
	today := fmtTime(todayStart)
	cut24h := fmtTime(since24h)
	cut72h := fmtTime(since72h)
	packagingCut := fmtTime(now.Add(-time.Hour))

	// 内容池
	content := map[string]int{}
	var contentRows []statusCount
	if err := client.ContentItem.Query().
		GroupBy(contentitem.FieldStatus).
		Aggregate(ent.Count()).
		Scan(ctx, &contentRows); err != nil {
		return nil, fmt.Errorf("content status: %w", err)
	}
	for _, r := range contentRows {
		content[r.Status] = r.Count
	}
	contentTotal := 0
	for _, n := range content {
		contentTotal += n
	}
	contentToday, err := client.ContentItem.Query().Where(contentitem.CreatedAtGTE(today)).Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("content today: %w", err)
	}
	content24h, err := client.ContentItem.Query().Where(contentitem.CreatedAtGTE(cut24h)).Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("content 24h: %w", err)
	}
	sourceTypes, err := client.ContentItem.Query().
		Where(contentitem.SourceTypeNEQ("")).
		GroupBy(contentitem.FieldSourceType).
		Strings(ctx)
	if err != nil {
		return nil, fmt.Errorf("content sources: %w", err)
	}

	// 评审队列
	review := map[string]int{}
	var reviewRows []statusCount
	if err := client.ReviewTask.Query().
		GroupBy(reviewtask.FieldStatus).
		Aggregate(ent.Count()).
		Scan(ctx, &reviewRows); err != nil {
		return nil, fmt.Errorf("review status: %w", err)
	}
	for _, r := range reviewRows {
		review[r.Status] = r.Count
	}
	reviewApprovedToday, err := client.ReviewTask.Query().
		Where(reviewtask.StatusEQ("approved"), reviewtask.DecidedAtGTE(today)).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("review approved today: %w", err)
	}
	reviewRejectedToday, err := client.ReviewTask.Query().
		Where(reviewtask.StatusEQ("rejected"), reviewtask.DecidedAtGTE(today)).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("review rejected today: %w", err)
	}
	oldestPendingAt := ""
	var oldestHours int
	if review["pending"] > 0 {
		oldest, err := client.ReviewTask.Query().
			Where(reviewtask.StatusEQ("pending")).
			Order(ent.Asc(reviewtask.FieldCreatedAt)).
			First(ctx)
		if err == nil && oldest != nil {
			oldestPendingAt = oldest.CreatedAt
			if t, perr := time.Parse(time.RFC3339, oldest.CreatedAt); perr == nil {
				oldestHours = int(now.Sub(t).Hours())
				if oldestHours < 0 {
					oldestHours = 0
				}
			}
		} else if err != nil && !ent.IsNotFound(err) {
			return nil, fmt.Errorf("review oldest: %w", err)
		}
	}

	// 发布队列
	publish := map[string]int{}
	var publishRows []statusCount
	if err := client.PublishJob.Query().
		GroupBy(publishjob.FieldStatus).
		Aggregate(ent.Count()).
		Scan(ctx, &publishRows); err != nil {
		return nil, fmt.Errorf("publish status: %w", err)
	}
	for _, r := range publishRows {
		publish[r.Status] = r.Count
	}
	publishDoneToday, err := client.PublishJob.Query().
		Where(publishjob.StatusEQ("packaged"), publishjob.UpdatedAtGTE(today)).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("publish done today: %w", err)
	}
	publishFailed24h, err := client.PublishJob.Query().
		Where(publishjob.StatusIn("failed", "dead_letter"), publishjob.UpdatedAtGTE(cut24h)).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("publish failed 24h: %w", err)
	}
	publishStuck, err := client.PublishJob.Query().
		Where(
			publishjob.StatusEQ("packaging"),
			publishjob.UpdatedAtNEQ(""),
			publishjob.UpdatedAtLT(packagingCut),
		).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("publish stuck: %w", err)
	}

	// 商机与供应商
	opps := map[string]int{}
	var oppRows []statusCount
	if err := client.Opportunity.Query().
		GroupBy(opportunity.FieldStatus).
		Aggregate(ent.Count()).
		Scan(ctx, &oppRows); err != nil {
		return nil, fmt.Errorf("opportunity status: %w", err)
	}
	for _, r := range oppRows {
		opps[r.Status] = r.Count
	}
	oppStale, err := client.Opportunity.Query().
		Where(opportunity.StatusEQ("open"), opportunity.UpdatedAtLT(cut72h)).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("opportunity stale: %w", err)
	}
	suppliers := map[string]int{}
	var supplierRows []statusCount
	if err := client.SupplierCandidate.Query().
		GroupBy(suppliercandidate.FieldStatus).
		Aggregate(ent.Count()).
		Scan(ctx, &supplierRows); err != nil {
		return nil, fmt.Errorf("supplier status: %w", err)
	}
	for _, r := range supplierRows {
		suppliers[r.Status] = r.Count
	}
	openAck := opps["open"] + opps["acknowledged"]
	covered, err := b.supplierCoverage(ctx)
	if err != nil {
		return nil, err
	}
	uncovered := openAck - covered
	if uncovered < 0 {
		uncovered = 0
	}
	coveragePct := -1.0
	if openAck > 0 {
		coveragePct = float64(covered) * 100 / float64(openAck)
	}

	// 组织、团队、会话
	deptCount, err := client.Organization.Query().
		Where(organization.LevelEQ("department"), organization.DeletedAtEQ("")).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("org departments: %w", err)
	}
	posCount, err := client.Organization.Query().
		Where(organization.LevelEQ("position"), organization.DeletedAtEQ("")).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("org positions: %w", err)
	}
	agentCount, err := client.Agent.Query().Where(agent.StatusEQ("active")).Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("agents: %w", err)
	}
	teamCount, err := client.Team.Query().Where(team.DeletedAtEQ("")).Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("teams: %w", err)
	}
	runningRuns, err := client.TeamRun.Query().Where(teamrun.StatusEQ("running")).Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("team runs running: %w", err)
	}
	runFailed24h, err := client.TeamRun.Query().
		Where(teamrun.StatusEQ("failed"), teamrun.CreatedAtGTE(cut24h)).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("team runs failed: %w", err)
	}
	activeSessions, err := client.Session.Query().
		Where(session.LastMessageAtGTE(cut24h), session.AgentIDNEQ("")).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("active sessions: %w", err)
	}
	decision24h, err := client.DecisionRecord.Query().
		Where(decisionrecord.CreatedAtGTE(cut24h)).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("decisions: %w", err)
	}

	// 成本与用量
	todayCost, todayTokens, err := b.usageSum(ctx, fmtHour(todayStart), "")
	if err != nil {
		return nil, err
	}
	weekCost, _, err := b.usageSum(ctx, fmtHour(todayStart.Add(-7*24*time.Hour)), fmtHour(todayStart))
	if err != nil {
		return nil, err
	}
	weekAvgCost := weekCost / 7
	costHealth := "green"
	if todayCost > 0 && weekAvgCost > 0 {
		if todayCost > weekAvgCost*3 {
			costHealth = "red"
		} else if todayCost > weekAvgCost*2 {
			costHealth = "yellow"
		}
	}

	// 模块与指标
	insightMetrics := []Metric{
		metricInt("content_inflow", "今日新增内容", "contentInflow", contentToday, "条", "today", "green",
			"content_items.created_at", &Drill{Label: "内容收件箱", LabelKey: "contentInbox", Path: "/content-inbox"}),
		metricInt("content_candidate", "候选内容", "candidateContent", content["candidate"], "条", "realtime", "green",
			"content_items.status", &Drill{Label: "内容收件箱", LabelKey: "contentInbox", Path: "/content-inbox"}),
	}
	produceMetrics := []Metric{
		metricInt("content_drafting", "生产中", "draftingContent", content["drafting"], "条", "realtime", "green",
			"content_items.status", &Drill{Label: "内容收件箱", LabelKey: "contentInbox", Path: "/content-inbox"}),
		metricInt("content_in_review", "待评审", "inReviewContent", content["in_review"], "条", "realtime", "green",
			"content_items.status", &Drill{Label: "内容收件箱", LabelKey: "contentInbox", Path: "/content-inbox"}),
	}
	reviewHealth := healthByCount(review["pending"], 10, 30)
	// 最久待审超过 24/48 小时，同样拉低评审模块健康度（与异常口径一致）。
	if oldestHours > 48 {
		reviewHealth = worst(reviewHealth, "red")
	} else if oldestHours > 24 {
		reviewHealth = worst(reviewHealth, "yellow")
	}
	stuckHealth := "green"
	if publishStuck > 0 {
		stuckHealth = "yellow"
	}
	reviewMetrics := []Metric{
		metricInt("review_pending", "待审积压", "reviewPending", review["pending"], "条", "realtime", reviewHealth,
			"review_tasks.status", &Drill{Label: "评审队列", LabelKey: "reviewQueue", Path: "/review-queue"}),
		metricInt("review_approved_today", "今日通过", "reviewApprovedToday", reviewApprovedToday, "条", "today", "green",
			"review_tasks.decided_at", &Drill{Label: "评审队列", LabelKey: "reviewQueue", Path: "/review-queue"}),
	}
	publishHealth := "green"
	if publishFailed24h > 0 {
		publishHealth = "red"
	} else if publishStuck > 0 {
		publishHealth = "yellow"
	}
	publishMetrics := []Metric{
		metricInt("publish_waiting", "待打包", "publishWaiting", publish["pending"]+publish["packaging"], "条", "realtime", publishHealth,
			"publish_jobs.status", nil),
		metricInt("publish_failed_24h", "24h 失败", "publishFailed24h", publishFailed24h, "条", "24h", publishHealth,
			"publish_jobs.status", nil),
	}
	convValue := -1.0
	convDisplay := "—"
	convPending := true
	if denom := opps["converted"] + opps["dismissed"]; denom > 0 {
		convValue = float64(opps["converted"]) * 100 / float64(denom)
		convDisplay = fmt.Sprintf("%.0f%%", convValue)
		convPending = false
	}
	commerceMetrics := []Metric{
		metricInt("opportunity_open", "待跟进商机", "opportunityOpen", openAck, "条", "realtime", "green",
			"opportunities.status", &Drill{Label: "商机看板", LabelKey: "opportunityBoard", Path: "/opportunity-board"}),
		{ID: "conversion_rate", Name: "已决商机转化率", NameKey: "conversionRate", Value: convValue, Display: convDisplay,
			Unit: "%", Window: "realtime", Health: "green", Source: "opportunities.status", Pending: convPending,
			Rule: "converted / (converted + dismissed)", Note: "不含未决商机",
			Drill: &Drill{Label: "商机看板", LabelKey: "opportunityBoard", Path: "/opportunity-board"}},
		{ID: "supply_coverage", Name: "供应商覆盖", NameKey: "supplyCoverage", Value: coveragePct, Display: pctDisplay(coveragePct),
			Unit: "%", Window: "realtime", Health: coverageHealth(coveragePct), Source: "supplier_candidates.opportunity_id",
			Pending: coveragePct < 0, Rule: "至少 1 个候选商的待跟进商机 / 待跟进商机",
			Drill: &Drill{Label: "选品看板", LabelKey: "sourcing", Path: "/sourcing"}},
	}
	opsMetrics := []Metric{
		metricInt("active_sessions", "活跃会话", "activeSessions", activeSessions, "个", "24h", "green",
			"sessions.last_message_at", nil),
		{ID: "cost_today", Name: "今日 AI 成本", NameKey: "costToday", Value: float64(todayCost) / 1e6,
			Display: fmt.Sprintf("$%.2f", float64(todayCost)/1e6), Unit: "USD", Window: "today", Health: costHealth,
			Source: "model_token_usage_hourly", Rule: ">2× 前 7 日均值黄，>3× 红",
			Drill: &Drill{Label: "用量明细", LabelKey: "usage", Path: "/usage/events"}},
		metricInt("running_runs", "运行中团队", "runningRuns", runningRuns, "个", "realtime", "green",
			"team_runs.status", &Drill{Label: "团队", LabelKey: "teams", Path: "/teams"}),
	}

	modules := []Module{
		{
			ID: "insight", Name: "情报选题", NameKey: "insight", Tone: "accent",
			Job: "找素材、追热点，保证内容池不断流", JobKey: "insightJob",
			Scope: "内容池 / 来源 / 热度", ScopeKey: "insightScope",
			Cadence: "实时", CadenceKey: "realtime", Health: "green", Working: content24h > 0,
			Metrics: insightMetrics,
			Entities: []Entity{
				entity("content_candidate", "候选内容", "candidateContent", "content", content["candidate"], "candidate", "green", "/content-inbox", "contentInbox"),
				entity("content_selected", "已选内容", "selectedContent", "content", content["selected"], "selected", "green", "/content-inbox", "contentInbox"),
				entity("content_sources", "来源渠道", "contentSources", "channel", len(sourceTypes), "source", "green", "/content-inbox", "contentInbox"),
			},
		},
		{
			ID: "produce", Name: "内容生产", NameKey: "produce", Tone: "ai",
			Job: "把选题变成可评审的稿件", JobKey: "produceJob",
			Scope: "草稿 / 待评审", ScopeKey: "produceScope",
			Cadence: "按任务", CadenceKey: "onTask", Health: "green",
			Working: content["drafting"]+content["in_review"] > 0,
			Metrics: produceMetrics,
			Entities: []Entity{
				entity("content_drafting", "草稿中", "draftingContent", "content", content["drafting"], "drafting", "green", "/content-inbox", "contentInbox"),
				entity("content_in_review", "待评审", "inReviewContent", "content", content["in_review"], "in_review", healthByCount(content["in_review"], 10, 30), "/content-inbox", "contentInbox"),
				entity("content_approved", "已通过", "approvedContent", "content", content["approved"], "approved", "green", "/content-inbox", "contentInbox"),
			},
		},
		{
			ID: "review", Name: "评审风控", NameKey: "review", Tone: "warn",
			Job: "人工把关，决定内容发不发", JobKey: "reviewJob",
			Scope: "待审队列 / 等待时长", ScopeKey: "reviewScope",
			Cadence: "实时", CadenceKey: "realtime", Health: reviewHealth,
			Working: review["pending"] > 0,
			Metrics: reviewMetrics,
			Entities: []Entity{
				entity("review_pending", "待审任务", "reviewPending", "review", review["pending"], "pending", reviewHealth, "/review-queue", "reviewQueue"),
				entity("review_approved_today", "今日通过", "reviewApprovedToday", "review", reviewApprovedToday, "approved", "green", "/review-queue", "reviewQueue"),
				entity("review_rejected_today", "今日驳回", "reviewRejectedToday", "review", reviewRejectedToday, "rejected", "green", "/review-queue", "reviewQueue"),
				entity("review_cancelled", "已取消", "reviewCancelled", "review", review["cancelled"], "cancelled", "green", "/review-queue", "reviewQueue"),
			},
		},
		{
			ID: "publish", Name: "发布分发", NameKey: "publish", Tone: "success",
			Job: "打包交付并跟踪失败", JobKey: "publishJobDesc",
			Scope: "打包队列 / 失败", ScopeKey: "publishScope",
			Cadence: "按任务", CadenceKey: "onTask", Health: publishHealth,
			Working: publish["pending"]+publish["packaging"] > 0,
			Metrics: publishMetrics,
			Entities: []Entity{
				entity("publish_pending", "待打包", "publishPending", "publish", publish["pending"], "pending", "green", "/review-queue", "reviewQueue"),
				entity("publish_packaging", "打包中", "publishPackaging", "publish", publish["packaging"], "packaging", stuckHealth, "/review-queue", "reviewQueue"),
				entity("publish_done_today", "今日已打包", "publishDoneToday", "publish", publishDoneToday, "packaged", "green", "/review-queue", "reviewQueue"),
				entity("publish_failed", "失败/死信", "publishFailed", "publish", publishFailed24h, "24h", publishHealth, "/review-queue", "reviewQueue"),
			},
		},
		{
			ID: "commerce", Name: "商机选品", NameKey: "commerce", Tone: "info",
			Job: "把内容流量转成商机与供给", JobKey: "commerceJob",
			Scope: "商机 / 供应商", ScopeKey: "commerceScope",
			Cadence: "每日", CadenceKey: "daily", Health: coverageHealth(coveragePct),
			Working: openAck > 0,
			Metrics: commerceMetrics,
			Entities: []Entity{
				entity("opportunity_open", "待跟进商机", "opportunityOpen", "opportunity", openAck, "open", "green", "/opportunity-board", "opportunityBoard"),
				entity("opportunity_converted", "已转化商机", "opportunityConverted", "opportunity", opps["converted"], "converted", "green", "/opportunity-board", "opportunityBoard"),
				entity("supplier_pending", "候选供应商", "supplierPending", "supplier", suppliers["suggested"]+suppliers["contacting"]+suppliers["sampled"], "candidate", "green", "/sourcing", "sourcing"),
				entity("supplier_approved", "已准入供应商", "supplierApproved", "supplier", suppliers["approved_for_purchase"], "approved_for_purchase", "green", "/sourcing", "sourcing"),
			},
		},
		{
			ID: "ops", Name: "组织协同", NameKey: "ops", Tone: "muted",
			Job: "组织 Agent 干活，控制成本", JobKey: "opsJob",
			Scope: "组织 / 团队 / 会话 / 成本", ScopeKey: "opsScope",
			Cadence: "实时", CadenceKey: "realtime", Health: costHealth,
			Working: activeSessions > 0 || runningRuns > 0,
			Metrics: opsMetrics,
			Entities: []Entity{
				entity("org_departments", "部门", "departments", "org", deptCount, "org", "green", "/organization", "organization"),
				entity("org_positions", "岗位", "positions", "org", posCount, "org", "green", "/organization", "organization"),
				entity("agents", "Agent", "agents", "agent", agentCount, "active", "green", "/agents", "agents"),
				entity("teams", "团队", "teams", "team", teamCount, "team", "green", "/teams", "teams"),
				entity("decisions_24h", "AI 决策", "decisions24h", "decision", decision24h, "24h", "green", "/monitor", "monitor"),
			},
		},
	}

	kpis := []Metric{
		insightMetrics[0],
		reviewMetrics[0],
		publishMetrics[1],
		commerceMetrics[0],
		commerceMetrics[1],
		commerceMetrics[2],
		opsMetrics[1],
		opsMetrics[0],
	}

	anomalies := buildAnomalies(buildAnomalyInput{
		reviewPending:    review["pending"],
		oldestHours:      oldestHours,
		oldestPendingAt:  oldestPendingAt,
		publishFailed24h: publishFailed24h,
		publishStuck:     publishStuck,
		oppsOpen:         opps["open"],
		oppStale:         oppStale,
		openAck:          openAck,
		uncovered:        uncovered,
		coveragePct:      coveragePct,
		costToday:        todayCost,
		weekAvgCost:      weekAvgCost,
		contentTotal:     contentTotal,
		content24h:       content24h,
		runFailed24h:     runFailed24h,
	})

	// 模块状态点与异常口径对齐：同一模块最严重的异常决定其健康度。
	for _, a := range anomalies {
		for i := range modules {
			if modules[i].ID == a.Module {
				modules[i].Health = worst(modules[i].Health, a.Level)
			}
		}
	}
	health := "green"
	for _, a := range anomalies {
		health = worst(health, a.Level)
	}
	if contentTotal == 0 && len(opps) == 0 && len(review) == 0 && len(publish) == 0 && len(suppliers) == 0 {
		health = "gray"
	}

	snap := &Snapshot{
		AsOf:      fmtTime(now),
		Window:    "24h",
		ScopeNote: "电商内容闭环 + 组织/成本；交易域未接入",
		Company: Company{
			Name:              "Aranea 电商",
			Health:            health,
			Departments:       deptCount,
			Positions:         posCount,
			Agents:            agentCount,
			Teams:             teamCount,
			ActiveSessions:    activeSessions,
			RunningRuns:       runningRuns,
			TodayCostMicroUSD: todayCost,
			TodayTokens:       todayTokens,
		},
		KPIs:      kpis,
		Modules:   modules,
		Anomalies: anomalies,
		Coverage:  defaultCoverage(),
		Notice:    "数据来自 Aranea OLTP 业务表，10 秒缓存；packaged 表示已打包，不等于外部平台已发布。",
	}
	return snap, nil
}

// supplierCoverage 返回“待跟进商机中至少有一个候选供应商”的数量。
func (b *Builder) supplierCoverage(ctx context.Context) (int, error) {
	oppIDs, err := b.client.Opportunity.Query().
		Where(opportunity.StatusIn("open", "acknowledged")).
		Select(opportunity.FieldID).
		Strings(ctx)
	if err != nil {
		return 0, fmt.Errorf("coverage opportunities: %w", err)
	}
	if len(oppIDs) == 0 {
		return 0, nil
	}
	coveredIDs, err := b.client.SupplierCandidate.Query().
		Where(suppliercandidate.OpportunityIDIn(oppIDs...)).
		GroupBy(suppliercandidate.FieldOpportunityID).
		Strings(ctx)
	if err != nil {
		return 0, fmt.Errorf("coverage suppliers: %w", err)
	}
	open := make(map[string]struct{}, len(oppIDs))
	for _, id := range oppIDs {
		open[id] = struct{}{}
	}
	n := 0
	for _, id := range coveredIDs {
		if _, ok := open[id]; ok {
			n++
		}
	}
	return n, nil
}

// usageSum 汇总 [from, to) 的 token 与成本；to 为空表示到当前。
func (b *Builder) usageSum(ctx context.Context, from, to string) (int64, int64, error) {
	q := b.client.ModelTokenUsageHourly.Query().Where(modeltokenusagehourly.HourKeyGTE(from))
	if to != "" {
		q = q.Where(modeltokenusagehourly.HourKeyLT(to))
	}
	var rows []struct {
		Cost   int64 `json:"cost"`
		Tokens int64 `json:"tokens"`
	}
	if err := q.Aggregate(
		ent.As(ent.Sum(modeltokenusagehourly.FieldTotalCostMicroUsd), "cost"),
		ent.As(ent.Sum(modeltokenusagehourly.FieldTotalTokens), "tokens"),
	).Scan(ctx, &rows); err != nil {
		return 0, 0, fmt.Errorf("usage sum: %w", err)
	}
	if len(rows) == 0 {
		return 0, 0, nil
	}
	return rows[0].Cost, rows[0].Tokens, nil
}

type buildAnomalyInput struct {
	reviewPending    int
	oldestHours      int
	oldestPendingAt  string
	publishFailed24h int
	publishStuck     int
	oppsOpen         int
	oppStale         int
	openAck          int
	uncovered        int
	coveragePct      float64
	costToday        int64
	weekAvgCost      int64
	contentTotal     int
	content24h       int
	runFailed24h     int
}

func buildAnomalies(in buildAnomalyInput) []Anomaly {
	out := make([]Anomaly, 0, 6)
	if in.reviewPending > 10 {
		level := "yellow"
		if in.reviewPending > 30 {
			level = "red"
		}
		out = append(out, Anomaly{
			ID: "review_backlog", Code: "reviewBacklog", Level: level, Module: "review",
			Text:   fmt.Sprintf("待审积压 %d 条，超过阈值", in.reviewPending),
			Params: map[string]any{"count": in.reviewPending},
			Drill:  &Drill{Label: "评审队列", LabelKey: "reviewQueue", Path: "/review-queue"},
		})
	}
	if in.oldestHours > 24 {
		level := "yellow"
		if in.oldestHours > 48 {
			level = "red"
		}
		out = append(out, Anomaly{
			ID: "review_stale", Code: "reviewStale", Level: level, Module: "review",
			Text:   fmt.Sprintf("最久待审已等待 %d 小时", in.oldestHours),
			Params: map[string]any{"hours": in.oldestHours},
			Since:  in.oldestPendingAt,
			Drill:  &Drill{Label: "评审队列", LabelKey: "reviewQueue", Path: "/review-queue"},
		})
	}
	if in.publishFailed24h > 0 {
		out = append(out, Anomaly{
			ID: "publish_failed", Code: "publishFailed", Level: "red", Module: "publish",
			Text:   fmt.Sprintf("24 小时内有 %d 个发布任务失败/进入死信", in.publishFailed24h),
			Params: map[string]any{"count": in.publishFailed24h},
			Drill:  &Drill{Label: "评审队列", LabelKey: "reviewQueue", Path: "/review-queue"},
		})
	}
	if in.publishStuck > 0 {
		out = append(out, Anomaly{
			ID: "publish_stuck", Code: "publishStuck", Level: "yellow", Module: "publish",
			Text:   fmt.Sprintf("%d 个发布任务打包超过 1 小时未完成", in.publishStuck),
			Params: map[string]any{"count": in.publishStuck},
			Drill:  &Drill{Label: "评审队列", LabelKey: "reviewQueue", Path: "/review-queue"},
		})
	}
	if in.oppStale > 0 {
		out = append(out, Anomaly{
			ID: "opportunity_stale", Code: "opportunityStale", Level: "yellow", Module: "commerce",
			Text:   fmt.Sprintf("%d 个商机超过 72 小时未跟进", in.oppStale),
			Params: map[string]any{"count": in.oppStale},
			Drill:  &Drill{Label: "商机看板", LabelKey: "opportunityBoard", Path: "/opportunity-board"},
		})
	}
	if in.openAck > 0 && in.coveragePct >= 0 && in.coveragePct < 30 {
		out = append(out, Anomaly{
			ID: "supply_gap", Code: "supplyGap", Level: "yellow", Module: "commerce",
			Text:   fmt.Sprintf("%d 个待跟进商机没有候选供应商（覆盖率 %.0f%%）", in.uncovered, in.coveragePct),
			Params: map[string]any{"count": in.uncovered, "coverage": fmt.Sprintf("%.0f", in.coveragePct)},
			Drill:  &Drill{Label: "选品看板", LabelKey: "sourcing", Path: "/sourcing"},
		})
	}
	if in.costToday > 0 && in.weekAvgCost > 0 && in.costToday > in.weekAvgCost*2 {
		level := "yellow"
		if in.costToday > in.weekAvgCost*3 {
			level = "red"
		}
		out = append(out, Anomaly{
			ID: "cost_spike", Code: "costSpike", Level: level, Module: "ops",
			Text: fmt.Sprintf("今日 AI 成本 $%.2f，超过前 7 日均值 $%.2f 的 2 倍",
				float64(in.costToday)/1e6, float64(in.weekAvgCost)/1e6),
			Params: map[string]any{
				"today_usd":   fmt.Sprintf("%.2f", float64(in.costToday)/1e6),
				"avg_usd":     fmt.Sprintf("%.2f", float64(in.weekAvgCost)/1e6),
				"today_micro": in.costToday,
				"avg_micro":   in.weekAvgCost,
			},
			Drill: &Drill{Label: "用量明细", LabelKey: "usage", Path: "/usage/events"},
		})
	}
	if in.contentTotal > 0 && in.content24h == 0 {
		out = append(out, Anomaly{
			ID: "content_stall", Code: "contentStall", Level: "yellow", Module: "insight",
			Text:  "内容池有存量，但过去 24 小时没有新增内容",
			Drill: &Drill{Label: "内容收件箱", LabelKey: "contentInbox", Path: "/content-inbox"},
		})
	}
	if in.runFailed24h > 0 {
		out = append(out, Anomaly{
			ID: "team_run_failed", Code: "teamRunFailed", Level: "yellow", Module: "ops",
			Text:   fmt.Sprintf("24 小时内有 %d 次团队运行失败", in.runFailed24h),
			Params: map[string]any{"count": in.runFailed24h},
			Drill:  &Drill{Label: "团队", LabelKey: "teams", Path: "/teams"},
		})
	}
	return out
}

func defaultCoverage() []Coverage {
	return []Coverage{
		{ID: "orders", Name: "订单", NameKey: "orders", Note: "当前系统无订单表；需外部平台订单回写", NoteKey: "ordersNote"},
		{ID: "payments", Name: "支付", NameKey: "payments", Note: "无支付流水表；需对接平台结算", NoteKey: "paymentsNote"},
		{ID: "inventory", Name: "库存", NameKey: "inventory", Note: "无库存表；需 WMS/平台库存同步", NoteKey: "inventoryNote"},
		{ID: "fulfillment", Name: "履约", NameKey: "fulfillment", Note: "无履约单表；需物流轨迹接入", NoteKey: "fulfillmentNote"},
		{ID: "aftersales", Name: "售后", NameKey: "aftersales", Note: "无售后/退款表；需平台售后接口", NoteKey: "aftersalesNote"},
		{ID: "traffic", Name: "流量", NameKey: "traffic", Note: "无访问/转化埋点表；需埋点或平台数据", NoteKey: "trafficNote"},
		{ID: "marketing", Name: "营销", NameKey: "marketing", Note: "无投放/ROI 表；需广告平台接入", NoteKey: "marketingNote"},
		{ID: "customer_service", Name: "客服", NameKey: "customerService", Note: "无客服会话表；邮件/IM 未纳入经营口径", NoteKey: "customerServiceNote"},
		{ID: "finance", Name: "财务 / GMV", NameKey: "finance", Note: "无收入/成本核算表；GMV 无法计算", NoteKey: "financeNote"},
	}
}

func metricInt(id, name, key string, v int, unit, window, health, source string, drill *Drill) Metric {
	return Metric{
		ID: id, Name: name, NameKey: key,
		Value: float64(v), Display: strconv.Itoa(v), Unit: unit, Window: window,
		Health: health, Source: source, Pending: false, Drill: drill,
	}
}

func entity(id, name, key, kind string, count int, status, health, path, labelKey string) Entity {
	return Entity{
		ID: id, Name: name, NameKey: key, Kind: kind, Count: count, Status: status, Health: health,
		Drill: &Drill{Label: name, LabelKey: labelKey, Path: path},
	}
}

func healthByCount(v, yellowAbove, redAbove int) string {
	if redAbove > 0 && v > redAbove {
		return "red"
	}
	if yellowAbove > 0 && v > yellowAbove {
		return "yellow"
	}
	return "green"
}

func coverageHealth(pct float64) string {
	if pct < 0 {
		return "gray"
	}
	if pct < 30 {
		return "yellow"
	}
	return "green"
}

func pctDisplay(pct float64) string {
	if pct < 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", pct)
}

func worst(a, b string) string {
	if levelRank(b) > levelRank(a) {
		return b
	}
	return a
}

func levelRank(s string) int {
	switch s {
	case "red":
		return 3
	case "yellow":
		return 2
	case "green":
		return 1
	default:
		return 0
	}
}

func bizDayStart(now time.Time) time.Time {
	n := now.In(bizLoc)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, bizLoc).UTC()
}

func fmtTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func fmtHour(t time.Time) string {
	return t.UTC().Format("2006-01-02T15")
}
