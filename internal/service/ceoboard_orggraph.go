// 看板总览「一张图管全部公司」的后端聚合（设计稿 03-看板 §14.2）。
//
// GET /v1/ceoboard/org-graph：一次返回 我 → 公司（含总经理 Agent）→ 部门 → Agent，
// 口径与前端 web/src/features/company/tree.ts 一致（纯函数在 biz/ceoboard/orggraph）。
// GET /v1/ceoboard/company：电商经营快照（bizview，只读主库）。
//
// 两条都是只读 JSON BFF，挂在常驻的 BoardService 上，不跟驾驶舱运行时（ceoboard 配置段）
// 开关——驾驶舱关着时经营快照原来会 404（§14.1）。
//
// 装载照 inbox_sources.go 的做法：进程内复用各领域既有的 service 读方法，租户隔离照旧；
// 对应 RPC 登记了权限码的来源（定时任务 system.cron、用量 usage.read）在这里同码过闸。
// 组织树或名册读不到返回 503；其余来源失败只记进 partial，不拦图。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	agentv1 "aranea-agents/api/kratos/agent/v1"
	officev1 "aranea-agents/api/kratos/office/v1"
	organizationv1 "aranea-agents/api/kratos/organization/v1"
	sessionv1 "aranea-agents/api/kratos/session/v1"
	usagev1 "aranea-agents/api/kratos/usage/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/ceoboard/bizview"
	"aranea-agents/internal/biz/ceoboard/orggraph"
	"aranea-agents/internal/biz/inbox"
	"aranea-agents/pkg/auth"
	loggateway "aranea-agents/pkg/loggateway"

	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	orgGraphTimeout        = 8 * time.Second
	companySnapshotTimeout = 6 * time.Second
	orgGraphCacheTTL       = 10 * time.Second
	orgGraphAgentPg        = 500
	orgGraphFetch          = 100
	// shortcut: 今日用量只取花费前 200 个 Agent（接口上限）；今天跑过的超过 200 个再分页。
	orgGraphUsageLimit = 200

	// 总经理巡视 / 汇报的定时任务 task_key 前缀（后接公司 id）。
	gmPatrolKeyPrefix = gmPatrolTarget + ":"
	gmReportKeyPrefix = gmReportTarget + ":"
)

// 与前端 tree.ts 的 PartialSource 同名。
const (
	partialInbox     = "inbox"
	partialDelegated = "delegated"
	partialCron      = "cron"
	partialUsage     = "usage"
)

// delegatedStuck 与 web/src/features/today/logic.ts 的 SITUATION_STUCK 同一口径。
var delegatedStuck = map[string]bool{"waiting": true, "failed": true, "cancelled": true}

// BoardDeps 是看板聚合依赖的领域服务；Org、Agents 必填，其余为 nil 时对应来源记为读不到。
type BoardDeps struct {
	Org      *OrganizationService
	Agents   *AgentService
	Inbox    *inbox.Aggregator
	Office   *OfficeMailService
	Sessions *SessionV2Service
	Cron     *biz.CronUsecase
	Usage    *UsageService
	Biz      *bizview.Builder
}

// BoardService 承载看板总览的只读聚合接口。
type BoardService struct {
	deps BoardDeps
	lg   loggateway.Logger
	now  func() time.Time

	mu    sync.Mutex
	cache map[string]orgGraphCached
}

type orgGraphCached struct {
	at   time.Time
	body []byte
}

// OrgGraph 是 org-graph 接口的返回体：Board 字段平铺（与 tree.ts 的 Board 同名）。
type OrgGraph struct {
	GeneratedAt string   `json:"generatedAt"`
	Partial     []string `json:"partial"`
	orggraph.Board
}

// NewBoardService 装配看板服务。
func NewBoardService(deps BoardDeps, lg loggateway.Logger) *BoardService {
	return &BoardService{deps: deps, lg: lg, now: time.Now, cache: map[string]orgGraphCached{}}
}

// boardViewAllowed：有主体必须持 ceoboard.view；无主体仅 dev/test 放行（同 GetEventByUIDHTTP）。
func boardViewAllowed(r *http.Request) bool {
	if a, ok := auth.FromContext(r.Context()); ok && a != nil {
		return a.HasPermission("ceoboard.view")
	}
	return auth.DevOrTestEnv()
}

// OrgGraphHTTP 是 GET /v1/ceoboard/org-graph 的处理函数。
func (s *BoardService) OrgGraphHTTP(w http.ResponseWriter, r *http.Request) {
	if !boardViewAllowed(r) {
		writeCeoboardHTTPError(w, http.StatusForbidden, "ceoboard.view required")
		return
	}
	key := orgGraphCacheKey(r.Context())
	s.mu.Lock()
	hit, ok := s.cache[key]
	s.mu.Unlock()
	if !ok || s.now().Sub(hit.at) > orgGraphCacheTTL {
		ctx, cancel := context.WithTimeout(r.Context(), orgGraphTimeout)
		defer cancel()
		graph, err := s.Load(ctx)
		if err != nil {
			s.lg.Error("ceoboard org graph failed", loggateway.Err(err), loggateway.Domain(ceoboardDomain))
			writeCeoboardHTTPError(w, http.StatusServiceUnavailable, "organization data temporarily unavailable")
			return
		}
		body, err := json.Marshal(graph)
		if err != nil {
			writeCeoboardHTTPError(w, http.StatusInternalServerError, "encode failed")
			return
		}
		hit = orgGraphCached{at: s.now(), body: body}
		s.mu.Lock()
		s.cache[key] = hit
		s.mu.Unlock()
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(hit.body)
}

// orgGraphCacheKey 按调用方隔离缓存：收件箱等来源按权限过滤，不同人看到的图不同。
func orgGraphCacheKey(ctx context.Context) string {
	if a, ok := auth.FromContext(ctx); ok && a != nil {
		return strconv.FormatInt(a.UserID, 10) + "|" + a.EffectiveWorkspaceID()
	}
	return "-"
}

// CompanyViewHTTP 是 GET /v1/ceoboard/company 的处理函数（电商经营快照）。
func (s *BoardService) CompanyViewHTTP(w http.ResponseWriter, r *http.Request) {
	if !boardViewAllowed(r) {
		writeCeoboardHTTPError(w, http.StatusForbidden, "ceoboard.view required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), companySnapshotTimeout)
	defer cancel()
	snap, err := s.deps.Biz.Build(ctx)
	if err != nil {
		s.lg.Error("ceoboard company snapshot failed", loggateway.Err(err), loggateway.Domain(ceoboardDomain))
		writeCeoboardHTTPError(w, http.StatusServiceUnavailable, "business data temporarily unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(snap)
}

// Load 按调用方身份读全部来源并拼成整张图（不走缓存；总经理巡视也用它）。
func (s *BoardService) Load(ctx context.Context) (OrgGraph, error) {
	now := s.now()
	tree, err := s.deps.Org.ListOrganizationTree(ctx, &emptypb.Empty{})
	if err != nil {
		return OrgGraph{}, err
	}
	agents, err := s.loadAgents(ctx)
	if err != nil {
		return OrgGraph{}, err
	}
	in := orggraph.Input{Orgs: orgNodesFromProto(tree.GetItems()), Agents: agents, Now: now}
	partial := []string{}

	if items, total, ok := s.loadInbox(ctx); ok {
		in.Inbox, in.InboxTotal = items, total
	} else {
		partial = append(partial, partialInbox)
	}
	if work, err := s.loadWork(ctx); err == nil {
		in.Work = work
	} else {
		s.lg.Warn("org graph: delegated tasks unavailable", loggateway.Err(err))
		partial = append(partial, partialDelegated)
	}
	tasks, cronOK := s.loadCronTasks(ctx)
	if cronOK {
		in.Cron = cronNextRuns(tasks)
	} else {
		partial = append(partial, partialCron)
	}
	if usage, err := s.loadUsage(ctx); err == nil {
		in.Usage = usage
	} else {
		partial = append(partial, partialUsage)
	}

	board := orggraph.Build(in)
	if cronOK {
		s.attachGM(ctx, &board, tasks)
	}
	return OrgGraph{GeneratedAt: now.Format(time.RFC3339), Partial: partial, Board: board}, nil
}

func (s *BoardService) loadAgents(ctx context.Context) ([]orggraph.Agent, error) {
	var out []orggraph.Agent
	for offset := 0; ; offset += orgGraphAgentPg {
		page, err := s.deps.Agents.ListAgents(ctx, &agentv1.ListAgentsRequest{Limit: orgGraphAgentPg, Offset: int32(offset)})
		if err != nil {
			return nil, err
		}
		for _, a := range page.GetItems() {
			out = append(out, orggraph.Agent{
				ID:            a.GetId(),
				Key:           a.GetAgentKey(),
				Name:          a.GetDisplayName(),
				PositionID:    a.GetPositionId(),
				LastRunStatus: a.GetLastRunStatus(),
				LastRunAt:     a.GetLastRunAt(),
				DeletedAt:     a.GetDeletedAt(),
			})
		}
		if len(page.GetItems()) < orgGraphAgentPg || len(out) >= int(page.GetTotal()) {
			return out, nil
		}
	}
}

// loadInbox 读等我拍板的事；总经理汇报（kind=report）是要我读的，不算拍板。
func (s *BoardService) loadInbox(ctx context.Context) ([]orggraph.InboxItem, int, bool) {
	if s.deps.Inbox == nil {
		return nil, 0, false
	}
	res, err := s.deps.Inbox.List(ctx, nil, 1, 200)
	if err != nil {
		s.lg.Warn("org graph: inbox unavailable", loggateway.Err(err))
		return nil, 0, false
	}
	items := make([]orggraph.InboxItem, 0, len(res.Items))
	for _, it := range res.Items {
		if it.Kind == inbox.KindReport {
			continue
		}
		due := ""
		if !it.DueAt.IsZero() {
			due = it.DueAt.Format(time.RFC3339)
		}
		items = append(items, orggraph.InboxItem{ID: it.ID, Title: it.Title, AgentKey: it.AgentKey, DueAt: due, RiskLevel: it.Risk, Link: it.Link})
	}
	total := int(res.Total - res.Counts[inboxSourceGMReport])
	return items, total, len(res.Degraded) == 0
}

// loadWork 读交办登记并按承载会话的执行态归成三态（同 today/logic 的 classifyOfficeDelegated）。
func (s *BoardService) loadWork(ctx context.Context) ([]orggraph.WorkItem, error) {
	if s.deps.Office == nil {
		return nil, errBoardSourceMissing
	}
	res, err := s.deps.Office.ListOfficeTasks(ctx, &officev1.ListOfficeTasksRequest{Status: "open", Source: "delegate", Limit: orgGraphFetch})
	if err != nil {
		return nil, err
	}
	tasks := res.GetItems()
	if len(tasks) == 0 {
		return []orggraph.WorkItem{}, nil
	}
	// 执行态读不到时按「在做」处理（与工作台交办列表同一口径）。
	latest := map[string]*sessionv1.TaskSituationV2{}
	if s.deps.Sessions != nil {
		page, err := s.deps.Sessions.ListTaskSituations(ctx, &sessionv1.ListTaskSituationsRequest{Limit: orgGraphFetch, SortBy: "updated_at", SortOrder: "desc"})
		if err != nil {
			s.lg.Warn("org graph: task situations unavailable", loggateway.Err(err))
		} else {
			for _, row := range page.GetItems() {
				seen := latest[row.GetSessionId()]
				if seen == nil || orggraph.ParseTime(row.GetUpdatedAt()).After(orggraph.ParseTime(seen.GetUpdatedAt())) {
					latest[row.GetSessionId()] = row
				}
			}
		}
	}
	return classifyDelegated(tasks, latest), nil
}

func classifyDelegated(tasks []*officev1.OfficeTask, latest map[string]*sessionv1.TaskSituationV2) []orggraph.WorkItem {
	out := make([]orggraph.WorkItem, 0, len(tasks))
	for _, t := range tasks {
		if t.GetId() == "" {
			continue
		}
		// 承载会话：session_id（派给 AI 的待办 source_ref 是邮件 id 等来源）；老数据回落 source_ref
		session := t.GetSessionId()
		if session == "" {
			session = t.GetSourceRef()
		}
		sit := latest[session]
		state := orggraph.WorkRunning
		if sit != nil && delegatedStuck[sit.GetStatus()] {
			state = orggraph.WorkStuck
		} else if sit != nil && sit.GetStatus() == "completed" {
			state = orggraph.WorkReview
		}
		at := ""
		if sit != nil {
			at = sit.GetUpdatedAt()
		}
		if at == "" && t.GetCreatedAt() != nil {
			at = t.GetCreatedAt().AsTime().Format(time.RFC3339)
		}
		owner := "agent"
		if t.GetAssigneeKind() == "team" {
			owner = "team"
		}
		to := "/chat"
		if ref := session; ref != "" {
			to = "/chat?session=" + url.QueryEscape(ref) + "&" + owner + "=" + url.QueryEscape(t.GetAssigneeId())
		}
		title := firstLineOf(t.GetTitle())
		if title == "" {
			title = t.GetId()
		}
		out = append(out, orggraph.WorkItem{
			ID: "office:" + t.GetId(), Title: title, AssigneeKind: t.GetAssigneeKind(), AssigneeID: t.GetAssigneeId(),
			State: state, At: at, To: to,
		})
	}
	return out
}

func (s *BoardService) loadCronTasks(ctx context.Context) ([]biz.CronTask, bool) {
	if s.deps.Cron == nil || !inboxCan(ctx, inboxPermCron) {
		return nil, false
	}
	tasks, err := s.deps.Cron.ListTasks(ctx)
	if err != nil {
		s.lg.Warn("org graph: cron tasks unavailable", loggateway.Err(err))
		return nil, false
	}
	return tasks, true
}

// cronMeta 是定时任务 metadata_json 里看板用到的几项（由 cronrunner 维护）。
type cronMeta struct {
	NextRunAt     string `json:"next_run_at"`
	LastRunAt     string `json:"last_run_at"`
	LastRunStatus string `json:"last_run_status"`
}

func parseCronMeta(raw string) cronMeta {
	var m cronMeta
	_ = json.Unmarshal([]byte(raw), &m)
	return m
}

func cronNextRuns(tasks []biz.CronTask) []orggraph.Cron {
	out := make([]orggraph.Cron, 0, len(tasks))
	for _, t := range tasks {
		if t.Enabled && t.AgentID != "" {
			out = append(out, orggraph.Cron{AgentID: t.AgentID, NextRunAt: parseCronMeta(t.MetadataJSON).NextRunAt})
		}
	}
	return out
}

func (s *BoardService) loadUsage(ctx context.Context) ([]orggraph.Usage, error) {
	if s.deps.Usage == nil || !inboxCan(ctx, "usage.read") {
		return nil, errBoardSourceMissing
	}
	res, err := s.deps.Usage.ListTopAgents(ctx, &usagev1.UsageQuery{Range: "today", Limit: orgGraphUsageLimit})
	if err != nil {
		s.lg.Warn("org graph: usage unavailable", loggateway.Err(err))
		return nil, err
	}
	out := make([]orggraph.Usage, 0, len(res.GetItems()))
	for _, row := range res.GetItems() {
		if row.GetAgentId() != "" {
			out = append(out, orggraph.Usage{AgentID: row.GetAgentId(), Calls: int64(row.GetCallCount()), CostMicro: row.GetTotalCostMicroUsd()})
		}
	}
	return out, nil
}

// attachGM 按总经理的两条定时任务填公司的巡视 / 汇报状态。
func (s *BoardService) attachGM(ctx context.Context, board *orggraph.Board, tasks []biz.CronTask) {
	byKey := map[string]biz.CronTask{}
	for _, t := range tasks {
		if strings.HasPrefix(t.TaskKey, gmPatrolKeyPrefix) || strings.HasPrefix(t.TaskKey, gmReportKeyPrefix) {
			byKey[t.TaskKey] = t
		}
	}
	for i := range board.Companies {
		c := &board.Companies[i]
		patrol, hasPatrol := byKey[gmPatrolKeyPrefix+c.ID]
		report, hasReport := byKey[gmReportKeyPrefix+c.ID]
		if !hasPatrol && !hasReport {
			continue
		}
		gm := &orggraph.GMStatus{Enabled: patrol.Enabled || report.Enabled}
		if hasPatrol {
			m := parseCronMeta(patrol.MetadataJSON)
			if m.LastRunStatus == "success" {
				gm.LastPatrolAt = m.LastRunAt
			}
			if patrol.Enabled {
				gm.NextPatrolAt = m.NextRunAt
			}
		}
		if hasReport {
			if report.Enabled {
				gm.NextReportAt = parseCronMeta(report.MetadataJSON).NextRunAt
			}
			if run, out, ok := latestGMRun(ctx, s.deps.Cron, report.ID); ok {
				gm.LastReport = &orggraph.GMReport{
					RunID: run.ID, At: firstNonEmptyStr(run.FinishedAt, run.CreatedAt), Conclusion: out.Conclusion,
					Reasons: out.Reasons, Writer: out.Writer, Acked: run.ResolvedAt != "",
				}
			}
		}
		c.GM = gm
	}
}

func orgNodesFromProto(items []*organizationv1.OrganizationTreeNode) []orggraph.OrgNode {
	out := make([]orggraph.OrgNode, 0, len(items))
	for _, it := range items {
		n := it.GetNode()
		out = append(out, orggraph.OrgNode{
			ID:              n.GetId(),
			Name:            n.GetName(),
			Level:           n.GetLevel(),
			Enabled:         n.GetEnabled(),
			SortOrder:       int(n.GetSortOrder()),
			DeletedAt:       n.GetDeletedAt(),
			DeptLeadAgentID: n.GetDeptLeadAgentId(),
			MetadataJSON:    n.GetMetadataJson(),
			Children:        orgNodesFromProto(it.GetChildren()),
		})
	}
	return out
}

func firstLineOf(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// errBoardSourceMissing：来源没装配或调用方无权读，按「读不到」记进 partial。
var errBoardSourceMissing = errors.New("board source unavailable")
