package main

// 看板总览 providers（设计稿 03-看板 §14）：常驻看板服务（org-graph + 经营快照）
// 与总经理 Agent 的巡视 / 汇报执行器。
//
// 本文件不带构建标签：wire.go/wire_part2.go（wireinject）与 wire_gen.go
// （!wireinject，手工镜像）共享同一份 provider 定义，循 wire_ceoboard.go 先例；
// 改动只需维护本文件 + wire_gen.go 的装配调用。

import (
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/ceoboard/bizview"
	"aranea-agents/internal/cronrunner"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/service"
	loggateway "aranea-agents/pkg/loggateway"
)

// provideGMCronStore 把 cron 仓储窄化为总经理巡视用的两个口子（断言恒成功；
// 失败返回 nil → 不补建任务、汇报来源不注册，诚实降级而不是启动失败）。
func provideGMCronStore(repo biz.CronRepo) service.GMCronStore {
	if s, ok := repo.(service.GMCronStore); ok {
		return s
	}
	return nil
}

// provideBoardService 装配常驻看板服务，并把总经理执行器挂到定时任务 runner 上
// （runner 先于这些 service 装配，故在此回填；runner 为 nil = 定时任务被关闭）。
// 经营快照只读主库：entsql.OpenDB 仅包装共享连接池，同 provideCeoboardStore。
func provideBoardService(
	org *service.OrganizationService,
	agents *service.AgentService,
	approvals *service.ApprovalCenterService,
	office *service.OfficeMailService,
	sessions *service.SessionV2Service,
	cron *biz.CronUsecase,
	usage *service.UsageService,
	d *data.Data,
	runner *cronrunner.Runner,
	gmStore service.GMCronStore,
	admins biz.AdminReader,
	chat *service.ChatService,
	lg loggateway.Logger,
) *service.BoardService {
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, providePrimaryRawDB(d))))
	board := service.NewBoardService(service.BoardDeps{
		Org:      org,
		Agents:   agents,
		Inbox:    approvals.InboxAggregator(),
		Office:   office,
		Sessions: sessions,
		Cron:     cron,
		Usage:    usage,
		Biz:      bizview.NewBuilder(client),
	}, lg)
	runner.WithGM(service.NewGMRunner(board, cron, gmStore, admins, chat, lg))
	return board
}
