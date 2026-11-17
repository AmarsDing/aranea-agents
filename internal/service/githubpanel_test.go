package service

// V1 续（E 阶段 W2c G）：GitHubPanelService 权限闸门——middleware 登记表的
// 第二道保险。与 secops_test.go 同例：零业务依赖装配（biz 层对 nil repo
// 有 Internal 守卫，不触空指针），用「过门后的 BadRequest/Internal」证明
// 权限门未拦；正路径由 biz/data 层测试覆盖。

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/github/v1"
	"aranea-agents/internal/biz/githubpanel"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"

	"google.golang.org/protobuf/types/known/emptypb"
)

func githubCtx(uid int64, roles ...string) context.Context {
	return auth.NewContext(context.Background(), &auth.Auth{UserID: uid, Roles: roles})
}

// newGitHubProbeService 装配零依赖 probe 服务：usecase 的 repo 为 nil，
// 过权限门后 biz 返回 Internal（未装配）。
func newGitHubProbeService() *GitHubPanelService {
	uc := githubpanel.NewUsecase(nil, nil, loggateway.NewNoop())
	return NewGitHubPanelService(uc, loggateway.NewNoop())
}

// TestGitHubPanelService_PermissionGates 钉死四道门：
//   - 未认证 → Unauthorized（任何 RPC）；
//   - auditor（无 github.*）→ Forbidden；
//   - viewer/ceo（仅 github.view）→ manage 三件套 Forbidden，view 二件过门
//     （biz nil repo 回 Internal 证明未被权限门拦）；
//   - sysadmin（github.manage）→ watch/unwatch 过门（BadRequest 证明），
//     但 view 二件 Forbidden——钉死 W0 登记映射：manage 不蕴含 view
//     （运维读面是否补 github.view 码位，登记 W4 评审裁决）；
//   - superadmin（*）→ view 过门。
func TestGitHubPanelService_PermissionGates(t *testing.T) {
	svc := newGitHubProbeService()
	noAuth := context.Background()

	cases := []struct {
		name string
		ctx  context.Context
		call func(context.Context) error
		code apierror.Code
	}{
		{"list no auth", noAuth, func(c context.Context) error {
			_, err := svc.ListWatchedRepos(c, &emptypb.Empty{})
			return err
		}, apierror.CodeUnauthorized},
		{"health no auth", noAuth, func(c context.Context) error {
			_, err := svc.GetRepoHealth(c, &emptypb.Empty{})
			return err
		}, apierror.CodeUnauthorized},
		{"watch no auth", noAuth, func(c context.Context) error {
			_, err := svc.WatchRepo(c, &v1.WatchRepoRequest{FullName: "a/b", CredentialId: "1"})
			return err
		}, apierror.CodeUnauthorized},
		{"unwatch no auth", noAuth, func(c context.Context) error {
			_, err := svc.UnwatchRepo(c, &v1.UnwatchRepoRequest{Id: "x"})
			return err
		}, apierror.CodeUnauthorized},
		{"list auditor forbidden", githubCtx(91, "auditor"), func(c context.Context) error {
			_, err := svc.ListWatchedRepos(c, &emptypb.Empty{})
			return err
		}, apierror.CodeForbidden},
		{"health auditor forbidden", githubCtx(91, "auditor"), func(c context.Context) error {
			_, err := svc.GetRepoHealth(c, &emptypb.Empty{})
			return err
		}, apierror.CodeForbidden},
		{"watch auditor forbidden", githubCtx(91, "auditor"), func(c context.Context) error {
			_, err := svc.WatchRepo(c, &v1.WatchRepoRequest{FullName: "a/b", CredentialId: "1"})
			return err
		}, apierror.CodeForbidden},
		{"watch viewer forbidden", githubCtx(31, "viewer"), func(c context.Context) error {
			_, err := svc.WatchRepo(c, &v1.WatchRepoRequest{FullName: "a/b", CredentialId: "1"})
			return err
		}, apierror.CodeForbidden},
		{"unwatch ceo forbidden", githubCtx(61, "ceo"), func(c context.Context) error {
			_, err := svc.UnwatchRepo(c, &v1.UnwatchRepoRequest{Id: "x"})
			return err
		}, apierror.CodeForbidden},
		{"list sysadmin forbidden (manage≠view)", githubCtx(71, "sysadmin"), func(c context.Context) error {
			_, err := svc.ListWatchedRepos(c, &emptypb.Empty{})
			return err
		}, apierror.CodeForbidden},
		// sysadmin 过 manage 门：输入校验给出 BadRequest（证明未被权限门拦）。
		{"watch sysadmin bad full_name", githubCtx(71, "sysadmin"), func(c context.Context) error {
			_, err := svc.WatchRepo(c, &v1.WatchRepoRequest{FullName: "noslash", CredentialId: "1"})
			return err
		}, apierror.CodeBadRequest},
		{"watch sysadmin bad credential", githubCtx(71, "sysadmin"), func(c context.Context) error {
			_, err := svc.WatchRepo(c, &v1.WatchRepoRequest{FullName: "a/b", CredentialId: "0"})
			return err
		}, apierror.CodeBadRequest},
		{"unwatch sysadmin empty id", githubCtx(71, "sysadmin"), func(c context.Context) error {
			_, err := svc.UnwatchRepo(c, &v1.UnwatchRepoRequest{Id: "  "})
			return err
		}, apierror.CodeBadRequest},
		// view 门过门：biz nil repo 回 Internal。
		{"list viewer passes gate", githubCtx(31, "viewer"), func(c context.Context) error {
			_, err := svc.ListWatchedRepos(c, &emptypb.Empty{})
			return err
		}, apierror.CodeInternal},
		{"health ceo passes gate", githubCtx(61, "ceo"), func(c context.Context) error {
			_, err := svc.GetRepoHealth(c, &emptypb.Empty{})
			return err
		}, apierror.CodeInternal},
		{"health superadmin passes gate", githubCtx(99, "superadmin"), func(c context.Context) error {
			_, err := svc.GetRepoHealth(c, &emptypb.Empty{})
			return err
		}, apierror.CodeInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(tc.ctx); !apierror.IsCode(err, tc.code) {
				t.Fatalf("want code %s, got %v", tc.code, err)
			}
		})
	}
}

// TestGitHubPanelService_Unwired 钉死结构体层兜底：usecase 未装配时任何
// RPC 返回 Internal（nil 服务在 wire 层另有拦截，这里钉死 service 层守卫）。
func TestGitHubPanelService_Unwired(t *testing.T) {
	svc := &GitHubPanelService{}
	if _, err := svc.ListWatchedRepos(githubCtx(31, "viewer"), &emptypb.Empty{}); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("unwired service must be Internal, got %v", err)
	}
	if _, err := svc.WatchRepo(githubCtx(71, "sysadmin"), &v1.WatchRepoRequest{FullName: "a/b", CredentialId: "1"}); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("unwired service must be Internal, got %v", err)
	}
}
