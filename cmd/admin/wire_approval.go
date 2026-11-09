package main

// approval 域 providers（E 阶段 W1b 统一审批中心，需求文档 §5.3 拍板 A-1/A-4）。
//
// 本文件不带构建标签：wire.go/wire_part2.go（wireinject）与 wire_gen.go
// （!wireinject，手工镜像）共享同一份 provider 定义，循 wire_ceoboard.go 先例；
// 改动只需维护本文件 + wire_gen.go 的装配调用。

import (
	bridge "aranea-agents/internal/biz/agentbridge"
	"aranea-agents/internal/biz/approval"
	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/data"
	"aranea-agents/internal/service"
)

// provideApprovalTaskReviewReader 将 agentbridge TaskRepo 窄化为审批中心
// 任务验收读口（ListPendingReview 已挂进 TaskRepo 接口，断言恒成功；
// 失败时返回 nil → 源不注册，诚实降级为空 lane 而不是启动失败）。
func provideApprovalTaskReviewReader(repo bridge.TaskRepo) approval.CodingTaskReviewReader {
	if r, ok := repo.(approval.CodingTaskReviewReader); ok {
		return r
	}
	return nil
}

// provideApprovalEvolutionReader 将统一进化存储窄化为审批中心进化读口
// （ListByTarget/CountByTarget 签名与 approval.EvolutionQueryReader 一致）。
func provideApprovalEvolutionReader(repo *data.UnifiedEvolutionRepo) approval.EvolutionQueryReader {
	return repo
}

// provideApprovalAggregator 装配审批聚合器：一期三类源（任务验收/进化建议/
// 技能审批）+ W1c 日程草案（schedule_draft，可见性闸门 office.schedule）
// + W2a D3 离职导出（offboard_export，闸门 user.offboard 且非自提）。
// reader 为 nil 时对应源不注册（NewXxxSource nil→nil 语义），
// mask_proposal/memory_fact 随 D6/D3 接入。
func provideApprovalAggregator(tasks approval.CodingTaskReviewReader, evo approval.EvolutionQueryReader, drafts approval.ScheduleDraftReader, offboard approval.OffboardRequestReader) *approval.Aggregator {
	return approval.NewAggregator(
		approval.NewTaskReviewSource(tasks),
		approval.NewEvolutionSuggestionSource(evo),
		approval.NewSkillApprovalSource(evo),
		approval.NewScheduleDraftSource(drafts),
		approval.NewOffboardExportSource(offboard),
	)
}

// provideApprovalMatrix 装配分级审批矩阵（A-4）：raw-SQL 仓储（迁移
// 20261323）+ monitor 管理审计适配（best-effort，失败仅告警）。
func provideApprovalMatrix(d *data.Data, mon *monitor.Usecase) *approval.Matrix {
	return approval.NewMatrix(data.NewApprovalRiskMatrixRepo(d), service.NewMatrixAuditSink(mon))
}

// provideApprovalCenterService 装配统一审批中心服务（/v1/approvals*）。
func provideApprovalCenterService(agg *approval.Aggregator, matrix *approval.Matrix) *service.ApprovalCenterService {
	return service.NewApprovalCenterService(agg, matrix)
}

// provideReviewService 装配 review.v1 收编服务（/v1/review/*；无依赖——
// 运行时走 mvp.Ensure() 单例，与旧裸路由同源）。
func provideReviewService() *service.ReviewService {
	return service.NewReviewService()
}
