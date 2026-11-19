package main

// E 阶段 W2c G：GitHub 研发情报面板装配（需求 §4，拍板 G-1/G-2）。
//
// 与 wire_secops.go 同例：改定义只需动本文件 + wire_gen.go 手工镜像
//（头部注释约定的共享 provider 纪律）。

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/githubpanel"
	"aranea-agents/internal/data"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// githubCredentialRefChecker 把凭据用例适配成 githubpanel 的存在性核验
// 窄口（G-2：watch 只核验凭据存在，不取明文——明文仅执行面经
// RevealSecretForExecution 进程内取用）。
type githubCredentialRefChecker struct {
	uc *biz.CredentialUsecase
}

func (c githubCredentialRefChecker) CredentialExists(ctx context.Context, id int64) (bool, error) {
	if c.uc == nil {
		return false, nil // 未装配凭据设施时保守放行（与 d==nil 停用口径一致）
	}
	_, err := c.uc.Get(ctx, id)
	if err == nil {
		return true, nil
	}
	if ae, ok := apierror.From(err); ok && ae.Code == apierror.CodeNotFound {
		return false, nil
	}
	return false, err
}

// provideGitHubPanelUsecase 装配 GitHub 面板用例（审计后置 setter，与
// secops 同坐标）。
func provideGitHubPanelUsecase(d *data.Data, cred *biz.CredentialUsecase, lg loggateway.Logger) *githubpanel.Usecase {
	uc := githubpanel.NewUsecase(data.NewGitHubPanelRepo(d), githubCredentialRefChecker{uc: cred}, lg)
	if d != nil {
		uc.SetAuditRecorder(data.NewMonitorAuditRepo(d))
	}
	return uc
}

// provideGitHubPanelService 装配 github.v1 服务。
func provideGitHubPanelService(uc *githubpanel.Usecase, lg loggateway.Logger) *service.GitHubPanelService {
	return service.NewGitHubPanelService(uc, lg)
}
