package office

import (
	"context"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"aranea-agents/pkg/apierror"
)

// AppsUsecase 应用启动器（K-5）：URL 快捷方式注册/分组/删除。
// 打开动作在前端（新窗口），服务端不代理跳转。command 类本机命令启动属
// 远控执行面（须挂沙箱/审批闸），F2 另行评估，本期模型不含。
type AppsUsecase struct {
	repo AppRepo
}

func NewAppsUsecase(repo AppRepo) *AppsUsecase {
	return &AppsUsecase{repo: repo}
}

// ErrAppInvalidURL 启动器只接受 http/https 绝对 URL（防 javascript: 等
// 伪协议经「打开」动作注入）。
var ErrAppInvalidURL = apierror.BadRequest(DomainOffice, "app url must be http(s)")

func validateApp(a *App) error {
	if strings.TrimSpace(a.Name) == "" {
		return ErrTitleRequired
	}
	u, err := url.Parse(strings.TrimSpace(a.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ErrAppInvalidURL
	}
	a.URL = u.String()
	return nil
}

func (u *AppsUsecase) Upsert(ctx context.Context, a *App) (*App, error) {
	if err := validateApp(a); err != nil {
		return nil, err
	}
	if a.ID == "" {
		a.ID = "app_" + uuid.NewString()
	}
	if err := u.repo.UpsertApp(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (u *AppsUsecase) Delete(ctx context.Context, id, workspaceID string) error {
	return u.repo.DeleteApp(ctx, id, workspaceID)
}

func (u *AppsUsecase) List(ctx context.Context, workspaceID string) ([]*App, error) {
	return u.repo.ListApps(ctx, workspaceID)
}
