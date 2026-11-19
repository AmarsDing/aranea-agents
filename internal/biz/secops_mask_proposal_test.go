package biz

// D6 V4 脱敏五闸提案用例测试（验收 #4 主体：提案永不自动生效、样本闸
// 召回 100%/负样本零命中/无正样本 fail-closed、静态闸 ReDoS 启发式、
// 不可自批、批准只生成 shadow、activate 灰度）。

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── 内存仓储（与 data 层 ReviewProposal/ActivateProposal 的契约同构：
// 审计闭包失败 = 整体失败不落状态）────────────────────────────────────

type maskProposalFakeRepo struct {
	proposals map[string]*MaskProposal
	samples   map[string][]MaskSample
	rules     map[string]*MaskRule
}

func newMaskProposalFakeRepo() *maskProposalFakeRepo {
	return &maskProposalFakeRepo{
		proposals: map[string]*MaskProposal{},
		samples:   map[string][]MaskSample{},
		rules:     map[string]*MaskRule{},
	}
}

func (f *maskProposalFakeRepo) ListProposals(_ context.Context, status string, limit, offset int) ([]MaskProposal, int64, error) {
	all := make([]MaskProposal, 0, len(f.proposals))
	for _, p := range f.proposals {
		if status == "" || p.Status == status {
			all = append(all, *p)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].ID > all[j].ID
		}
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})
	total := int64(len(all))
	if offset > len(all) {
		return nil, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
}

func (f *maskProposalFakeRepo) GetProposal(_ context.Context, id string) (*MaskProposal, error) {
	p, ok := f.proposals[id]
	if !ok {
		return nil, apierror.NotFound(apierror.DomainAdmin, "mask proposal not found")
	}
	cp := *p
	return &cp, nil
}

func (f *maskProposalFakeRepo) CreateProposal(_ context.Context, p MaskProposal) error {
	cp := p
	f.proposals[p.ID] = &cp
	return nil
}

func (f *maskProposalFakeRepo) ListSamplesByEntity(_ context.Context, entity string) ([]MaskSample, error) {
	return f.samples[entity], nil
}

func (f *maskProposalFakeRepo) UpdateGates(_ context.Context, id, fromStatus, toStatus string, recall float64, detailJSON, findingsJSON string) (bool, error) {
	p, ok := f.proposals[id]
	if !ok || p.Status != fromStatus {
		return false, nil
	}
	p.Status = toStatus
	p.SampleRecall = &recall
	p.SampleDetailJSON = detailJSON
	p.StaticFindingsJSON = findingsJSON
	return true, nil
}

func (f *maskProposalFakeRepo) ReviewProposal(ctx context.Context, id, reviewerID string, approve bool, ruleID string, audit func(context.Context) error) (*MaskProposal, error) {
	p, ok := f.proposals[id]
	if !ok {
		return nil, apierror.NotFound(apierror.DomainAdmin, "mask proposal not found")
	}
	if p.Status != MaskProposalStatusPendingApproval {
		return nil, apierror.FailedPrecondition(apierror.DomainAdmin, "proposal is %s", p.Status)
	}
	if reviewerID != "" && p.CreatedBy == reviewerID {
		return nil, apierror.Forbidden(apierror.DomainAdmin, "cannot review own mask proposal")
	}
	status := MaskProposalStatusRejected
	if approve {
		status = MaskProposalStatusShadow
		// 模拟同事务规则插入（审计失败时与状态一起回滚）。
	}
	if err := audit(ctx); err != nil {
		return nil, fmt.Errorf("audit persist failed: %w", err)
	}
	if approve {
		f.rules[ruleID] = &MaskRule{
			ID: ruleID, Name: p.Name, Entity: p.Entity, Pattern: p.Pattern,
			Operator: p.Operator, Enabled: true, Shadow: true, RolloutPct: 100, Version: 1,
		}
	}
	p.Status = status
	p.ReviewedBy = reviewerID
	return p, nil
}

func (f *maskProposalFakeRepo) ActivateProposal(ctx context.Context, id string, rolloutPct int, ruleID string, audit func(context.Context) error) (*MaskProposal, error) {
	p, ok := f.proposals[id]
	if !ok {
		return nil, apierror.NotFound(apierror.DomainAdmin, "mask proposal not found")
	}
	if p.Status != MaskProposalStatusShadow {
		return nil, apierror.FailedPrecondition(apierror.DomainAdmin, "proposal is %s", p.Status)
	}
	rule, ok := f.rules[ruleID]
	if !ok || !rule.Shadow {
		return nil, apierror.Internal(apierror.DomainAdmin, "shadow rule not found")
	}
	if err := audit(ctx); err != nil {
		return nil, err
	}
	rule.Shadow = false
	rule.RolloutPct = rolloutPct
	rule.Version++
	p.Status = MaskProposalStatusActivated
	return p, nil
}

// ── 测试装配 ─────────────────────────────────────────────────────────

type maskProposalAuditSink struct {
	entries []AuditLog
	err     error
}

func (a *maskProposalAuditSink) InsertAuditLog(_ context.Context, entry AuditLog) error {
	if a.err != nil {
		return a.err
	}
	a.entries = append(a.entries, entry)
	return nil
}

func (a *maskProposalAuditSink) actions() []string {
	out := make([]string, 0, len(a.entries))
	for _, e := range a.entries {
		out = append(out, e.Action)
	}
	return out
}

func newMaskProposalTestUC(t *testing.T) (*SecopsMaskProposalUsecase, *maskProposalFakeRepo, *maskProposalAuditSink) {
	t.Helper()
	repo := newMaskProposalFakeRepo()
	uc := NewSecopsMaskProposalUsecase(repo, nil, loggateway.NewNoop())
	now := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	uc.now = func() time.Time { return now }
	uc.newID = func() string { return "mp_fixed" }
	sink := &maskProposalAuditSink{}
	uc.SetAuditRecorder(sink)
	return uc, repo, sink
}

func seedMaskPhoneSamples(repo *maskProposalFakeRepo) {
	repo.samples["手机号"] = []MaskSample{
		{ID: "ms_phone_p1", Entity: "手机号", Text: "我的手机号是13812345678，下班打给我", Positive: true},
		{ID: "ms_phone_p2", Entity: "手机号", Text: "紧急联系人电话13998765432", Positive: true},
		{ID: "ms_phone_n1", Entity: "手机号", Text: "订单号20261103001已发货，共11位数字编号", Positive: false},
	}
}

// ── 闸门1：提案入库 ───────────────────────────────────────────────────

func TestMaskProposalCreate_NormalizesAndProposed(t *testing.T) {
	uc, repo, sink := newMaskProposalTestUC(t)
	p, err := uc.CreateProposal(context.Background(), MaskProposalInput{
		Entity: " 手机号 ", Pattern: `\b1[3-9]\d{9}\b`,
	}, "u1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.Status != MaskProposalStatusProposed {
		t.Fatalf("status = %s, want proposed (AI/手工永不自动生效)", p.Status)
	}
	if p.ID != "mp_fixed" || p.Name != "手机号" || p.Entity != "手机号" {
		t.Fatalf("normalize: %+v", p)
	}
	if p.Operator != MaskOperatorPlaceholder || p.Source != MaskProposalSourceManual {
		t.Fatalf("defaults: operator=%s source=%s", p.Operator, p.Source)
	}
	if p.WordlistJSON != "[]" {
		t.Fatalf("wordlist default = %q", p.WordlistJSON)
	}
	if _, ok := repo.proposals["mp_fixed"]; !ok {
		t.Fatal("提案未入库")
	}
	if got := sink.actions(); len(got) != 1 || got[0] != SecopsMaskProposalActionCreate {
		t.Fatalf("audits = %v", got)
	}
}

func TestMaskProposalCreate_Validation(t *testing.T) {
	uc, _, _ := newMaskProposalTestUC(t)
	cases := []struct {
		name string
		in   MaskProposalInput
	}{
		{"empty entity", MaskProposalInput{Pattern: `x`}},
		{"no pattern no wordlist", MaskProposalInput{Entity: "手机号"}},
		{"bad operator", MaskProposalInput{Entity: "手机号", Pattern: `x`, Operator: "replace"}},
		{"bad wordlist json", MaskProposalInput{Entity: "手机号", WordlistJSON: `{"a":1}`}},
		{"wordlist not string array", MaskProposalInput{Entity: "手机号", WordlistJSON: `[1,2]`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := uc.CreateProposal(context.Background(), tc.in, "u1"); !apierror.IsCode(err, apierror.CodeBadRequest) {
				t.Fatalf("want BadRequest, got %v", err)
			}
		})
	}
}

// ── 闸门2：样本试跑 ───────────────────────────────────────────────────

func TestRunMaskSampleGate_RecallAndNegative(t *testing.T) {
	samples := []MaskSample{
		{ID: "p1", Text: "联系 13812345678", Positive: true},
		{ID: "p2", Text: "备用 13900001111", Positive: true},
		{ID: "n1", Text: "订单 20261103001", Positive: false},
	}
	recall, detail, pass, err := RunMaskSampleGate(samples, MaskRule{Entity: "手机号", Pattern: `\b1[3-9]\d{9}\b`, Operator: MaskOperatorPlaceholder})
	if err != nil || !pass {
		t.Fatalf("pass=%v err=%v", pass, err)
	}
	if recall != 1.0 {
		t.Fatalf("recall = %v", recall)
	}
	var results []MaskGateSampleResult
	if err := json.Unmarshal([]byte(detail), &results); err != nil {
		t.Fatalf("detail json: %v", err)
	}
	if len(results) != 3 || !results[0].Pass || !results[2].Pass {
		t.Fatalf("results = %+v", results)
	}
	// 负样本被误命中 → 不通过。
	recall, _, pass, _ = RunMaskSampleGate([]MaskSample{{ID: "n1", Text: "13812345678", Positive: false}}, MaskRule{Entity: "手机号", Pattern: `\b1[3-9]\d{9}\b`})
	if pass || recall != 0 {
		t.Fatalf("negative hit must fail: pass=%v recall=%v", pass, recall)
	}
}

func TestRunMaskSampleGate_NoPositiveFailClosed(t *testing.T) {
	recall, detail, pass, err := RunMaskSampleGate([]MaskSample{{ID: "n1", Text: "无", Positive: false}}, MaskRule{Entity: "X", Pattern: `\b1[3-9]\d{9}\b`})
	if err != nil || pass {
		t.Fatalf("no positive samples must fail-closed: pass=%v err=%v", pass, err)
	}
	if recall != 0 || !strings.Contains(detail, "无正样本") {
		t.Fatalf("detail = %q recall=%v", detail, recall)
	}
}

// ── 闸门2+3 编排 ─────────────────────────────────────────────────────

func TestMaskProposalRunGates_SampleFailedLandsTerminal(t *testing.T) {
	uc, repo, sink := newMaskProposalTestUC(t)
	seedMaskPhoneSamples(repo)
	repo.proposals["mp_1"] = &MaskProposal{ID: "mp_1", Name: "手机号", Entity: "手机号", Pattern: `\b1[3-9]\d{9}\d\b`, WordlistJSON: "[]", Operator: MaskOperatorPlaceholder, Status: MaskProposalStatusProposed, CreatedBy: "u1"}

	p, err := uc.RunMaskGates(context.Background(), "mp_1")
	if err != nil {
		t.Fatalf("gates: %v", err)
	}
	if p.Status != MaskProposalStatusSampleFailed {
		t.Fatalf("status = %s, want sample_failed", p.Status)
	}
	if p.SampleRecall == nil || *p.SampleRecall != 0 {
		t.Fatalf("recall = %v", p.SampleRecall)
	}
	got := sink.actions()
	if len(got) != 1 || got[0] != SecopsMaskProposalActionSampleFailed {
		t.Fatalf("audits = %v", got)
	}
	// wb 语义：sample_failed/static_failed 可修提案后再跑闸门（幂等重跑
	// 同样落终态）。
	p2, err := uc.RunMaskGates(context.Background(), "mp_1")
	if err != nil {
		t.Fatalf("rerun after fix must be allowed: %v", err)
	}
	if p2.Status != MaskProposalStatusSampleFailed {
		t.Fatalf("rerun status = %s", p2.Status)
	}
}

func TestMaskProposalRunGates_StaticFailed(t *testing.T) {
	uc, repo, sink := newMaskProposalTestUC(t)
	seedMaskPhoneSamples(repo)
	// 召回覆盖样本，但含重叠通配分支 → 静态闸拒。
	repo.proposals["mp_1"] = &MaskProposal{ID: "mp_1", Name: "手机号", Entity: "手机号", Pattern: `\b1[3-9]\d{9}\b(.*)(.*)`, WordlistJSON: "[]", Operator: MaskOperatorPlaceholder, Status: MaskProposalStatusProposed, CreatedBy: "u1"}

	p, err := uc.RunMaskGates(context.Background(), "mp_1")
	if err != nil {
		t.Fatalf("gates: %v", err)
	}
	if p.Status != MaskProposalStatusStaticFailed {
		t.Fatalf("status = %s, want static_failed", p.Status)
	}
	if !strings.Contains(p.StaticFindingsJSON, "重叠通配分支") {
		t.Fatalf("findings = %q", p.StaticFindingsJSON)
	}
	got := sink.actions()
	if len(got) != 1 || got[0] != SecopsMaskProposalActionStaticFailed {
		t.Fatalf("audits = %v", got)
	}
}

func TestMaskProposalRunGates_PassToPendingApproval(t *testing.T) {
	uc, repo, sink := newMaskProposalTestUC(t)
	seedMaskPhoneSamples(repo)
	repo.proposals["mp_1"] = &MaskProposal{ID: "mp_1", Name: "手机号", Entity: "手机号", Pattern: `\b1[3-9]\d{9}\b`, WordlistJSON: "[]", Operator: MaskOperatorPlaceholder, Status: MaskProposalStatusProposed, CreatedBy: "u1"}

	p, err := uc.RunMaskGates(context.Background(), "mp_1")
	if err != nil {
		t.Fatalf("gates: %v", err)
	}
	if p.Status != MaskProposalStatusPendingApproval || p.StaticFindingsJSON != "[]" {
		t.Fatalf("p = %+v", p)
	}
	if p.SampleRecall == nil || *p.SampleRecall != 1 {
		t.Fatalf("recall = %v", p.SampleRecall)
	}
	got := sink.actions()
	if len(got) != 1 || got[0] != SecopsMaskProposalActionGatePassed {
		t.Fatalf("audits = %v", got)
	}
}

func TestMaskProposalRunGates_CompileErrorBadRequest(t *testing.T) {
	uc, repo, _ := newMaskProposalTestUC(t)
	seedMaskPhoneSamples(repo)
	repo.proposals["mp_1"] = &MaskProposal{ID: "mp_1", Name: "X", Entity: "手机号", Pattern: `[unclosed`, WordlistJSON: "[]", Operator: MaskOperatorPlaceholder, Status: MaskProposalStatusProposed}
	if _, err := uc.RunMaskGates(context.Background(), "mp_1"); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("compile error must be BadRequest, got %v", err)
	}
	// 状态未迁移。
	if got := repo.proposals["mp_1"].Status; got != MaskProposalStatusProposed {
		t.Fatalf("status = %s, want proposed unchanged", got)
	}
}

// ── 闸门3 静态检查单测 ────────────────────────────────────────────────

func TestStaticCheckMaskPattern(t *testing.T) {
	if f := StaticCheckMaskPattern(""); len(f) != 0 {
		t.Fatalf("empty pattern findings = %v", f)
	}
	if f := StaticCheckMaskPattern(`\b1[3-9]\d{9}\b`); len(f) != 0 {
		t.Fatalf("clean pattern findings = %v", f)
	}
	nested := StaticCheckMaskPattern(`(a+)+$`)
	if len(nested) == 0 || !strings.Contains(nested[0], "嵌套量词") {
		t.Fatalf("nested findings = %v", nested)
	}
	// 量词叠加启发式对「能编译但含 +* 相邻片段」的写法生效（如字符类）。
	// 形如 a+* 的裸写法 RE2 直接编译失败，走编译失败分支（wb 同序）。
	stacked := StaticCheckMaskPattern(`x[a+*]`)
	if len(stacked) == 0 || !strings.Contains(stacked[0], "量词叠加") {
		t.Fatalf("stacked findings = %v", stacked)
	}
	if f := StaticCheckMaskPattern(`(.*)|x`); len(f) == 0 || !strings.Contains(f[0], "重叠通配分支") {
		t.Fatalf("wildcard findings = %v", f)
	}
	if f := StaticCheckMaskPattern(`(.*)(.*)`); len(f) == 0 || !strings.Contains(f[0], "重叠通配分支") {
		t.Fatalf("双通配 findings = %v", f)
	}
	compile := StaticCheckMaskPattern(`[unclosed`)
	if len(compile) != 1 || !strings.Contains(compile[0], "正则编译失败") {
		t.Fatalf("compile findings = %v", compile)
	}
}

// ── 闸门4：审批 ───────────────────────────────────────────────────────

func seedPendingProposal(repo *maskProposalFakeRepo, id, createdBy string) {
	repo.proposals[id] = &MaskProposal{
		ID: id, Name: "手机号", Entity: "手机号", Pattern: `\b1[3-9]\d{9}\b`,
		WordlistJSON: "[]", Operator: MaskOperatorPlaceholder,
		Status: MaskProposalStatusPendingApproval, CreatedBy: createdBy,
	}
}

func TestMaskProposalApprove_CreatesShadowRule(t *testing.T) {
	uc, repo, sink := newMaskProposalTestUC(t)
	seedPendingProposal(repo, "mp_1", "u1")
	p, err := uc.Approve(context.Background(), "mp_1", "u2")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if p.Status != MaskProposalStatusShadow {
		t.Fatalf("status = %s, want shadow (批准只生成影子规则)", p.Status)
	}
	ruleID := MaskRuleIDForProposal("mp_1")
	rule, ok := repo.rules[ruleID]
	if !ok || !rule.Shadow || !rule.Enabled || rule.RolloutPct != 100 {
		t.Fatalf("shadow rule = %+v", rule)
	}
	got := sink.actions()
	if len(got) != 1 || got[0] != SecopsMaskProposalActionApprove {
		t.Fatalf("audits = %v", got)
	}
	if !strings.Contains(sink.entries[0].Detail, ruleID) {
		t.Fatalf("detail = %q, want rule_id", sink.entries[0].Detail)
	}
}

func TestMaskProposalApprove_SelfApprovalForbidden(t *testing.T) {
	uc, repo, _ := newMaskProposalTestUC(t)
	seedPendingProposal(repo, "mp_1", "u1")
	if _, err := uc.Approve(context.Background(), "mp_1", "u1"); !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("self approve must be Forbidden, got %v", err)
	}
	if _, ok := repo.rules[MaskRuleIDForProposal("mp_1")]; ok {
		t.Fatal("self-approval must not create shadow rule")
	}
}

func TestMaskProposalReview_StatusGuard(t *testing.T) {
	uc, repo, _ := newMaskProposalTestUC(t)
	repo.proposals["mp_1"] = &MaskProposal{ID: "mp_1", Status: MaskProposalStatusProposed, CreatedBy: "u1"}
	if _, err := uc.Approve(context.Background(), "mp_1", "u2"); !apierror.IsCode(err, apierror.CodeFailedPrecondition) {
		t.Fatalf("proposed cannot be approved directly, got %v", err)
	}
	if _, err := uc.Reject(context.Background(), "mp_1", "u2"); !apierror.IsCode(err, apierror.CodeFailedPrecondition) {
		t.Fatalf("proposed cannot be rejected, got %v", err)
	}
}

func TestMaskProposalReject_LandsTerminal(t *testing.T) {
	uc, repo, sink := newMaskProposalTestUC(t)
	seedPendingProposal(repo, "mp_1", "u1")
	p, err := uc.Reject(context.Background(), "mp_1", "u2")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if p.Status != MaskProposalStatusRejected {
		t.Fatalf("status = %s", p.Status)
	}
	if _, ok := repo.rules[MaskRuleIDForProposal("mp_1")]; ok {
		t.Fatal("reject must not create rule")
	}
	got := sink.actions()
	if len(got) != 1 || got[0] != SecopsMaskProposalActionReject {
		t.Fatalf("audits = %v", got)
	}
}

func TestMaskProposalApprove_AuditFailureRollsBack(t *testing.T) {
	uc, repo, sink := newMaskProposalTestUC(t)
	seedPendingProposal(repo, "mp_1", "u1")
	sink.err = fmt.Errorf("audit down")
	if _, err := uc.Approve(context.Background(), "mp_1", "u2"); err == nil || !strings.Contains(err.Error(), "audit persist failed") {
		t.Fatalf("audit failure must block approval, got %v", err)
	}
	if _, ok := repo.rules[MaskRuleIDForProposal("mp_1")]; ok {
		t.Fatal("audit failure must not leave shadow rule")
	}
	if got := repo.proposals["mp_1"].Status; got != MaskProposalStatusPendingApproval {
		t.Fatalf("status = %s, want pending_approval unchanged", got)
	}
}

// ── 闸门5→生效：激活 ─────────────────────────────────────────────────

func TestMaskProposalActivate_ShadowToActivated(t *testing.T) {
	uc, repo, sink := newMaskProposalTestUC(t)
	seedPendingProposal(repo, "mp_1", "u1")
	if _, err := uc.Approve(context.Background(), "mp_1", "u2"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	p, err := uc.Activate(context.Background(), "mp_1", 30)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if p.Status != MaskProposalStatusActivated {
		t.Fatalf("status = %s", p.Status)
	}
	rule := repo.rules[MaskRuleIDForProposal("mp_1")]
	if rule.Shadow || rule.RolloutPct != 30 || rule.Version != 2 {
		t.Fatalf("rule = %+v", rule)
	}
	got := sink.actions()
	if len(got) != 2 || got[1] != SecopsMaskProposalActionActivate {
		t.Fatalf("audits = %v", got)
	}
}

func TestMaskProposalActivate_RolloutPctFallback(t *testing.T) {
	for _, pct := range []int{0, -3, 101} {
		t.Run(fmt.Sprintf("pct=%d", pct), func(t *testing.T) {
			uc, repo, _ := newMaskProposalTestUC(t)
			seedPendingProposal(repo, "mp_1", "u1")
			if _, err := uc.Approve(context.Background(), "mp_1", "u2"); err != nil {
				t.Fatalf("approve: %v", err)
			}
			if _, err := uc.Activate(context.Background(), "mp_1", pct); err != nil {
				t.Fatalf("activate: %v", err)
			}
			if got := repo.rules[MaskRuleIDForProposal("mp_1")].RolloutPct; got != 100 {
				t.Fatalf("rollout_pct = %d, want 100 fallback", got)
			}
		})
	}
}

func TestMaskProposalActivate_StatusGuard(t *testing.T) {
	uc, repo, _ := newMaskProposalTestUC(t)
	seedPendingProposal(repo, "mp_1", "u1")
	// pending_approval（未批准）不可激活。
	if _, err := uc.Activate(context.Background(), "mp_1", 10); !apierror.IsCode(err, apierror.CodeFailedPrecondition) {
		t.Fatalf("pending cannot activate, got %v", err)
	}
	// shadow 但影子规则缺失 → Internal（wb 500 语义）。
	repo.proposals["mp_2"] = &MaskProposal{ID: "mp_2", Status: MaskProposalStatusShadow}
	if _, err := uc.Activate(context.Background(), "mp_2", 10); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("missing shadow rule must be Internal, got %v", err)
	}
}

// ── 列表分页与状态校验 ────────────────────────────────────────────────

func TestMaskProposalList_PaginationAndStatusValidation(t *testing.T) {
	uc, repo, _ := newMaskProposalTestUC(t)
	base := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("mp_%d", i)
		repo.proposals[id] = &MaskProposal{ID: id, Status: MaskProposalStatusProposed, CreatedAt: base.Add(time.Duration(i) * time.Minute)}
	}
	items, total, err := uc.ListProposals(context.Background(), "", 1, 2)
	if err != nil || total != 3 || len(items) != 2 {
		t.Fatalf("page1: items=%d total=%d err=%v", len(items), total, err)
	}
	if items[0].ID != "mp_2" {
		t.Fatalf("order = %s, want newest first", items[0].ID)
	}
	_, totalShadow, err := uc.ListProposals(context.Background(), MaskProposalStatusShadow, 1, 0)
	if err != nil || totalShadow != 0 {
		t.Fatalf("status filter: total=%d err=%v", totalShadow, err)
	}
	if _, _, err := uc.ListProposals(context.Background(), "bogus", 1, 0); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("bogus status must be BadRequest, got %v", err)
	}
}

// MaskRuleIDForProposal 是确定性关联（approve/activate 同一条规则）。
func TestMaskRuleIDForProposal_Deterministic(t *testing.T) {
	if got := MaskRuleIDForProposal("mp_1"); got != "mr_mp_1" {
		t.Fatalf("rule id = %q", got)
	}
	if MaskRuleIDForProposal("mp_1") != MaskRuleIDForProposal("mp_1") {
		t.Fatal("rule id must be deterministic")
	}
}
