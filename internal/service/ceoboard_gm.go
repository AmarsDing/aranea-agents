// 总经理 Agent 的巡视与汇报（设计稿 03-看板 §14.3）。
//
//   - 定时：每家有总经理的公司两条定时任务，内置目标 gm_patrol（每小时 :10）/
//     gm_report（08:20 / 12:20 / 20:20），task_key = <目标>:<公司 id>，启动时补建；
//     已存在（含被删除或停用的）不重建、不重开。
//   - 巡视：读本公司那一块 org-graph，按规则记事实（不调模型）。
//   - 汇报：当场巡视 + 和上次汇报比，把事实交给总经理用自己的模型写「结论 + 理由」；
//     模型没答出来回落规则文本。结果进收件箱来源 gm_report（kind=report，只有「知道了」）。
//   - 以工作区负责人的身份看：每次运行取该工作区最早建立、仍在用的超级管理员，
//     按其当前角色取权限。
//   - 权限边界：只读 + 写自己的运行记录；不派活、不给其他 Agent 发消息、不改任何对象。
//
// 动作流水就是 cron_task_run：每次巡视 / 汇报一行，output_json.gm 存事实与结论。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	organizationv1 "aranea-agents/api/kratos/organization/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/ceoboard/orggraph"
	"aranea-agents/internal/biz/inbox"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	loggateway "aranea-agents/pkg/loggateway"

	"google.golang.org/protobuf/types/known/emptypb"
)

// 定时任务内置目标（cronrunner 按 target_type 分发到 GMRunner）。
const (
	gmPatrolTarget      = "gm_patrol"
	gmReportTarget      = "gm_report"
	inboxSourceGMReport = "gm_report"

	gmPatrolCron = "10 * * * *"
	gmReportCron = "20 8,12,20 * * *"
	gmTimezone   = "Asia/Shanghai"
	// 总经理写汇报的一轮对话上限；超时回落规则文本。
	gmReportTurnTimeout = 3 * time.Minute
)

// GMChat 让总经理在一个新的定时会话里回答一条消息并返回回答原文（*ChatService 实现）。
type GMChat interface {
	RunCronReport(ctx context.Context, agentID, userID, title, content string, timeout time.Duration) (sessionID, reply string, err error)
}

// GMCronStore 是总经理巡视用到、但 biz.CronRepo 没有的两个口子（*data.cronRepo 实现）。
type GMCronStore interface {
	// CronTaskKeyExists 含已软删除的任务（被删过的不重建）。
	CronTaskKeyExists(ctx context.Context, taskKey string) (bool, error)
	// ResolveCronTaskRunsThrough 核销同任务、同状态、不晚于 runID 的全部未核销运行。
	ResolveCronTaskRunsThrough(ctx context.Context, runID, status, actor string) (int, error)
}

// GMRunner 执行总经理的巡视与汇报。
type GMRunner struct {
	board  *BoardService
	cron   *biz.CronUsecase
	store  GMCronStore
	admins biz.AdminReader
	chat   GMChat
	lg     loggateway.Logger
	now    func() time.Time
}

// NewGMRunner 装配；store / chat 为 nil 时分别不补建任务、汇报只用规则文本。
func NewGMRunner(board *BoardService, cron *biz.CronUsecase, store GMCronStore, admins biz.AdminReader, chat GMChat, lg loggateway.Logger) *GMRunner {
	return &GMRunner{board: board, cron: cron, store: store, admins: admins, chat: chat, lg: lg, now: time.Now}
}

type gmTaskConfig struct {
	CompanyID string `json:"company_id"`
}

// gmRunOutput 存进 cron_task_run.output_json.gm。
type gmRunOutput struct {
	Kind string `json:"kind"` // patrol | report
	orggraph.Patrol
	AgentKey   string   `json:"agent_key,omitempty"`
	Conclusion string   `json:"conclusion,omitempty"`
	Reasons    []string `json:"reasons,omitempty"`
	Writer     string   `json:"writer,omitempty"` // model | rule
	Note       string   `json:"note,omitempty"`
}

// RunGM 执行一次巡视或汇报，返回承载会话 id（汇报才有）与要并进运行记录的输出。
func (g *GMRunner) RunGM(ctx context.Context, task biz.CronTask, target string) (string, map[string]any, error) {
	var cfg gmTaskConfig
	_ = json.Unmarshal([]byte(task.ConfigJSON), &cfg)
	if cfg.CompanyID == "" {
		return "", nil, apierror.BadRequest("CRON", "company_id is required for "+target)
	}
	ownerCtx, owner, err := g.ownerContext(ctx, task.WorkspaceID)
	if err != nil {
		return "", nil, err
	}
	graph, err := g.board.Load(ownerCtx)
	if err != nil {
		return "", nil, err
	}
	var company *orggraph.CompanyNode
	for i := range graph.Companies {
		if graph.Companies[i].ID == cfg.CompanyID {
			company = &graph.Companies[i]
		}
	}
	if company == nil {
		return "", nil, apierror.NotFound("CRON", "公司不存在，或工作区负责人看不到它："+cfg.CompanyID)
	}
	now := g.now()
	out := gmRunOutput{Kind: "patrol", Patrol: orggraph.PatrolCompany(*company, now)}
	if company.CEO != nil {
		out.AgentKey = company.CEO.Key
	}
	if target != gmReportTarget {
		return "", map[string]any{"gm": out}, nil
	}

	out.Kind = "report"
	var prev *orggraph.Patrol
	prevAt := ""
	if run, p, ok := latestGMRun(ownerCtx, g.cron, task.ID); ok {
		prev, prevAt = &p.Patrol, firstNonEmptyStr(run.FinishedAt, run.CreatedAt)
	}
	out.Conclusion = orggraph.Conclusion(out.Patrol)
	out.Reasons = orggraph.Reasons(out.Patrol, prev, prevAt, now)
	out.Writer = "rule"
	sessionID := ""
	switch {
	case company.CEO == nil:
		out.Note = "公司没有总经理 Agent，按规则生成"
	case g.chat == nil:
		out.Note = "对话服务不可用，按规则生成"
	default:
		prompt := orggraph.ReportPrompt(out.Patrol, out.Conclusion, out.Reasons, now)
		sid, reply, cerr := g.chat.RunCronReport(ctx, company.CEO.ID, strconv.FormatInt(owner.ID, 10), company.Name+" · 定时汇报", prompt, gmReportTurnTimeout)
		sessionID = sid
		if c, r, ok := orggraph.ParseReport(reply); cerr == nil && ok {
			out.Conclusion, out.Writer = c, "model"
			if len(r) > 0 {
				out.Reasons = r
			}
		} else {
			out.Note = "模型未回答，按规则生成"
			if cerr != nil {
				g.lg.Warn("gm report: model turn failed", loggateway.Str("company_id", company.ID), loggateway.Err(cerr))
			}
		}
	}
	return sessionID, map[string]any{"gm": out}, nil
}

// ownerContext 构造「工作区负责人」的读身份：最早建立、仍在用的超级管理员。
func (g *GMRunner) ownerContext(ctx context.Context, ws string) (context.Context, *biz.Admin, error) {
	if ws == "" {
		ws = workspace.DefaultWorkspaceID
	}
	if g.admins == nil {
		return nil, nil, apierror.Unavailable("CRON", "admin directory unavailable")
	}
	// shortcut: 只看前 500 个账号；单机部署够用，多租户大库时改成按角色查询。
	admins, err := g.admins.ListAdmins(workspace.WithSystemWorkspace(ctx), biz.ListLimit(500))
	if err != nil {
		return nil, nil, err
	}
	owner := pickWorkspaceOwner(admins, ws)
	if owner == nil {
		return nil, nil, apierror.NotFound("CRON", "工作区 "+ws+" 没有在用的超级管理员，总经理无人可代看")
	}
	roles := owner.Roles
	if len(roles) == 0 {
		roles = auth.LegacyRolesForAccess(owner.Access)
	}
	principal := &auth.Auth{UserID: owner.ID, Access: owner.Access, WorkspaceID: ws, Roles: roles, Clearance: owner.Clearance}
	return auth.NewContext(workspace.WithContext(ctx, ws), principal), owner, nil
}

func pickWorkspaceOwner(admins []*biz.Admin, ws string) *biz.Admin {
	var picked *biz.Admin
	for _, a := range admins {
		if a == nil || a.Status != biz.AdminStatusActive {
			continue
		}
		if (&auth.Auth{WorkspaceID: a.WorkspaceID}).EffectiveWorkspaceID() != ws {
			continue
		}
		roles := a.Roles
		if len(roles) == 0 {
			roles = auth.LegacyRolesForAccess(a.Access)
		}
		if !(&auth.Auth{Roles: roles}).HasRole("superadmin") {
			continue
		}
		if picked == nil || a.ID < picked.ID {
			picked = a
		}
	}
	return picked
}

// EnsureGMTasks 给每家有总经理的公司补建巡视 / 汇报两条定时任务（启动时调用，幂等）。
func (g *GMRunner) EnsureGMTasks(ctx context.Context) {
	if g.store == nil || g.cron == nil {
		return
	}
	sys := workspace.WithSystemWorkspace(ctx)
	tree, err := g.board.deps.Org.ListOrganizationTree(sys, &emptypb.Empty{})
	if err != nil {
		g.lg.Warn("gm: list organization tree failed", loggateway.Err(err))
		return
	}
	for _, item := range tree.GetItems() {
		n := item.GetNode()
		ceoID := orggraph.CompanyLeadID(n.GetMetadataJson())
		if n.GetLevel() != "company" || n.GetDeletedAt() != "" || ceoID == "" {
			continue
		}
		for _, spec := range []struct{ target, name, expr, message string }{
			{gmPatrolTarget, "总经理巡视", gmPatrolCron, "巡视本公司，记录要看一眼的事"},
			{gmReportTarget, "总经理汇报", gmReportCron, "按点给老板写一条「结论 + 理由」的汇报"},
		} {
			g.ensureTask(sys, n, ceoID, spec.target, spec.name, spec.expr, spec.message)
		}
	}
}

func (g *GMRunner) ensureTask(ctx context.Context, company *organizationv1.OrganizationNode, ceoID, target, name, expr, message string) {
	key := target + ":" + company.GetId()
	exists, err := g.store.CronTaskKeyExists(ctx, key)
	if err != nil || exists {
		if err != nil {
			g.lg.Warn("gm: check cron task failed", loggateway.Str("task_key", key), loggateway.Err(err))
		}
		return
	}
	cfg, _ := json.Marshal(map[string]any{
		"target_type":     target,
		"company_id":      company.GetId(),
		"schedule_type":   "cron",
		"cron_expression": expr,
		"timezone":        gmTimezone,
		"message":         message,
	})
	_, err = g.cron.CreateTask(ctx, biz.CronTask{
		TaskKey:     key,
		Name:        company.GetName() + " · " + name,
		Description: "看板总经理 Agent（设计稿 03-看板 §14）；只读 + 写汇报，不派活",
		Enabled:     true,
		Status:      "active",
		AgentID:     ceoID,
		ConfigJSON:  string(cfg),
		WorkspaceID: company.GetWorkspaceId(),
	})
	if err != nil {
		g.lg.Warn("gm: create cron task failed", loggateway.Str("task_key", key), loggateway.Err(err))
		return
	}
	g.lg.Info("gm: cron task created", loggateway.Str("task_key", key))
}

// latestGMRun 取一条定时任务最近一次成功运行及其 gm 输出（含已核销的）。
func latestGMRun(ctx context.Context, cron *biz.CronUsecase, taskID string) (biz.CronTaskRun, gmRunOutput, bool) {
	if cron == nil {
		return biz.CronTaskRun{}, gmRunOutput{}, false
	}
	runs, _, err := cron.ListTaskRuns(ctx, biz.CronTaskRunQuery{TaskID: taskID, Status: "success", Limit: 1, IncludeResolved: true})
	if err != nil || len(runs) == 0 {
		return biz.CronTaskRun{}, gmRunOutput{}, false
	}
	out, ok := parseGMOutput(runs[0].OutputJSON)
	return runs[0], out, ok
}

func parseGMOutput(raw string) (gmRunOutput, bool) {
	var wrap struct {
		GM *gmRunOutput `json:"gm"`
	}
	if err := json.Unmarshal([]byte(raw), &wrap); err != nil || wrap.GM == nil {
		return gmRunOutput{}, false
	}
	return *wrap.GM, true
}

// ============================================================================
// 收件箱来源：总经理汇报
// ============================================================================

// inboxGMReportSource 每家公司只列最近一条还没「知道了」的汇报（新汇报顶掉旧的）。
type inboxGMReportSource struct {
	cron  *biz.CronUsecase
	store GMCronStore
}

func (s *inboxGMReportSource) Name() string { return inboxSourceGMReport }

func (s *inboxGMReportSource) List(ctx context.Context, limit int) ([]inbox.Item, int64, error) {
	if !inboxCan(ctx, "ceoboard.view") {
		return nil, 0, nil
	}
	tasks, err := s.cron.ListTasks(ctx)
	if err != nil {
		return nil, 0, err
	}
	var items []inbox.Item
	for _, t := range tasks {
		if !strings.HasPrefix(t.TaskKey, gmReportKeyPrefix) {
			continue
		}
		run, out, ok := latestGMRun(ctx, s.cron, t.ID)
		if !ok || run.ResolvedAt != "" {
			continue
		}
		risk := inbox.RiskLow
		if out.State == orggraph.StateAttention {
			risk = inbox.RiskMedium
		}
		ctxJSON, _ := json.Marshal(map[string]any{"company_id": out.CompanyID, "reasons": out.Reasons, "writer": out.Writer, "note": out.Note})
		items = append(items, inbox.Item{
			NativeID:    run.ID,
			Kind:        inbox.KindReport,
			Title:       fmt.Sprintf("总经理汇报 · %s", firstNonEmptyStr(out.Company, t.Name)),
			Summary:     out.Conclusion,
			Risk:        risk,
			Requester:   "总经理",
			AgentKey:    out.AgentKey,
			CreatedAt:   orggraph.ParseTime(firstNonEmptyStr(run.FinishedAt, run.CreatedAt)),
			Actions:     []string{inbox.ActionAck},
			Link:        "/ceo/company",
			ContextJSON: string(ctxJSON),
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	total := int64(len(items))
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, total, nil
}

func (s *inboxGMReportSource) Decide(ctx context.Context, item inbox.Item, action, _ string) error {
	if action != inbox.ActionAck {
		return inbox.ErrActionNotAllowed
	}
	_, err := s.store.ResolveCronTaskRunsThrough(ctx, item.NativeID, "success", inboxActor(ctx))
	return err
}
