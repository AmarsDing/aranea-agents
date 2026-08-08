package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/self_improvement/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// ── fake admin port ──────────────────────────────────────────────────────────

type siAdminPortFake struct {
	listRuns  []biz.SelfImprovementRun
	listTotal int
	listErr   error
	gotFilter biz.RunFilter

	getRun *biz.SelfImprovementRun
	getErr error

	actionID, actionOperator, actionReason string
	actionErr                              error

	stats    *biz.SIOutcomeStats
	statsErr error

	riskRules biz.SIRiskRules
	riskErr   error
	gotRules  biz.SIRiskRules
}

func (f *siAdminPortFake) List(_ context.Context, filter biz.RunFilter) ([]biz.SelfImprovementRun, int, error) {
	f.gotFilter = filter
	return f.listRuns, f.listTotal, f.listErr
}

func (f *siAdminPortFake) Get(_ context.Context, _ string) (*biz.SelfImprovementRun, error) {
	return f.getRun, f.getErr
}

func (f *siAdminPortFake) Approve(_ context.Context, id, operator, reason string) error {
	f.actionID, f.actionOperator, f.actionReason = id, operator, reason
	return f.actionErr
}

func (f *siAdminPortFake) Reject(_ context.Context, id, operator, reason string) error {
	f.actionID, f.actionOperator, f.actionReason = id, operator, reason
	return f.actionErr
}

func (f *siAdminPortFake) Rollback(_ context.Context, id, operator, reason string) error {
	f.actionID, f.actionOperator, f.actionReason = id, operator, reason
	return f.actionErr
}

func (f *siAdminPortFake) Close(_ context.Context, id, operator string) error {
	f.actionID, f.actionOperator = id, operator
	return f.actionErr
}

func (f *siAdminPortFake) OutcomeStats(context.Context) (*biz.SIOutcomeStats, error) {
	return f.stats, f.statsErr
}

func (f *siAdminPortFake) GetRiskRules(context.Context) (biz.SIRiskRules, error) {
	return f.riskRules, f.riskErr
}

func (f *siAdminPortFake) UpdateRiskRules(_ context.Context, operator string, rules biz.SIRiskRules) (biz.SIRiskRules, error) {
	f.actionOperator = operator
	f.gotRules = rules
	if f.riskErr != nil {
		return biz.SIRiskRules{}, f.riskErr
	}
	return rules, nil
}

// ── fixtures ─────────────────────────────────────────────────────────────────

func siSvcAdminCtx() context.Context {
	return auth.NewContext(context.Background(), &auth.Auth{UserID: 7, Access: "admin"})
}

func siSvcUserCtx() context.Context {
	return auth.NewContext(context.Background(), &auth.Auth{UserID: 7, Access: "user"})
}

func siSvcNew(uc siAdminPort) *SelfImprovementService {
	return &SelfImprovementService{uc: uc, lg: loggateway.NewNoop()}
}

// ── auth guards ──────────────────────────────────────────────────────────────

func TestSelfImprovementService_Unauthenticated(t *testing.T) {
	svc := siSvcNew(&siAdminPortFake{})
	if _, err := svc.ListRuns(context.Background(), &v1.ListRunsRequest{}); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("ListRuns err = %v, want ErrUnauthorized", err)
	}
	if _, err := svc.ApproveRun(context.Background(), &v1.ApproveRunRequest{Id: "r1"}); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("ApproveRun err = %v, want ErrUnauthorized", err)
	}
}

func TestSelfImprovementService_ForbiddenForNonAdmin(t *testing.T) {
	svc := siSvcNew(&siAdminPortFake{})
	if _, err := svc.ListRuns(siSvcUserCtx(), &v1.ListRunsRequest{}); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("ListRuns err = %v, want ErrForbidden", err)
	}
	if _, err := svc.RollbackRun(siSvcUserCtx(), &v1.RollbackRunRequest{Id: "r1"}); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("RollbackRun err = %v, want ErrForbidden", err)
	}
}

// usecase 为 nil（self_improvement.enabled=false 未接线）→ Unavailable。
func TestSelfImprovementService_NilUsecase(t *testing.T) {
	svc := siSvcNew(nil)
	if _, err := svc.ListRuns(siSvcAdminCtx(), &v1.ListRunsRequest{}); !apierror.IsCode(err, apierror.CodeUnavailable) {
		t.Fatalf("err = %v, want Unavailable", err)
	}
}

// 构造函数 typed-nil 防护回归（P5.5）：wire 注入的是 nil 具体指针
// *biz.SelfImprovementAdminUsecase，直接存入接口字段会形成 typed-nil
// （uc != nil 但接收者为 nil → 调用即 panic）。构造函数必须显式转换。
func TestNewSelfImprovementService_TypedNilUsecase(t *testing.T) {
	var uc *biz.SelfImprovementAdminUsecase // nil 具体指针（wire disabled 路径的实际注入值）
	svc := NewSelfImprovementService(uc, &conf.SelfImprovement{}, nil, nil, nil)
	if svc.uc != nil {
		t.Fatalf("uc = %T, want nil interface（typed-nil 会绕过 requireAdmin 守卫）", svc.uc)
	}
	if _, err := svc.ListRuns(siSvcAdminCtx(), &v1.ListRunsRequest{}); !apierror.IsCode(err, apierror.CodeUnavailable) {
		t.Fatalf("err = %v, want Unavailable（typed-nil 应触发 disabled 守卫而非 panic）", err)
	}
}

// ── GetStatus（P5.5 前置自检：disabled 也可用，不依赖 usecase）──────────────

type siRefineLLMFake struct {
	setting biz.RefineLLMSetting
	err     error
}

func (f *siRefineLLMFake) GetRefineLLM(context.Context) (biz.RefineLLMSetting, error) {
	return f.setting, f.err
}

func TestSelfImprovementService_GetStatus_RequiresAdmin(t *testing.T) {
	svc := siSvcNew(nil)
	if _, err := svc.GetStatus(context.Background(), &v1.GetStatusRequest{}); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("GetStatus err = %v, want ErrUnauthorized", err)
	}
	if _, err := svc.GetStatus(siSvcUserCtx(), &v1.GetStatusRequest{}); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("GetStatus err = %v, want ErrForbidden", err)
	}
}

// disabled + 未配置 LLM + repo_root 回退进程 cwd（包目录无 .git → invalid）。
func TestSelfImprovementService_GetStatus_Disabled(t *testing.T) {
	svc := siSvcNew(nil)
	svc.cfg = &conf.SelfImprovement{}
	svc.refineLLM = &siRefineLLMFake{}
	st, err := svc.GetStatus(siSvcAdminCtx(), &v1.GetStatusRequest{})
	if err != nil {
		t.Fatalf("GetStatus (disabled) err = %v, want nil（disabled 也必须可用）", err)
	}
	if st.GetEnabled() {
		t.Fatalf("Enabled = true, want false")
	}
	if st.GetRefineLlmConfigured() {
		t.Fatalf("RefineLlmConfigured = true, want false（未配置 provider/model）")
	}
	if st.GetRepoRoot() == "" {
		t.Fatalf("RepoRoot empty, want 进程 cwd 回退值")
	}
	if st.GetRepoRootValid() {
		t.Fatalf("RepoRootValid = true, want false（包目录无 .git）")
	}
}

// enabled + 已配置 LLM + 显式合法 repo_root（含 .git）。
func TestSelfImprovementService_GetStatus_Enabled(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	svc := siSvcNew(&siAdminPortFake{})
	svc.cfg = &conf.SelfImprovement{
		Enabled: true,
		Sandbox: &conf.SelfImprovement_Sandbox{RepoRoot: dir},
	}
	svc.refineLLM = &siRefineLLMFake{setting: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-x"}}
	st, err := svc.GetStatus(siSvcAdminCtx(), &v1.GetStatusRequest{})
	if err != nil {
		t.Fatalf("GetStatus err = %v", err)
	}
	if !st.GetEnabled() {
		t.Fatalf("Enabled = false, want true")
	}
	if !st.GetRefineLlmConfigured() {
		t.Fatalf("RefineLlmConfigured = false, want true")
	}
	if st.GetRefineLlmProvider() != "openai" || st.GetRefineLlmModel() != "gpt-x" {
		t.Fatalf("provider/model = %q/%q, want openai/gpt-x", st.GetRefineLlmProvider(), st.GetRefineLlmModel())
	}
	if st.GetRepoRoot() != dir || !st.GetRepoRootValid() {
		t.Fatalf("repo root = %q valid=%v, want %q valid=true", st.GetRepoRoot(), st.GetRepoRootValid(), dir)
	}
}

// refine LLM 读取失败 → 降级为未配置（不报错）。
func TestSelfImprovementService_GetStatus_RefineLLMDegraded(t *testing.T) {
	svc := siSvcNew(nil)
	svc.refineLLM = &siRefineLLMFake{err: errors.New("store down")}
	st, err := svc.GetStatus(siSvcAdminCtx(), &v1.GetStatusRequest{})
	if err != nil {
		t.Fatalf("GetStatus err = %v, want nil（LLM 读取失败须降级）", err)
	}
	if st.GetRefineLlmConfigured() {
		t.Fatalf("RefineLlmConfigured = true, want false（读取失败降级）")
	}
}

// ── ListRuns ─────────────────────────────────────────────────────────────────

func TestSelfImprovementService_ListRuns(t *testing.T) {
	fake := &siAdminPortFake{
		listRuns: []biz.SelfImprovementRun{{
			ID: "run-1", SuggestionID: "sug-1", Status: biz.RunStatusObserving,
			TriggerSource: biz.TriggerSourceErrorCluster, PatchKind: biz.PatchKindCode,
			RiskLevel: biz.RiskLevelHigh, DiffStats: biz.DiffStats{Files: 2, Additions: 10, Deletions: 3},
			Attempts: 1, CreatedAt: time.Now(), UpdatedAt: time.Now(),
			Diff: "SHOULD-NOT-LEAK", // 列表不返重字段
		}},
		listTotal: 42,
	}
	svc := siSvcNew(fake)
	resp, err := svc.ListRuns(siSvcAdminCtx(), &v1.ListRunsRequest{
		Status: "observing", RiskLevel: "high", TriggerSource: "error_cluster",
		Page: 2, PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if fake.gotFilter.Status != biz.RunStatusObserving ||
		fake.gotFilter.RiskLevel != biz.RiskLevelHigh ||
		fake.gotFilter.TriggerSource != biz.TriggerSourceErrorCluster {
		t.Fatalf("filter = %+v", fake.gotFilter)
	}
	if fake.gotFilter.Limit != 10 || fake.gotFilter.Offset != 10 {
		t.Fatalf("page 2/size 10 → limit/offset = %d/%d, want 10/10", fake.gotFilter.Limit, fake.gotFilter.Offset)
	}
	if resp.Total != 42 || len(resp.Items) != 1 {
		t.Fatalf("resp total=%d items=%d", resp.Total, len(resp.Items))
	}
	item := resp.Items[0]
	if item.Id != "run-1" || item.RiskLevel != "high" || item.DiffStats.Additions != 10 {
		t.Fatalf("item = %+v", item)
	}
	if item.Diff != "" || item.Diagnosis != nil || item.Governance != nil {
		t.Fatal("列表项不应携带重字段（diff/diagnosis/governance）")
	}
}

// ── GetRun ───────────────────────────────────────────────────────────────────

func TestSelfImprovementService_GetRun(t *testing.T) {
	observeUntil := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	fake := &siAdminPortFake{getRun: &biz.SelfImprovementRun{
		ID: "run-1", SuggestionID: "sug-1", Status: biz.RunStatusObserving,
		TriggerSource: biz.TriggerSourceEvalRegression, PatchKind: biz.PatchKindConfig,
		RiskLevel: biz.RiskLevelMedium, Diff: "diff --git a/x b/x",
		Diagnosis:          &biz.Diagnosis{RootCause: "cache miss", AffectedFiles: []string{"a.go"}, ImpactScope: "module", FixStrategy: "warmup", Confidence: 0.8},
		VerificationReport: []biz.SandboxGateResult{{Gate: biz.SandboxGateBuild, Passed: true, Output: "ok", DurationMS: 1234}},
		CriticReport:       &biz.CriticReport{IsSafe: true, RiskLevel: "low", Concerns: []string{"none"}, Suggestion: "ship"},
		Governance:         &biz.GovernanceDecision{RiskLevel: biz.RiskLevelMedium, Channel: "notify", RuleHits: []string{"R2"}},
		ObserveUntil:       &observeUntil,
		CreatedAt:          time.Now(), UpdatedAt: time.Now(),
	}}
	svc := siSvcNew(fake)
	resp, err := svc.GetRun(siSvcAdminCtx(), &v1.GetRunRequest{Id: "run-1"})
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	run := resp.Run
	if run.Diff == "" || run.Diagnosis.RootCause != "cache miss" || run.Diagnosis.Confidence != 0.8 {
		t.Fatalf("detail fields = %+v", run)
	}
	if len(run.VerificationReport) != 1 || run.VerificationReport[0].Gate != "g1_build" || run.VerificationReport[0].DurationMs != 1234 {
		t.Fatalf("verification report = %+v", run.VerificationReport)
	}
	if run.CriticReport.Suggestion != "ship" || run.Governance.Channel != "notify" || run.Governance.RuleHits[0] != "R2" {
		t.Fatalf("critic/governance = %+v / %+v", run.CriticReport, run.Governance)
	}
	if run.ObserveUntil == nil {
		t.Fatal("observe_until 应映射")
	}
}

func TestSelfImprovementService_GetRunNotFound(t *testing.T) {
	svc := siSvcNew(&siAdminPortFake{getErr: apierror.NotFound("SELF_IMPROVEMENT", "run nope not found")})
	if _, err := svc.GetRun(siSvcAdminCtx(), &v1.GetRunRequest{Id: "nope"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("err = %v, want NotFound", err)
	}
}

// ── mutations：operator 取认证身份，reason 透传 ──────────────────────────────

func TestSelfImprovementService_ApproveRun(t *testing.T) {
	fake := &siAdminPortFake{}
	svc := siSvcNew(fake)
	if _, err := svc.ApproveRun(siSvcAdminCtx(), &v1.ApproveRunRequest{Id: "run-1", Reason: "looks safe"}); err != nil {
		t.Fatalf("ApproveRun: %v", err)
	}
	if fake.actionID != "run-1" || fake.actionOperator != "7" || fake.actionReason != "looks safe" {
		t.Fatalf("action = %q/%q/%q", fake.actionID, fake.actionOperator, fake.actionReason)
	}
}

func TestSelfImprovementService_RejectRunReasonRequired(t *testing.T) {
	fake := &siAdminPortFake{actionErr: apierror.BadRequest("SELF_IMPROVEMENT", "reject reason is required")}
	svc := siSvcNew(fake)
	if _, err := svc.RejectRun(siSvcAdminCtx(), &v1.RejectRunRequest{Id: "run-1"}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("err = %v, want BadRequest", err)
	}
}

func TestSelfImprovementService_ConflictPropagates(t *testing.T) {
	fake := &siAdminPortFake{actionErr: apierror.Conflict("SELF_IMPROVEMENT", "run run-1 not awaiting_governance")}
	svc := siSvcNew(fake)
	if _, err := svc.ApproveRun(siSvcAdminCtx(), &v1.ApproveRunRequest{Id: "run-1"}); !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("err = %v, want Conflict", err)
	}
}

// ── GetOutcomeStats ──────────────────────────────────────────────────────────

func TestSelfImprovementService_GetOutcomeStats(t *testing.T) {
	fake := &siAdminPortFake{stats: &biz.SIOutcomeStats{
		Total: 6, Effective: 3, Neutral: 2, Regressed: 1,
		EffectiveRate: 0.5, RollbackRate: 1.0 / 6.0,
		ByTrigger: []biz.SITriggerOutcomeStats{
			{TriggerSource: biz.TriggerSourceErrorCluster, Total: 4, Effective: 3, Regressed: 1},
			{TriggerSource: biz.TriggerSourceTestFailure, Total: 2, Neutral: 2},
		},
	}}
	svc := siSvcNew(fake)
	resp, err := svc.GetOutcomeStats(siSvcAdminCtx(), &v1.GetOutcomeStatsRequest{})
	if err != nil {
		t.Fatalf("GetOutcomeStats: %v", err)
	}
	if resp.Total != 6 || resp.Effective != 3 || resp.EffectiveRate != 0.5 {
		t.Fatalf("resp = %+v", resp)
	}
	if len(resp.ByTrigger) != 2 || resp.ByTrigger[0].TriggerSource != "error_cluster" || resp.ByTrigger[0].Total != 4 {
		t.Fatalf("by_trigger = %+v", resp.ByTrigger)
	}
}

// ── Risk rules ───────────────────────────────────────────────────────────────

// GetRiskRules：configured 透传原始值，effective 为归一化后的消费值。
func TestSelfImprovementService_GetRiskRules(t *testing.T) {
	fake := &siAdminPortFake{riskRules: biz.SIRiskRules{
		LowMaxLines:    50, // 覆盖默认
		MediumMaxLines: 0,  // 0 → 继承默认 300
		CorePathGlobs:  []string{"internal/service/**"},
	}}
	svc := siSvcNew(fake)
	resp, err := svc.GetRiskRules(siSvcAdminCtx(), &v1.GetRiskRulesRequest{})
	if err != nil {
		t.Fatalf("GetRiskRules: %v", err)
	}
	if resp.Configured.LowMaxLines != 50 || resp.Configured.MediumMaxLines != 0 {
		t.Fatalf("configured = %+v, want raw 50/0", resp.Configured)
	}
	if len(resp.Configured.CorePathGlobs) != 1 || resp.Configured.CorePathGlobs[0] != "internal/service/**" {
		t.Fatalf("configured globs = %v", resp.Configured.CorePathGlobs)
	}
	want := biz.DefaultSIRiskRules()
	if resp.Effective.LowMaxLines != 50 {
		t.Fatalf("effective low = %d, want 50 (configured)", resp.Effective.LowMaxLines)
	}
	if resp.Effective.MediumMaxLines != int32(want.MediumMaxLines) {
		t.Fatalf("effective medium = %d, want default %d", resp.Effective.MediumMaxLines, want.MediumMaxLines)
	}
	if resp.Effective.DailyAutoQuota != want.DailyAutoQuota {
		t.Fatalf("effective quota = %d, want default %d", resp.Effective.DailyAutoQuota, want.DailyAutoQuota)
	}
}

// UpdateRiskRules：请求字段映射到 biz 模型，operator 取认证身份，响应双视图。
func TestSelfImprovementService_UpdateRiskRules(t *testing.T) {
	fake := &siAdminPortFake{}
	svc := siSvcNew(fake)
	resp, err := svc.UpdateRiskRules(siSvcAdminCtx(), &v1.UpdateRiskRulesRequest{
		LowMaxLines:    80,
		MediumMaxLines: 200,
		CorePathGlobs:  []string{"cmd/**"},
		DailyAutoQuota: 3,
	})
	if err != nil {
		t.Fatalf("UpdateRiskRules: %v", err)
	}
	if fake.actionOperator != "7" {
		t.Fatalf("operator = %q, want \"7\" (auth user id)", fake.actionOperator)
	}
	if fake.gotRules.LowMaxLines != 80 || fake.gotRules.MediumMaxLines != 200 ||
		fake.gotRules.DailyAutoQuota != 3 || len(fake.gotRules.CorePathGlobs) != 1 {
		t.Fatalf("biz rules = %+v", fake.gotRules)
	}
	if resp.Configured.LowMaxLines != 80 || resp.Effective.LowMaxLines != 80 ||
		resp.Effective.MediumMaxLines != 200 || resp.Effective.DailyAutoQuota != 3 {
		t.Fatalf("resp = %+v / %+v", resp.Configured, resp.Effective)
	}
}

// UpdateRiskRules：biz 校验失败（BadRequest）透传。
func TestSelfImprovementService_UpdateRiskRulesValidationPropagates(t *testing.T) {
	fake := &siAdminPortFake{riskErr: apierror.BadRequest("SELF_IMPROVEMENT", "low_max_lines (300) must be <= medium_max_lines (100)")}
	svc := siSvcNew(fake)
	if _, err := svc.UpdateRiskRules(siSvcAdminCtx(), &v1.UpdateRiskRulesRequest{LowMaxLines: 300, MediumMaxLines: 100}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("err = %v, want BadRequest", err)
	}
}

// 风险规则端点同样走 admin 鉴权。
func TestSelfImprovementService_RiskRulesAuthGuard(t *testing.T) {
	svc := siSvcNew(&siAdminPortFake{})
	if _, err := svc.GetRiskRules(siSvcUserCtx(), &v1.GetRiskRulesRequest{}); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("GetRiskRules err = %v, want ErrForbidden", err)
	}
	if _, err := svc.UpdateRiskRules(context.Background(), &v1.UpdateRiskRulesRequest{}); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("UpdateRiskRules err = %v, want ErrUnauthorized", err)
	}
}
