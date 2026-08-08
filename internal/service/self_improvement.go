package service

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	v1 "aranea-agents/api/kratos/self_improvement/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// siAdminPort is the service-side view of biz.SelfImprovementAdminUsecase
// (kept narrow for test doubles; the concrete usecase satisfies it).
type siAdminPort interface {
	List(ctx context.Context, filter biz.RunFilter) ([]biz.SelfImprovementRun, int, error)
	Get(ctx context.Context, runID string) (*biz.SelfImprovementRun, error)
	Approve(ctx context.Context, runID, operator, reason string) error
	Reject(ctx context.Context, runID, operator, reason string) error
	Rollback(ctx context.Context, runID, operator, reason string) error
	Close(ctx context.Context, runID, operator string) error
	OutcomeStats(ctx context.Context) (*biz.SIOutcomeStats, error)
	GetRiskRules(ctx context.Context) (biz.SIRiskRules, error)
	UpdateRiskRules(ctx context.Context, operator string, rules biz.SIRiskRules) (biz.SIRiskRules, error)
}

// SIRefineLLMReader is the narrow read port for the platform DefaultRefineLLM
// setting — the Analyst/Patcher/Critic stages' hard dependency (satisfied by
// *biz.SystemSettingUsecase). nil → GetStatus reports refine_llm_configured=false.
type SIRefineLLMReader interface {
	GetRefineLLM(ctx context.Context) (biz.RefineLLMSetting, error)
}

// SelfImprovementService implements the proto-generated
// SelfImprovementServiceServer: the P5 console API of the platform
// self-improvement loop (73-self-iteration-v3, design §七). All endpoints
// require admin auth; operator identity is taken from the authenticated
// principal (never from the request body).
//
// The service is ALWAYS constructed and registered (P5.5) — even when the
// feature is disabled — so the console gets a structured 503 SELF_IMPROVEMENT
// instead of a bare 404; GetStatus answers regardless of the switch.
type SelfImprovementService struct {
	v1.UnimplementedSelfImprovementServiceServer

	uc siAdminPort // nil when self_improvement.enabled=false → 业务端点 Unavailable
	// cfg is the self_improvement config block (nil-safe accessors).
	cfg *conf.SelfImprovement
	// refineLLM reads DefaultRefineLLM for the GetStatus preflight (nil → 降级为未配置).
	refineLLM SIRefineLLMReader
	// control is the in-memory user-intervention command plane consumed by the
	// pipeline at stage boundaries (nil → ControlRun Unavailable).
	control *biz.SIControlPlane
	lg      loggateway.Logger
}

// NewSelfImprovementService builds the console service. uc may be nil when
// self_improvement.enabled=false — business endpoints then return Unavailable
// with reason SELF_IMPROVEMENT, while GetStatus stays available.
//
// NOTE: uc arrives as a concrete *biz.SelfImprovementAdminUsecase; storing a
// nil concrete pointer in the siAdminPort interface field would create a
// typed-nil interface (s.uc == nil is false → nil-receiver panic → 500).
// Convert explicitly so the disabled guard in requireAdmin actually fires.
func NewSelfImprovementService(uc *biz.SelfImprovementAdminUsecase, cfg *conf.SelfImprovement, refineLLM SIRefineLLMReader, control *biz.SIControlPlane, lg loggateway.Logger) *SelfImprovementService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	var port siAdminPort
	if uc != nil {
		port = uc
	}
	return &SelfImprovementService{uc: port, cfg: cfg, refineLLM: refineLLM, control: control, lg: lg}
}

// requireAuth enforces admin access only（GetStatus 等不依赖管线的端点用）。
func (s *SelfImprovementService) requireAuth(ctx context.Context) (*auth.Auth, error) {
	a, ok := auth.FromContext(ctx)
	if !ok || a == nil {
		return nil, auth.ErrUnauthorized
	}
	if !a.HasAdminAccess() {
		return nil, auth.ErrForbidden
	}
	return a, nil
}

// requireAdmin authenticates the principal and guards the (possibly
// feature-disabled) usecase.
func (s *SelfImprovementService) requireAdmin(ctx context.Context) (*auth.Auth, error) {
	a, err := s.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if s == nil || s.uc == nil {
		return nil, apierror.Unavailable("SELF_IMPROVEMENT", "self-improvement feature not enabled")
	}
	return a, nil
}

// GetStatus reports the feature availability + hard-prerequisite preflight
// (design §七, P5.5). It intentionally does NOT depend on the admin usecase so
// the console can render the disabled empty state / missing-prerequisite
// guidance even when the pipeline is off. All probes degrade to false/empty,
// never to an error.
func (s *SelfImprovementService) GetStatus(ctx context.Context, _ *v1.GetStatusRequest) (*v1.GetStatusResponse, error) {
	if _, err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	resp := &v1.GetStatusResponse{Enabled: s.cfg.SIEnabled()}
	if s.refineLLM != nil {
		if rl, err := s.refineLLM.GetRefineLLM(ctx); err == nil {
			resp.RefineLlmProvider = strings.TrimSpace(rl.Provider)
			resp.RefineLlmModel = strings.TrimSpace(rl.Model)
		}
	}
	resp.RefineLlmConfigured = resp.RefineLlmProvider != "" && resp.RefineLlmModel != ""
	root := s.cfg.SIRepoRoot()
	if root == "" {
		if wd, err := os.Getwd(); err == nil {
			root = wd
		}
	}
	resp.RepoRoot = root
	resp.RepoRootValid = siRepoRootValid(root)
	return resp, nil
}

// siRepoRootValid reports whether dir exists and looks like a git checkout
// (the sandbox/applier hard requirement: worktree + merge run git inside it).
func siRepoRootValid(dir string) bool {
	if dir == "" {
		return false
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return false
	}
	return true
}

// ListRuns returns runs filtered by status / risk_level / trigger_source with
// pagination. Heavy fields (diff/diagnosis/reports) are omitted.
func (s *SelfImprovementService) ListRuns(ctx context.Context, req *v1.ListRunsRequest) (*v1.ListRunsResponse, error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	limit, offset, page, pageSize := biz.PageToLimitOffset(req.GetPage(), req.GetPageSize())
	filter := biz.RunFilter{
		Status:        biz.SelfImprovementRunStatus(req.GetStatus()),
		RiskLevel:     biz.SelfImprovementRiskLevel(req.GetRiskLevel()),
		TriggerSource: req.GetTriggerSource(),
		Limit:         limit,
		Offset:        offset,
	}
	runs, total, err := s.uc.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListRunsResponse{Total: int64(total), Page: page, PageSize: pageSize}
	for i := range runs {
		resp.Items = append(resp.Items, siRunToProto(&runs[i], false))
	}
	return resp, nil
}

// GetRun returns the full run detail including diff/diagnosis/reports.
func (s *SelfImprovementService) GetRun(ctx context.Context, req *v1.GetRunRequest) (*v1.GetRunResponse, error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	run, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &v1.GetRunResponse{Run: siRunToProto(run, true)}, nil
}

// ApproveRun approves a high-risk run (awaiting_governance → applying).
func (s *SelfImprovementService) ApproveRun(ctx context.Context, req *v1.ApproveRunRequest) (*v1.ApproveRunResponse, error) {
	a, err := s.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.uc.Approve(ctx, req.GetId(), siOperator(a), req.GetReason()); err != nil {
		return nil, err
	}
	return &v1.ApproveRunResponse{}, nil
}

// RejectRun rejects an awaiting_governance run (reason required).
func (s *SelfImprovementService) RejectRun(ctx context.Context, req *v1.RejectRunRequest) (*v1.RejectRunResponse, error) {
	a, err := s.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.uc.Reject(ctx, req.GetId(), siOperator(a), req.GetReason()); err != nil {
		return nil, err
	}
	return &v1.RejectRunResponse{}, nil
}

// RollbackRun manually rolls back an applied/observing run.
func (s *SelfImprovementService) RollbackRun(ctx context.Context, req *v1.RollbackRunRequest) (*v1.RollbackRunResponse, error) {
	a, err := s.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.uc.Rollback(ctx, req.GetId(), siOperator(a), req.GetReason()); err != nil {
		return nil, err
	}
	return &v1.RollbackRunResponse{}, nil
}

// CloseRun closes an observing run early (confirmed effective).
func (s *SelfImprovementService) CloseRun(ctx context.Context, req *v1.CloseRunRequest) (*v1.CloseRunResponse, error) {
	a, err := s.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.uc.Close(ctx, req.GetId(), siOperator(a)); err != nil {
		return nil, err
	}
	return &v1.CloseRunResponse{}, nil
}

// ControlRun issues a user-intervention command (pause/skip_retry/rollback)
// to an in-flight run. The command is enqueued into the in-memory control
// plane and consumed asynchronously by the pipeline at stage boundaries; this
// endpoint does not change the run status synchronously. Only runs in
// detected/diagnosing/patching/verifying accept commands (other statuses never
// reach a poll point, so a command would linger unconsumed).
func (s *SelfImprovementService) ControlRun(ctx context.Context, req *v1.ControlRunRequest) (*v1.ControlRunResponse, error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.control == nil {
		return nil, apierror.Unavailable("SELF_IMPROVEMENT", "self-improvement control plane not wired")
	}
	cmd, err := biz.ParseSIControlCommand(strings.TrimSpace(req.GetCommand()))
	if err != nil {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "%s", err.Error())
	}
	run, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	switch run.Status {
	case biz.RunStatusDetected, biz.RunStatusDiagnosing, biz.RunStatusPatching, biz.RunStatusVerifying:
	default:
		return nil, apierror.Conflict("SELF_IMPROVEMENT", "run %s in status %s does not accept control commands", run.ID, run.Status)
	}
	if err := s.control.Issue(run.ID, cmd); err != nil {
		return nil, apierror.Internal("SELF_IMPROVEMENT", "%s", err.Error())
	}
	s.lg.Info("self-improve control command issued",
		loggateway.StepID("si_console.control"),
		loggateway.Str("run_id", run.ID), loggateway.Str("command", string(cmd)))
	return &v1.ControlRunResponse{}, nil
}

// GetOutcomeStats returns Learn-stage attribution statistics (verdict
// distribution + per-trigger breakdown).
func (s *SelfImprovementService) GetOutcomeStats(ctx context.Context, _ *v1.GetOutcomeStatsRequest) (*v1.GetOutcomeStatsResponse, error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	stats, err := s.uc.OutcomeStats(ctx)
	if err != nil {
		return nil, err
	}
	resp := &v1.GetOutcomeStatsResponse{
		Total:         int64(stats.Total),
		Effective:     int64(stats.Effective),
		Neutral:       int64(stats.Neutral),
		Regressed:     int64(stats.Regressed),
		EffectiveRate: stats.EffectiveRate,
		RollbackRate:  stats.RollbackRate,
	}
	for _, t := range stats.ByTrigger {
		resp.ByTrigger = append(resp.ByTrigger, &v1.TriggerOutcomeStatsMsg{
			TriggerSource: t.TriggerSource,
			Total:         int64(t.Total),
			Effective:     int64(t.Effective),
			Neutral:       int64(t.Neutral),
			Regressed:     int64(t.Regressed),
		})
	}
	return resp, nil
}

// GetRiskRules returns the raw configured rule set plus the normalized
// effective view actually consumed by the pipeline/router.
func (s *SelfImprovementService) GetRiskRules(ctx context.Context, _ *v1.GetRiskRulesRequest) (*v1.GetRiskRulesResponse, error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	rules, err := s.uc.GetRiskRules(ctx)
	if err != nil {
		return nil, err
	}
	return &v1.GetRiskRulesResponse{
		Configured: siRiskRulesToProto(rules),
		Effective:  siRiskRulesToProto(biz.NormalizeSIRiskRules(rules)),
	}, nil
}

// UpdateRiskRules validates + persists a new rule set (operator identity from
// the authenticated principal). Takes effect for newly-classified runs.
func (s *SelfImprovementService) UpdateRiskRules(ctx context.Context, req *v1.UpdateRiskRulesRequest) (*v1.UpdateRiskRulesResponse, error) {
	a, err := s.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	saved, err := s.uc.UpdateRiskRules(ctx, siOperator(a), biz.SIRiskRules{
		LowMaxLines:    int(req.GetLowMaxLines()),
		MediumMaxLines: int(req.GetMediumMaxLines()),
		CorePathGlobs:  req.GetCorePathGlobs(),
		DailyAutoQuota: req.GetDailyAutoQuota(),
	})
	if err != nil {
		return nil, err
	}
	return &v1.UpdateRiskRulesResponse{
		Configured: siRiskRulesToProto(saved),
		Effective:  siRiskRulesToProto(biz.NormalizeSIRiskRules(saved)),
	}, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

// siOperator derives the operator identity from the authenticated principal
// (audit fields: approved_by / closed_reason).
func siOperator(a *auth.Auth) string {
	return strconv.FormatInt(a.UserID, 10)
}

// siRunToProto converts a biz run to the console message. detail=false omits
// the heavy fields (diff/diagnosis/verification_report/critic_report/
// governance) for the list view.
func siRunToProto(run *biz.SelfImprovementRun, detail bool) *v1.SelfImprovementRunMsg {
	msg := &v1.SelfImprovementRunMsg{
		Id:            run.ID,
		SuggestionId:  run.SuggestionID,
		Status:        string(run.Status),
		TriggerSource: run.TriggerSource,
		PatchKind:     string(run.PatchKind),
		RiskLevel:     string(run.RiskLevel),
		BaseRef:       run.BaseRef,
		Branch:        run.Branch,
		DiffStats: &v1.DiffStatsMsg{
			Files:     int32(run.DiffStats.Files),
			Additions: int32(run.DiffStats.Additions),
			Deletions: int32(run.DiffStats.Deletions),
		},
		Attempts:      int32(run.Attempts),
		ApprovedBy:    run.ApprovedBy,
		AppliedCommit: run.AppliedCommit,
		ClosedReason:  run.ClosedReason,
		CreatedAt:     timestamppb.New(run.CreatedAt),
		UpdatedAt:     timestamppb.New(run.UpdatedAt),
	}
	if run.ObserveUntil != nil {
		msg.ObserveUntil = timestamppb.New(*run.ObserveUntil)
	}
	if !detail {
		return msg
	}
	msg.Diff = run.Diff
	if d := run.Diagnosis; d != nil {
		msg.Diagnosis = &v1.DiagnosisMsg{
			RootCause:     d.RootCause,
			AffectedFiles: d.AffectedFiles,
			ImpactScope:   d.ImpactScope,
			FixStrategy:   d.FixStrategy,
			Confidence:    d.Confidence,
		}
	}
	for _, g := range run.VerificationReport {
		msg.VerificationReport = append(msg.VerificationReport, &v1.SandboxGateResultMsg{
			Gate:       string(g.Gate),
			Passed:     g.Passed,
			Output:     g.Output,
			DurationMs: g.DurationMS,
		})
	}
	if c := run.CriticReport; c != nil {
		msg.CriticReport = &v1.CriticReportMsg{
			IsSafe:     c.IsSafe,
			RiskLevel:  c.RiskLevel,
			Concerns:   c.Concerns,
			Suggestion: c.Suggestion,
		}
	}
	if g := run.Governance; g != nil {
		msg.Governance = &v1.GovernanceDecisionMsg{
			RiskLevel: string(g.RiskLevel),
			Channel:   g.Channel,
			RuleHits:  g.RuleHits,
		}
	}
	return msg
}

// siRiskRulesToProto converts one biz rule-set view (configured or
// normalized-effective) to the console message.
func siRiskRulesToProto(r biz.SIRiskRules) *v1.RiskRulesMsg {
	return &v1.RiskRulesMsg{
		LowMaxLines:    int32(r.LowMaxLines),
		MediumMaxLines: int32(r.MediumMaxLines),
		CorePathGlobs:  r.CorePathGlobs,
		DailyAutoQuota: r.DailyAutoQuota,
	}
}
