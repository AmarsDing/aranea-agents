package service

import (
	"context"

	v1 "aranea-agents/api/kratos/github/v1"
	"aranea-agents/internal/biz/githubpanel"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ============================================================================
// GitHubPanelService 实现 github.v1（E 阶段 W2c G，需求 §4）。
//
// 权限模型（双保险）：
//   - middleware 硬判：github.view（四读 RPC）/github.manage（watch 三件套
//     + TriggerPoll）——登记表由 make api 生成，非手工。
//   - service 兜底：每 RPC 复核调用者身份（防登记表被旁路或豁免误配）。
//
// V1 落位：watch 三件套 + GetRepoHealth（健康快照读+stale 诚实裁决）。
// SLA/热榜/手动轮询由 V3-V5 纵列收编——未实现的 RPC 经嵌入的
// UnimplementedGitHubPanelServiceServer 返回 Unimplemented。
// ============================================================================

// GitHubPanelService 是 github.v1 服务实现。
type GitHubPanelService struct {
	v1.UnimplementedGitHubPanelServiceServer
	uc *githubpanel.Usecase
	lg loggateway.Logger
}

// NewGitHubPanelService 装配 GitHub 面板服务。
func NewGitHubPanelService(uc *githubpanel.Usecase, lg loggateway.Logger) *GitHubPanelService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &GitHubPanelService{uc: uc, lg: lg}
}

func (s *GitHubPanelService) requireWired() error {
	if s.uc == nil {
		return apierror.Internal("GITHUB", "githubpanel usecase not wired")
	}
	return nil
}

// githubCaller 取 JWT 主体；无主体 → Unauthorized（middleware 之前不得放行）。
func githubCaller(ctx context.Context) (*auth.Auth, error) {
	a, ok := auth.FromContext(ctx)
	if !ok || a == nil {
		return nil, apierror.Unauthorized("GITHUB", "authentication required")
	}
	return a, nil
}

func requireGitHubView(a *auth.Auth) error {
	if !a.HasPermission("github.view") {
		return apierror.Forbidden("GITHUB", "github.view permission required")
	}
	return nil
}

func requireGitHubManage(a *auth.Auth) error {
	if !a.HasPermission("github.manage") {
		return apierror.Forbidden("GITHUB", "github.manage permission required")
	}
	return nil
}

func watchedRepoToProto(r githubpanel.WatchedRepo) *v1.WatchedRepo {
	out := &v1.WatchedRepo{
		Id:           r.ID,
		FullName:     r.FullName,
		CredentialId: r.CredentialID,
		WebhookOk:    r.WebhookOK,
	}
	if !r.LastEventAt.IsZero() {
		out.LastEventAt = timestamppb.New(r.LastEventAt)
	}
	if !r.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(r.CreatedAt)
	}
	return out
}

// ListWatchedRepos 列出观测仓库（github.view）。
func (s *GitHubPanelService) ListWatchedRepos(ctx context.Context, _ *emptypb.Empty) (*v1.ListWatchedReposResponse, error) {
	if err := s.requireWired(); err != nil {
		return nil, err
	}
	caller, err := githubCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireGitHubView(caller); err != nil {
		return nil, err
	}
	repos, err := s.uc.ListWatched(ctx)
	if err != nil {
		return nil, err
	}
	out := &v1.ListWatchedReposResponse{Items: make([]*v1.WatchedRepo, 0, len(repos))}
	for _, r := range repos {
		out.Items = append(out.Items, watchedRepoToProto(r))
	}
	return out, nil
}

// WatchRepo 登记观测仓库（github.manage）。幂等：同 full_name 重 watch
// 更新 credential 绑定。
func (s *GitHubPanelService) WatchRepo(ctx context.Context, req *v1.WatchRepoRequest) (*v1.WatchedRepo, error) {
	if err := s.requireWired(); err != nil {
		return nil, err
	}
	caller, err := githubCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireGitHubManage(caller); err != nil {
		return nil, err
	}
	repo, err := s.uc.Watch(ctx, req.GetFullName(), req.GetCredentialId())
	if err != nil {
		return nil, err
	}
	return watchedRepoToProto(repo), nil
}

// UnwatchRepo 摘除观测仓库（github.manage）。
func (s *GitHubPanelService) UnwatchRepo(ctx context.Context, req *v1.UnwatchRepoRequest) (*emptypb.Empty, error) {
	if err := s.requireWired(); err != nil {
		return nil, err
	}
	caller, err := githubCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireGitHubManage(caller); err != nil {
		return nil, err
	}
	if err := s.uc.Unwatch(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// GetRepoHealth 仓库健康卡（github.view）：快照列 + stale 诚实裁决
//（health_updated_at 空或超窗 → stale=true，数据照给不伪造）。
func (s *GitHubPanelService) GetRepoHealth(ctx context.Context, _ *emptypb.Empty) (*v1.GetRepoHealthResponse, error) {
	if err := s.requireWired(); err != nil {
		return nil, err
	}
	caller, err := githubCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireGitHubView(caller); err != nil {
		return nil, err
	}
	health, err := s.uc.Health(ctx)
	if err != nil {
		return nil, err
	}
	out := &v1.GetRepoHealthResponse{Items: make([]*v1.RepoHealth, 0, len(health))}
	for _, h := range health {
		item := &v1.RepoHealth{
			FullName:         h.FullName,
			CiStatus:         h.CIStatus,
			OpenPrs:          int32(h.OpenPRs),
			OldestPrAgeHours: h.OldestPRAgeHours,
			OpenIssues:       int32(h.OpenIssues),
			Stale:            h.Stale,
		}
		if !h.AsOf.IsZero() {
			item.AsOf = timestamppb.New(h.AsOf)
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}
