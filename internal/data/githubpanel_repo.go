package data

import (
	"context"
	"database/sql"

	"aranea-agents/internal/biz/githubpanel"
	"aranea-agents/pkg/apierror"
)

// ============================================================================
// GitHub 面板台账（E 阶段 W2c G，DDL 迁移 20261330）。查询走
// RWDB().ReadDB/WriteDB 返回的 execer（事务感知）；upsert 用
// ON CONFLICT ... DO UPDATE（PG/SQLite 双方言可用），与
// secops_classification_repo 同例。
// ============================================================================

type githubPanelRepo struct {
	data *Data
}

var _ githubpanel.Repo = (*githubPanelRepo)(nil)

// NewGitHubPanelRepo 装配 GitHub 面板台账。
func NewGitHubPanelRepo(d *Data) githubpanel.Repo {
	if d == nil {
		return nil
	}
	return &githubPanelRepo{data: d}
}

const githubRepoColumns = `id, full_name, credential_id, webhook_ok, last_event_at,
	ci_status, open_prs, oldest_pr_age_hours, open_issues, health_updated_at, created_at, updated_at`

// UpsertWatch 幂等 watch：同 full_name 重 watch 更新 credential 绑定与
// updated_at，健康快照列不动（不清空历史采集）。
func (r *githubPanelRepo) UpsertWatch(ctx context.Context, repo githubpanel.WatchedRepo) (githubpanel.WatchedRepo, error) {
	if r.data.RWDB().WriteDB(ctx) == nil {
		return githubpanel.WatchedRepo{}, apierror.Internal("GITHUB", "repository unavailable")
	}
	now := nowRFC3339()
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`INSERT INTO github_repos
			(id, full_name, credential_id, webhook_ok, ci_status, created_at, updated_at)
			VALUES (?, ?, ?, FALSE, ?, ?, ?)
			ON CONFLICT (full_name) DO UPDATE SET credential_id = EXCLUDED.credential_id, updated_at = EXCLUDED.updated_at`),
		repo.ID, repo.FullName, repo.CredentialID, repo.CIStatus, now, now)
	if err != nil {
		return githubpanel.WatchedRepo{}, entErrToBizErr(err, "GITHUB")
	}
	// 写后读回（upsert 命中旧行时拿旧 id/快照列）。
	return r.getByFullName(ctx, repo.FullName)
}

func (r *githubPanelRepo) getByFullName(ctx context.Context, fullName string) (githubpanel.WatchedRepo, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT `+githubRepoColumns+` FROM github_repos WHERE full_name = ? LIMIT 1`), fullName)
	if err != nil {
		return githubpanel.WatchedRepo{}, entErrToBizErr(err, "GITHUB")
	}
	defer rows.Close()
	if !rows.Next() {
		return githubpanel.WatchedRepo{}, githubpanel.ErrRepoNotFound
	}
	repo, err := scanGitHubRepo(rows)
	if err != nil {
		return githubpanel.WatchedRepo{}, err
	}
	return repo, entErrToBizErr(rows.Err(), "GITHUB")
}

// Unwatch 摘除观测仓库；无行 → ErrRepoNotFound。
func (r *githubPanelRepo) Unwatch(ctx context.Context, id string) error {
	if r.data.RWDB().WriteDB(ctx) == nil {
		return apierror.Internal("GITHUB", "repository unavailable")
	}
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`DELETE FROM github_repos WHERE id = ?`), id)
	if err != nil {
		return entErrToBizErr(err, "GITHUB")
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return entErrToBizErr(err, "GITHUB")
	}
	if affected == 0 {
		return githubpanel.ErrRepoNotFound
	}
	return nil
}

// ListWatched 全量列出（watch 清单规模为管理员级小表，无分页）。
func (r *githubPanelRepo) ListWatched(ctx context.Context) ([]githubpanel.WatchedRepo, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT `+githubRepoColumns+` FROM github_repos ORDER BY created_at ASC`)
	if err != nil {
		return nil, entErrToBizErr(err, "GITHUB")
	}
	defer rows.Close()
	out := make([]githubpanel.WatchedRepo, 0)
	for rows.Next() {
		repo, err := scanGitHubRepo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, repo)
	}
	return out, entErrToBizErr(rows.Err(), "GITHUB")
}

// scanGitHubRepo 行扫描（时间列 TIMESTAMPTZ 经字符串桥接，与
// secops_classification_repo 同例；可空列走 NullString）。
func scanGitHubRepo(rows *sql.Rows) (githubpanel.WatchedRepo, error) {
	var repo githubpanel.WatchedRepo
	var lastEventAt, healthUpdatedAt sql.NullString
	var createdAt, updatedAt string
	if err := rows.Scan(&repo.ID, &repo.FullName, &repo.CredentialID, &repo.WebhookOK, &lastEventAt,
		&repo.CIStatus, &repo.OpenPRs, &repo.OldestPRAgeHours, &repo.OpenIssues, &healthUpdatedAt,
		&createdAt, &updatedAt); err != nil {
		return githubpanel.WatchedRepo{}, entErrToBizErr(err, "GITHUB")
	}
	repo.LastEventAt = parseRFC3339Time(lastEventAt.String)
	repo.HealthUpdatedAt = parseRFC3339Time(healthUpdatedAt.String)
	repo.CreatedAt = parseRFC3339Time(createdAt)
	repo.UpdatedAt = parseRFC3339Time(updatedAt)
	return repo, nil
}
