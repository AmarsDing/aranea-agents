package githubpanel

// V1 续测试（E 阶段 W2c G）：watch 三件套校验矩阵 + 审计留痕口径 +
// Health stale 读侧裁决。正路径落库由 internal/data 的 55433 roundtrip
// 测试覆盖（githubpanel_repo_test.go）。

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// fakeRepo 内存版台账，镜像 UpsertWatch 幂等语义：同 full_name 重 watch
// 保留原 id/created/健康快照列，只更新 credential 绑定。
type fakeRepo struct {
	rows    map[string]WatchedRepo // by full_name
	deleted []string
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[string]WatchedRepo{}} }

func (r *fakeRepo) UpsertWatch(_ context.Context, repo WatchedRepo) (WatchedRepo, error) {
	if old, ok := r.rows[repo.FullName]; ok {
		old.CredentialID = repo.CredentialID
		r.rows[repo.FullName] = old
		return old, nil
	}
	r.rows[repo.FullName] = repo
	return repo, nil
}

func (r *fakeRepo) Unwatch(_ context.Context, id string) error {
	for k, v := range r.rows {
		if v.ID == id {
			delete(r.rows, k)
			r.deleted = append(r.deleted, id)
			return nil
		}
	}
	return ErrRepoNotFound
}

func (r *fakeRepo) ListWatched(context.Context) ([]WatchedRepo, error) {
	out := make([]WatchedRepo, 0, len(r.rows))
	for _, v := range r.rows {
		out = append(out, v)
	}
	return out, nil
}

type fakeCredChecker struct {
	existing map[int64]bool
	err      error
}

func (c *fakeCredChecker) CredentialExists(_ context.Context, id int64) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	return c.existing[id], nil
}

type auditSink struct {
	entries []monitor.AuditLog
	err     error
}

func (s *auditSink) InsertAuditLog(_ context.Context, e monitor.AuditLog) error {
	if s.err != nil {
		return s.err
	}
	s.entries = append(s.entries, e)
	return nil
}

func newWatchUsecase(repo Repo, cred CredentialRefChecker, sink *auditSink) *Usecase {
	uc := NewUsecase(repo, cred, loggateway.NewNoop())
	if sink != nil {
		uc.SetAuditRecorder(sink)
	}
	return uc
}

// TestUsecase_WatchValidationMatrix 钉死输入校验矩阵：full_name 必须
// owner/repo 形态，credential_id 必须正整数字符串（先于存在性核验）。
func TestUsecase_WatchValidationMatrix(t *testing.T) {
	cred := &fakeCredChecker{existing: map[int64]bool{1: true, 2: true}}
	cases := []struct {
		name     string
		fullName string
		credID   string
	}{
		{"empty full_name", "", "1"},
		{"no slash", "noslash", "1"},
		{"two slashes", "a/b/c", "1"},
		{"space inside", "a b/c", "1"},
		{"leading slash", "/repo", "1"},
		{"trailing slash", "owner/", "1"},
		{"empty credential", "owner/repo", ""},
		{"alpha credential", "owner/repo", "abc"},
		{"zero credential", "owner/repo", "0"},
		{"negative credential", "owner/repo", "-7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uc := newWatchUsecase(newFakeRepo(), cred, nil)
			if _, err := uc.Watch(context.Background(), tc.fullName, tc.credID); !apierror.IsCode(err, apierror.CodeBadRequest) {
				t.Fatalf("want BadRequest, got %v", err)
			}
		})
	}
}

// TestUsecase_WatchCredentialChecks 钉死 G-2 凭据核验口径：绑定凭据必须
// 存在；checker 故障错误透传（不放行）；未装配 repo → Internal。
func TestUsecase_WatchCredentialChecks(t *testing.T) {
	uc := newWatchUsecase(newFakeRepo(), &fakeCredChecker{existing: map[int64]bool{}}, nil)
	if _, err := uc.Watch(context.Background(), "owner/repo", "9"); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("missing credential must be BadRequest, got %v", err)
	}

	sentinel := errors.New("credential store down")
	uc = newWatchUsecase(newFakeRepo(), &fakeCredChecker{err: sentinel}, nil)
	if _, err := uc.Watch(context.Background(), "owner/repo", "1"); !errors.Is(err, sentinel) {
		t.Fatalf("checker error must propagate, got %v", err)
	}

	uc = NewUsecase(nil, nil, loggateway.NewNoop())
	if _, err := uc.Watch(context.Background(), "owner/repo", "1"); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("unwired repo must be Internal, got %v", err)
	}
	if _, err := uc.ListWatched(context.Background()); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("unwired list must be Internal, got %v", err)
	}
	if _, err := uc.Health(context.Background()); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("unwired health must be Internal, got %v", err)
	}
}

// TestUsecase_WatchSuccessAudit 钉死正路径：输入两侧空白容忍、初始健康
// 快照默认、幂等重 watch 保留原 id、审计最小留痕（credential_bound 布尔，
// 不含凭据 id 值——明文/敏感值三不进）。
func TestUsecase_WatchSuccessAudit(t *testing.T) {
	repo := newFakeRepo()
	sink := &auditSink{}
	uc := newWatchUsecase(repo, &fakeCredChecker{existing: map[int64]bool{42: true}}, sink)
	ctx := auth.NewContext(context.Background(), &auth.Auth{UserID: 71})

	saved, err := uc.Watch(ctx, " org/repo ", " 42 ")
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	if saved.ID == "" || saved.FullName != "org/repo" || saved.CredentialID != "42" {
		t.Fatalf("saved mismatch: %+v", saved)
	}
	if saved.CIStatus != "unknown" || saved.WebhookOK {
		t.Fatalf("initial snapshot defaults wrong: %+v", saved)
	}

	re, err := uc.Watch(ctx, "org/repo", "42")
	if err != nil {
		t.Fatalf("re-watch: %v", err)
	}
	if re.ID != saved.ID {
		t.Fatalf("re-watch must keep original id: %s vs %s", re.ID, saved.ID)
	}

	if len(sink.entries) != 2 {
		t.Fatalf("want 2 audit entries, got %d", len(sink.entries))
	}
	entry := sink.entries[0]
	if entry.Action != AuditActionRepoWatch || entry.Resource != "github" || entry.ResourceID != "org/repo" {
		t.Fatalf("audit entry = %+v", entry)
	}
	if entry.Actor != "71" || entry.Severity != "info" {
		t.Fatalf("audit actor/severity = %+v", entry)
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(entry.Detail), &detail); err != nil {
		t.Fatalf("detail json: %v", err)
	}
	if detail["credential_bound"] != true {
		t.Fatalf("detail credential_bound missing: %s", entry.Detail)
	}
	if strings.Contains(entry.Detail, "42") {
		t.Fatalf("detail must not carry credential id value: %s", entry.Detail)
	}
}

// TestUsecase_WatchAuditBestEffort 审计写失败不翻转决策（与 embedding 闸
// 同口径，W4 日志 (y)②）。
func TestUsecase_WatchAuditBestEffort(t *testing.T) {
	sink := &auditSink{err: errors.New("audit store down")}
	uc := newWatchUsecase(newFakeRepo(), &fakeCredChecker{existing: map[int64]bool{1: true}}, sink)
	if _, err := uc.Watch(context.Background(), "owner/repo", "1"); err != nil {
		t.Fatalf("audit failure must not fail watch, got %v", err)
	}
}

// TestUsecase_Unwatch 钉死摘除口径：空 id → BadRequest；未登记 id →
// NotFound；正路径删除 + 审计。
func TestUsecase_Unwatch(t *testing.T) {
	repo := newFakeRepo()
	sink := &auditSink{}
	uc := newWatchUsecase(repo, &fakeCredChecker{existing: map[int64]bool{1: true}}, sink)

	if err := uc.Unwatch(context.Background(), "  "); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("empty id must be BadRequest, got %v", err)
	}
	if err := uc.Unwatch(context.Background(), "missing"); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("missing id must be NotFound, got %v", err)
	}

	saved, err := uc.Watch(context.Background(), "owner/repo", "1")
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	if err := uc.Unwatch(context.Background(), saved.ID); err != nil {
		t.Fatalf("unwatch: %v", err)
	}
	if len(repo.deleted) != 1 || repo.deleted[0] != saved.ID {
		t.Fatalf("repo delete not called: %v", repo.deleted)
	}
	last := sink.entries[len(sink.entries)-1]
	if last.Action != AuditActionRepoUnwatch || last.ResourceID != saved.ID {
		t.Fatalf("unwatch audit = %+v", last)
	}
}

// TestUsecase_HealthStaleMatrix 钉死 stale 读侧裁决：health_updated_at 零值
// 或超 HealthStaleAfter（严格 >）→ stale=true；数据照给不伪造清空。
func TestUsecase_HealthStaleMatrix(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	repo.rows["a/never"] = WatchedRepo{ID: "1", FullName: "a/never", CIStatus: "unknown"}
	repo.rows["a/fresh"] = WatchedRepo{ID: "2", FullName: "a/fresh", CIStatus: "green", OpenPRs: 3, HealthUpdatedAt: now.Add(-10 * time.Minute)}
	repo.rows["a/edge"] = WatchedRepo{ID: "3", FullName: "a/edge", CIStatus: "green", HealthUpdatedAt: now.Add(-HealthStaleAfter)}
	repo.rows["a/stale"] = WatchedRepo{ID: "4", FullName: "a/stale", CIStatus: "red", HealthUpdatedAt: now.Add(-HealthStaleAfter - time.Second)}

	uc := NewUsecase(repo, nil, loggateway.NewNoop())
	uc.SetClock(func() time.Time { return now })
	health, err := uc.Health(context.Background())
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if len(health) != 4 {
		t.Fatalf("want 4 health rows, got %d", len(health))
	}
	byName := map[string]RepoHealth{}
	for _, h := range health {
		byName[h.FullName] = h
	}
	if !byName["a/never"].Stale {
		t.Fatal("never-collected must be stale")
	}
	if byName["a/fresh"].Stale {
		t.Fatal("10min old must be fresh")
	}
	if byName["a/edge"].Stale {
		t.Fatal("exactly at window edge must be fresh (strict >)")
	}
	if !byName["a/stale"].Stale {
		t.Fatal("beyond window must be stale")
	}
	if byName["a/stale"].CIStatus != "red" || byName["a/fresh"].OpenPRs != 3 {
		t.Fatalf("snapshot fields must pass through: %+v", byName)
	}
	if !byName["a/never"].AsOf.IsZero() {
		t.Fatal("never-collected AsOf must be zero")
	}
}
