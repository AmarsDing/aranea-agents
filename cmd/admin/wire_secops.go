package main

// secops 离职处置 providers（E 阶段 W2a D3 offboard 五步，需求 §6.3）。
// 本文件不带构建标签：wire_part2.go（wireinject）与 wire_gen.go（!wireinject，
// 手工镜像）共享同一份 provider 定义，循 wire_approval.go/wire_memory.go 先例；
// 改动同步维护 wire_gen.go 的装配调用。

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/approval"
	"aranea-agents/internal/data"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/loggateway"
)

// provideSecopsOffboardRepo 装配离职申请单台账（nil Data → nil，源不注册）。
func provideSecopsOffboardRepo(d *data.Data) biz.SecopsOffboardRequestRepo {
	if d == nil {
		return nil
	}
	return data.NewSecopsOffboardRepo(d)
}

// provideApprovalOffboardReader 将台账窄化为审批中心「离职导出」lane 读口
// （方法集超集转接口，与 provideApprovalEvolutionReader 同法）。
func provideApprovalOffboardReader(repo biz.SecopsOffboardRequestRepo) approval.OffboardRequestReader {
	if repo == nil {
		return nil
	}
	return repo
}

// provideSecopsOffboardUsecase 装配离职处置用例：admin 读写 + 台账 + L3 事实
// 导出/清理（同一 l3FactRepo 适配器，口径与导出对称）+ 会话吊销 + 审计
// （构造签名取硬依赖，revoker/audit 后置 setter 注入）。
func provideSecopsOffboardUsecase(adminReader biz.AdminReader, adminWriter biz.AdminWriter, repo biz.SecopsOffboardRequestRepo, d *data.Data, revoker *data.TokenVersionStore, lg loggateway.Logger) *biz.SecopsOffboardUsecase {
	uc := biz.NewSecopsOffboardUsecase(adminReader, adminWriter, repo,
		data.NewSecopsOffboardFactLister(d), data.NewSecopsOffboardFactClearer(d), lg)
	if revoker != nil {
		uc.SetSessionRevoker(revoker)
	}
	if d != nil {
		uc.SetSecopsOffboardAuditRecorder(data.NewMonitorAuditRepo(d))
	}
	return uc
}

// provideSecopsService 装配 secops.v1 服务（离职五步 + D6 密级标签/模型
// 策略四 RPC；脱敏五闸随 D6 后续纵列接入）。
func provideSecopsService(uc *biz.SecopsOffboardUsecase, classification *biz.SecopsClassificationUsecase, policy *biz.SecopsModelPolicyUsecase, lg loggateway.Logger) *service.SecopsService {
	return service.NewSecopsService(uc, classification, policy, lg)
}

// ── D6 W2b 密级面（L-1/L-4：标签/水位线/模型策略，DDL 迁移 20261327）────

// provideSecopsClassificationRepo 装配密级标签台账（nil Data → nil）。
func provideSecopsClassificationRepo(d *data.Data) biz.SecopsClassificationRepo {
	if d == nil {
		return nil
	}
	return data.NewSecopsClassificationRepo(d)
}

// provideSecopsSessionWaterlineRepo 装配会话/运行水位线台账（GREATEST
// 只升不降写路径；nil Data → nil）。
func provideSecopsSessionWaterlineRepo(d *data.Data) biz.SecopsSessionWaterlineRepo {
	if d == nil {
		return nil
	}
	return data.NewSecopsSessionWaterlineRepo(d)
}

// provideSecopsModelPolicyRepo 装配模型策略台账（nil Data → nil）。
func provideSecopsModelPolicyRepo(d *data.Data) biz.SecopsModelPolicyRepo {
	if d == nil {
		return nil
	}
	return data.NewSecopsModelPolicyRepo(d)
}

// provideSecopsClassificationUsecase 装配密级用例（标签 + 水位线；
// 审计后置 setter 注入，wire 友好）。
func provideSecopsClassificationUsecase(labels biz.SecopsClassificationRepo, waterline biz.SecopsSessionWaterlineRepo, d *data.Data, lg loggateway.Logger) *biz.SecopsClassificationUsecase {
	uc := biz.NewSecopsClassificationUsecase(labels, waterline, lg)
	if d != nil {
		uc.SetSecopsClassificationAuditRecorder(data.NewMonitorAuditRepo(d))
	}
	return uc
}

// provideSecopsModelPolicyUsecase 装配模型策略用例（热副本：selector
// 每 turn 判定打缓存，Set 写即失效；审计后置 setter 注入）。
func provideSecopsModelPolicyUsecase(repo biz.SecopsModelPolicyRepo, d *data.Data, lg loggateway.Logger) *biz.SecopsModelPolicyUsecase {
	uc := biz.NewSecopsModelPolicyUsecase(repo, lg)
	if d != nil {
		uc.SetSecopsModelPolicyAuditRecorder(data.NewMonitorAuditRepo(d))
	}
	return uc
}
