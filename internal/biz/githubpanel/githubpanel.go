// Package githubpanel GitHub 研发情报面板（E 阶段 W2c G，需求文档 §4）。
//
// 定位：研发情报面板（仓库健康/SLA/热榜结构化分析），不再造 GitHub。
// 接入（G-1）：GitHub webhook → ceoboard 入站通道（source=github，
// delivery ID 幂等）+ REST 轮询兜底（cron 10min）；token 存凭据模块
//（G-2：仅经 CredentialChecker 核验存在性 + 执行面
// RevealSecretForExecution 进程内取用，明文不出凭据设施）。
package githubpanel

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// 审计动作（Resource=github）。
const (
	AuditActionRepoWatch   = "github.repo.watch"
	AuditActionRepoUnwatch = "github.repo.unwatch"
)

// HealthStaleAfter 健康快照新鲜度窗口：轮询周期 10min 的两倍（拍板值）；
// 超窗或从未采集 → stale=true 诚实标记（读侧裁决，不落库）。
const HealthStaleAfter = 20 * time.Minute

var (
	// fullNameRe 钉死 owner/repo 形态（GitHub 命名规则子集，保守收紧）。
	fullNameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

	ErrFullNameInvalid     = apierror.BadRequest("GITHUB", "full_name must match owner/repo")
	ErrCredentialInvalid   = apierror.BadRequest("GITHUB", "credential_id must be a positive integer")
	ErrCredentialNotFound  = apierror.BadRequest("GITHUB", "credential_id does not reference an existing credential")
	ErrRepoNotFound        = apierror.NotFound("GITHUB", "watched repo not found")
	ErrRepoIDRequired      = apierror.BadRequest("GITHUB", "id is required")
	errRepoStoreUnavailable = apierror.Internal("GITHUB", "repository unavailable")
)

// WatchedRepo 观测仓库（github_repos 行投影）。
type WatchedRepo struct {
	ID           string
	FullName     string
	CredentialID string // 凭据模块条目 id（token 不出库）
	WebhookOK    bool
	LastEventAt  time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	// 健康快照列（webhook/poller 更新；GetRepoHealth 读侧裁 stale）
	CIStatus         string
	OpenPRs          int
	OldestPRAgeHours float64
	OpenIssues       int
	HealthUpdatedAt  time.Time
}

// RepoHealth 仓库健康卡（proto RepoHealth 同形）。
type RepoHealth struct {
	FullName         string
	CIStatus         string
	OpenPRs          int
	OldestPRAgeHours float64
	OpenIssues       int
	AsOf             time.Time
	Stale            bool
}

// Repo 数据窄口（data.githubPanelRepo 满足）。
type Repo interface {
	// UpsertWatch 幂等 watch：同 full_name 重 watch 更新 credential 绑定。
	UpsertWatch(ctx context.Context, repo WatchedRepo) (WatchedRepo, error)
	Unwatch(ctx context.Context, id string) error
	ListWatched(ctx context.Context) ([]WatchedRepo, error)
}

// CredentialRefChecker 凭据存在性核验窄口（G-2：watch 绑定的凭据必须
// 存在；不取明文——明文仅执行面经 RevealSecretForExecution 取用）。
type CredentialRefChecker interface {
	CredentialExists(ctx context.Context, id int64) (bool, error)
}

// AuditRecorder 审计窄口（data.NewMonitorAuditRepo 满足）。
type AuditRecorder interface {
	InsertAuditLog(ctx context.Context, entry monitor.AuditLog) error
}

// Usecase GitHub 面板用例。
type Usecase struct {
	repo  Repo
	cred  CredentialRefChecker
	audit AuditRecorder
	lg    loggateway.Logger
	// now 时钟注入（测试钉 stale 裁决）。
	now func() time.Time
}

// NewUsecase 装配用例。
func NewUsecase(repo Repo, cred CredentialRefChecker, lg loggateway.Logger) *Usecase {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &Usecase{repo: repo, cred: cred, lg: lg, now: time.Now}
}

// SetAuditRecorder 注入审计记录器（构造后置，wire 友好）。
func (u *Usecase) SetAuditRecorder(rec AuditRecorder) { u.audit = rec }

// SetClock 注入时钟（测试用）。
func (u *Usecase) SetClock(now func() time.Time) { u.now = now }

// Watch 登记观测仓库（github.manage）。幂等：同 full_name 重 watch
// 更新 credential 绑定，不清空健康快照。
func (u *Usecase) Watch(ctx context.Context, fullName, credentialIDStr string) (WatchedRepo, error) {
	if u.repo == nil {
		return WatchedRepo{}, errRepoStoreUnavailable
	}
	fullName = strings.TrimSpace(fullName)
	if !fullNameRe.MatchString(fullName) {
		return WatchedRepo{}, ErrFullNameInvalid
	}
	credID, err := strconv.ParseInt(strings.TrimSpace(credentialIDStr), 10, 64)
	if err != nil || credID <= 0 {
		return WatchedRepo{}, ErrCredentialInvalid
	}
	if u.cred != nil {
		ok, err := u.cred.CredentialExists(ctx, credID)
		if err != nil {
			return WatchedRepo{}, err
		}
		if !ok {
			return WatchedRepo{}, ErrCredentialNotFound
		}
	}
	repo := WatchedRepo{
		ID:           uuid.NewString(),
		FullName:     fullName,
		CredentialID: strconv.FormatInt(credID, 10),
		CIStatus:     "unknown",
	}
	saved, err := u.repo.UpsertWatch(ctx, repo)
	if err != nil {
		return WatchedRepo{}, err
	}
	u.recordAudit(ctx, AuditActionRepoWatch, saved.FullName, map[string]any{
		"credential_bound": credID > 0, // 不记凭据 id 值本身（最小留痕）
	})
	return saved, nil
}

// Unwatch 摘除观测仓库（github.manage）。
func (u *Usecase) Unwatch(ctx context.Context, id string) error {
	if u.repo == nil {
		return errRepoStoreUnavailable
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrRepoIDRequired
	}
	if err := u.repo.Unwatch(ctx, id); err != nil {
		return err
	}
	u.recordAudit(ctx, AuditActionRepoUnwatch, id, nil)
	return nil
}

// ListWatched 列出观测仓库（github.view）。
func (u *Usecase) ListWatched(ctx context.Context) ([]WatchedRepo, error) {
	if u.repo == nil {
		return nil, errRepoStoreUnavailable
	}
	return u.repo.ListWatched(ctx)
}

// Health 读仓库健康卡（github.view）：快照列 + stale 诚实裁决
//（health_updated_at 空或超 HealthStaleAfter → stale=true，数据照给）。
func (u *Usecase) Health(ctx context.Context) ([]RepoHealth, error) {
	repos, err := u.ListWatched(ctx)
	if err != nil {
		return nil, err
	}
	now := u.now()
	out := make([]RepoHealth, 0, len(repos))
	for _, r := range repos {
		stale := r.HealthUpdatedAt.IsZero() || now.Sub(r.HealthUpdatedAt) > HealthStaleAfter
		out = append(out, RepoHealth{
			FullName:         r.FullName,
			CIStatus:         r.CIStatus,
			OpenPRs:          r.OpenPRs,
			OldestPRAgeHours: r.OldestPRAgeHours,
			OpenIssues:       r.OpenIssues,
			AsOf:             r.HealthUpdatedAt,
			Stale:            stale,
		})
	}
	return out, nil
}

// recordAudit 管理写审计（best-effort：写失败只进日志不翻转决策，
// 与 embedding 闸同口径——W4 日志 (y)②）。
func (u *Usecase) recordAudit(ctx context.Context, action, resourceID string, detail map[string]any) {
	if u.audit == nil {
		return
	}
	payload, err := json.Marshal(detail)
	if err != nil {
		u.lg.Warn("githubpanel: marshal audit detail failed", loggateway.Err(err), loggateway.Str("action", action))
		return
	}
	entry := monitor.AuditLog{
		Action:     action,
		Resource:   "github",
		ResourceID: resourceID,
		Detail:     string(payload),
		Severity:   "info",
	}
	if p, ok := auth.FromContext(ctx); ok && p != nil {
		entry.Actor = strconv.FormatInt(p.UserID, 10)
	}
	if err := u.audit.InsertAuditLog(ctx, entry); err != nil {
		u.lg.Warn("githubpanel: audit insert failed", loggateway.Err(err), loggateway.Str("action", action), loggateway.Str("resource_id", resourceID))
	}
}
