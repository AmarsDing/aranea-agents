package biz

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── fakes ────────────────────────────────────────────────────────────

type fakeClassificationRepo struct {
	labels []ClassificationLabel
	puts   []ClassificationLabel
}

func (f *fakeClassificationRepo) ListLabels(ctx context.Context) ([]ClassificationLabel, error) {
	return f.labels, nil
}

func (f *fakeClassificationRepo) PutLabel(ctx context.Context, label ClassificationLabel) error {
	f.puts = append(f.puts, label)
	return nil
}

type fakeWaterlineRepo struct {
	ceiling    int
	raiseCalls []int
}

func (f *fakeWaterlineRepo) GetSessionCeiling(ctx context.Context, sessionID string) (int, error) {
	return f.ceiling, nil
}

func (f *fakeWaterlineRepo) RaiseSessionCeiling(ctx context.Context, sessionID string, level int) (int, error) {
	f.raiseCalls = append(f.raiseCalls, level)
	if level > f.ceiling {
		f.ceiling = level
	}
	return f.ceiling, nil
}

func (f *fakeWaterlineRepo) RaiseRunCeiling(ctx context.Context, runID string, level int) (int, error) {
	return f.RaiseSessionCeiling(ctx, runID, level)
}

type fakePolicyRepo struct {
	policy   *SecopsModelPolicy
	getCalls int
	setCalls []int
	setErr   error
}

func (f *fakePolicyRepo) GetPolicy(ctx context.Context) (*SecopsModelPolicy, error) {
	f.getCalls++
	return f.policy, nil
}

func (f *fakePolicyRepo) SetPolicy(ctx context.Context, threshold int, updatedBy string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.setCalls = append(f.setCalls, threshold)
	f.policy = &SecopsModelPolicy{LocalThreshold: threshold, UpdatedBy: updatedBy}
	return nil
}

type fakeSecopsAudit struct {
	entries []AuditLog
	err     error
}

func (f *fakeSecopsAudit) InsertAuditLog(ctx context.Context, entry AuditLog) error {
	if f.err != nil {
		return f.err
	}
	f.entries = append(f.entries, entry)
	return nil
}

func lastAuditAction(f *fakeSecopsAudit) string {
	if len(f.entries) == 0 {
		return ""
	}
	return f.entries[len(f.entries)-1].Action
}

// ── 密级标签 ─────────────────────────────────────────────────────────

func TestSecopsClassification_PutLabelValidation(t *testing.T) {
	repo := &fakeClassificationRepo{}
	audit := &fakeSecopsAudit{}
	uc := NewSecopsClassificationUsecase(repo, &fakeWaterlineRepo{}, loggateway.NewNoop())
	uc.SetSecopsClassificationAuditRecorder(audit)

	cases := []struct {
		name    string
		level   int
		wantErr apierror.Code
	}{
		{"level zero", 0, apierror.CodeBadRequest},
		{"level six", 6, apierror.CodeBadRequest},
		{"level negative", -1, apierror.CodeBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := uc.PutLabel(context.Background(), tc.level, "x", "", 7)
			if !apierror.IsCode(err, tc.wantErr) {
				t.Fatalf("want %s, got %v", tc.wantErr, err)
			}
		})
	}
	if len(repo.puts) != 0 {
		t.Fatalf("invalid levels must not reach repo: %+v", repo.puts)
	}
	if len(audit.entries) != 0 {
		t.Fatalf("invalid levels must not be audited: %+v", audit.entries)
	}

	// 空名拒。
	if _, err := uc.PutLabel(context.Background(), 3, "   ", "", 7); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("empty name: %v", err)
	}

	// 正路径：写 repo + 审计。
	out, err := uc.PutLabel(context.Background(), 3, "秘密", "强制本地", 7)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if out.Level != 3 || out.Name != "秘密" || out.UpdatedAt.IsZero() {
		t.Fatalf("out: %+v", out)
	}
	if len(repo.puts) != 1 || repo.puts[0].Name != "秘密" {
		t.Fatalf("repo puts: %+v", repo.puts)
	}
	if lastAuditAction(audit) != "secops.classification.label_put" {
		t.Fatalf("audit: %+v", audit.entries)
	}
}

func TestSecopsClassification_SetSessionCeilingRejectLower(t *testing.T) {
	waterline := &fakeWaterlineRepo{ceiling: 3}
	audit := &fakeSecopsAudit{}
	uc := NewSecopsClassificationUsecase(&fakeClassificationRepo{}, waterline, loggateway.NewNoop())
	uc.SetSecopsClassificationAuditRecorder(audit)

	// 降阈 3→2：拒 + 审计 warn（验收 §7.3 #2）。
	got, err := uc.SetSessionCeiling(context.Background(), "s1", 2, 9)
	if !apierror.IsCode(err, apierror.CodeFailedPrecondition) {
		t.Fatalf("lower must be FailedPrecondition: %v", err)
	}
	if got != 3 {
		t.Fatalf("reject returns current ceiling: %d", got)
	}
	if len(waterline.raiseCalls) != 0 {
		t.Fatalf("reject must not write: %+v", waterline.raiseCalls)
	}
	if len(audit.entries) != 1 || audit.entries[0].Action != "secops.classification.ceiling_rejected" || audit.entries[0].Severity != "warn" {
		t.Fatalf("reject audit: %+v", audit.entries)
	}

	// 升/等：放行走 GREATEST。
	if _, err := uc.SetSessionCeiling(context.Background(), "s1", 4, 9); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if waterline.ceiling != 4 || len(waterline.raiseCalls) != 1 || waterline.raiseCalls[0] != 4 {
		t.Fatalf("raise calls: %+v ceiling=%d", waterline.raiseCalls, waterline.ceiling)
	}
	// 降拒审计仅此一条（升不审计，防审计洪泛）。
	if len(audit.entries) != 1 {
		t.Fatalf("raise must not audit: %+v", audit.entries)
	}
}

func TestSecopsClassification_ParseCeilingLevel(t *testing.T) {
	cases := []struct {
		raw     string
		want    int
		wantErr bool
	}{
		{"", 0, false},
		{"0", 0, false},
		{"3", 3, false},
		{" 4 ", 4, false},
		{"5", 5, false},
		{"6", 0, true},
		{"-1", 0, true},
		{"abc", 0, true},
		{"2.5", 0, true},
	}
	for _, tc := range cases {
		got, err := ParseCeilingLevel(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%q must error", tc.raw)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("%q: got %d %v", tc.raw, got, err)
		}
	}
}

func TestSecopsClassification_CeilingForcesLocal(t *testing.T) {
	cases := []struct {
		ceiling, threshold int
		want               bool
	}{
		{0, 3, false}, // 未定级永不触发
		{2, 3, false},
		{3, 3, true}, // L3 = 强制本地
		{5, 3, true},
		{1, 1, true},
	}
	for _, tc := range cases {
		if got := CeilingForcesLocal(tc.ceiling, tc.threshold); got != tc.want {
			t.Fatalf("(%d,%d): got %v want %v", tc.ceiling, tc.threshold, got, tc.want)
		}
	}
}

// ── 模型策略 ─────────────────────────────────────────────────────────

func TestSecopsModelPolicy_DefaultAndHotCopy(t *testing.T) {
	repo := &fakePolicyRepo{} // 表空 → 默认 3
	uc := NewSecopsModelPolicyUsecase(repo, loggateway.NewNoop())

	p, err := uc.GetPolicy(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if p.LocalThreshold != SecopsDefaultLocalThreshold {
		t.Fatalf("default threshold: %+v", p)
	}
	// 热副本：5s TTL 内第二次 GetPolicy 不再打库。
	if _, err := uc.GetPolicy(context.Background()); err != nil {
		t.Fatalf("get2: %v", err)
	}
	if repo.getCalls != 1 {
		t.Fatalf("hot copy must hit repo once, got %d", repo.getCalls)
	}
}

func TestSecopsModelPolicy_SetValidation(t *testing.T) {
	repo := &fakePolicyRepo{}
	audit := &fakeSecopsAudit{}
	uc := NewSecopsModelPolicyUsecase(repo, loggateway.NewNoop())
	uc.SetSecopsModelPolicyAuditRecorder(audit)

	for _, threshold := range []int{0, 6, -3} {
		if _, err := uc.SetModelPolicy(context.Background(), threshold, 7); !apierror.IsCode(err, apierror.CodeBadRequest) {
			t.Fatalf("threshold %d: %v", threshold, err)
		}
	}
	if len(repo.setCalls) != 0 || len(audit.entries) != 0 {
		t.Fatalf("invalid set must not write/audit: %+v %+v", repo.setCalls, audit.entries)
	}
}

func TestSecopsModelPolicy_SetHappyPath(t *testing.T) {
	repo := &fakePolicyRepo{}
	audit := &fakeSecopsAudit{}
	uc := NewSecopsModelPolicyUsecase(repo, loggateway.NewNoop())
	uc.SetSecopsModelPolicyAuditRecorder(audit)

	out, err := uc.SetModelPolicy(context.Background(), 4, 7)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if out.LocalThreshold != 4 || out.UpdatedBy != "7" {
		t.Fatalf("out: %+v", out)
	}
	if len(repo.setCalls) != 1 || repo.setCalls[0] != 4 {
		t.Fatalf("repo sets: %+v", repo.setCalls)
	}
	// 审计：old=默认3 new=4。
	if len(audit.entries) != 1 || audit.entries[0].Action != "secops.classification.policy_update" {
		t.Fatalf("audit: %+v", audit.entries)
	}
	// 写后热副本失效：下一次 GetPolicy 重读库（见新值 4）。
	// getCalls == 2：Set 内预热一次 + 写后失效重读一次。
	p, err := uc.GetPolicy(context.Background())
	if err != nil || p.LocalThreshold != 4 {
		t.Fatalf("post-set get: %v %v", p, err)
	}
	if repo.getCalls != 2 {
		t.Fatalf("repo get calls: %d", repo.getCalls)
	}
}

func TestSecopsModelPolicy_SetRepoError(t *testing.T) {
	repo := &fakePolicyRepo{setErr: errors.New("db down")}
	audit := &fakeSecopsAudit{}
	uc := NewSecopsModelPolicyUsecase(repo, loggateway.NewNoop())
	uc.SetSecopsModelPolicyAuditRecorder(audit)

	if _, err := uc.SetModelPolicy(context.Background(), 4, 7); err == nil {
		t.Fatal("repo error must propagate")
	}
	if len(audit.entries) != 0 {
		t.Fatalf("failed set must not audit: %+v", audit.entries)
	}
}
