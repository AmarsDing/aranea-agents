package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"aranea-agents/internal/biz"
	bizmonitor "aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── Self-improvement port adapters (73-self-iteration-v3, P3 偏差回收 / W6) ──
//
// Service-layer adapters bridging the biz self-improvement ports to platform
// infrastructure:
//
//	SINotifier            → SIMonitorNotifier        （Monitor Events，Events 页可见）
//	SIApprovalSink        → SIMonitorApprovalSink    （Monitor Events 审批请求，幂等）
//	SIActivitySink        → SIMonitorActivitySink    （Meta Team 阶段树 → Monitor Events）
//	SINegativePatternSink → SIKBNegativePatternSink  （FailurePattern KB，D8 负面样本）
//	SITriggerFeedbackSink → SIOrchestratorFeedbackSink（统一编排器冷却乘数，D8 降频）
//
// 聊天审批 activity / 前端审批决议属 P5（Proto + 控制台）；P4 经 Monitor
// Events 暴露待审批项，操作员用 Operator usecase 内部路径决议。

const (
	// siEventKeyNotify / siEventKeyApproval / siEventKeyStage are the monitor
	// event keys emitted by the adapters (Events 页 event_type 过滤用）。
	siEventKeyNotify   = "self_improvement.notify"
	siEventKeyApproval = "self_improvement.approval_request"
	siEventKeyStage    = "self_improvement.stage"
	// siApprovalScanLimit bounds the idempotency scan of recent approval
	// requests (awaiting_governance runs are few; 100 足够覆盖）。
	siApprovalScanLimit = 100
)

// siRunEventMetadata packs the common run identity into event metadata.
func siRunEventMetadata(run *biz.SelfImprovementRun) map[string]any {
	m := map[string]any{}
	if run == nil {
		return m
	}
	m["run_id"] = run.ID
	m["suggestion_id"] = run.SuggestionID
	m["trigger_source"] = run.TriggerSource
	m["risk_level"] = string(run.RiskLevel)
	m["patch_kind"] = string(run.PatchKind)
	m["diff_additions"] = run.DiffStats.Additions
	m["diff_deletions"] = run.DiffStats.Deletions
	return m
}

func siMarshalMetadata(m map[string]any) string {
	data, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// SIMonitorNotifier delivers operator-facing self-improvement notifications
// as monitor events (biz.SINotifier).
type SIMonitorNotifier struct {
	events biz.MonitorEventRepo
	lg     loggateway.Logger
}

// NewSIMonitorNotifier wires the notifier adapter.
func NewSIMonitorNotifier(events biz.MonitorEventRepo, lg loggateway.Logger) *SIMonitorNotifier {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SIMonitorNotifier{events: events, lg: lg.With(loggateway.Domain("si_notifier"))}
}

// NotifySelfImprovement implements biz.SINotifier.
func (n *SIMonitorNotifier) NotifySelfImprovement(ctx context.Context, run *biz.SelfImprovementRun, message string) error {
	if n == nil || n.events == nil {
		return apierror.Internal("SELF_IMPROVEMENT", "monitor notifier not initialized")
	}
	return n.events.InsertMonitorEvent(ctx, bizmonitor.EventWrite{
		EventKey:     siEventKeyNotify,
		Name:         "自改进通知",
		Description:  message,
		Status:       "warn",
		MetadataJSON: siMarshalMetadata(siRunEventMetadata(run)),
	})
}

// SIMonitorApprovalSink files manual-approval requests as monitor events
// (biz.SIApprovalSink). Submission is idempotent per run: a recent approval
// request carrying the same run_id suppresses re-insertion (drive 重启后
// re-route 不产生重复请求）。
type SIMonitorApprovalSink struct {
	events biz.MonitorEventRepo
	lg     loggateway.Logger
}

// NewSIMonitorApprovalSink wires the approval sink adapter.
func NewSIMonitorApprovalSink(events biz.MonitorEventRepo, lg loggateway.Logger) *SIMonitorApprovalSink {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SIMonitorApprovalSink{events: events, lg: lg.With(loggateway.Domain("si_approval"))}
}

// SubmitApproval implements biz.SIApprovalSink. The returned approval ID is
// deterministic ("si-approval:<runID>"); the router currently discards it.
func (s *SIMonitorApprovalSink) SubmitApproval(ctx context.Context, run *biz.SelfImprovementRun) (string, error) {
	if s == nil || s.events == nil {
		return "", apierror.Internal("SELF_IMPROVEMENT", "approval sink not initialized")
	}
	if run == nil {
		return "", apierror.BadRequest("SELF_IMPROVEMENT", "run is required")
	}
	approvalID := "si-approval:" + run.ID
	dup, err := s.hasPendingRequest(ctx, run.ID)
	if err != nil {
		// 幂等扫描失败不阻断提交（宁可重复事件，不可丢审批）。
		s.lg.Warn("si approval idempotency scan degraded",
			loggateway.StepID("si_approval.scan"),
			loggateway.Str("run_id", run.ID), loggateway.Err(err))
	} else if dup {
		return approvalID, nil
	}
	meta := siRunEventMetadata(run)
	meta["approval_id"] = approvalID
	if run.Governance != nil {
		meta["rule_hits"] = run.Governance.RuleHits
	}
	desc := fmt.Sprintf("自改进补丁待审批（%s 风险）: run=%s kind=%s +%d/-%d",
		run.RiskLevel, run.ID, run.PatchKind, run.DiffStats.Additions, run.DiffStats.Deletions)
	if err := s.events.InsertMonitorEvent(ctx, bizmonitor.EventWrite{
		EventKey:     siEventKeyApproval,
		Name:         "自改进审批请求",
		Description:  desc,
		Status:       "warn",
		MetadataJSON: siMarshalMetadata(meta),
	}); err != nil {
		return "", err
	}
	return approvalID, nil
}

// hasPendingRequest reports whether a recent approval request already carries
// this run ID (restart idempotency, see biz drive `routed` 注释）。
func (s *SIMonitorApprovalSink) hasPendingRequest(ctx context.Context, runID string) (bool, error) {
	res, err := s.events.ListMonitorEvents(ctx, bizmonitor.EventsQuery{
		Limit:     siApprovalScanLimit,
		EventType: siEventKeyApproval,
	})
	if err != nil {
		return false, err
	}
	needle := `"run_id":"` + runID + `"`
	for i := range res.Items {
		ev := &res.Items[i]
		if strings.Contains(ev.MetadataJSON, needle) {
			return true, nil
		}
	}
	return false, nil
}

// SIMonitorActivitySink mounts the Meta Team stage tree as monitor events
// (biz.SIActivitySink）。平台级 run 无会话上下文，Activity/WS 树属 P5 控制台；
// P4 先落 Events 页可视。
type SIMonitorActivitySink struct {
	events biz.MonitorEventRepo
	lg     loggateway.Logger
}

// NewSIMonitorActivitySink wires the activity sink adapter.
func NewSIMonitorActivitySink(events biz.MonitorEventRepo, lg loggateway.Logger) *SIMonitorActivitySink {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SIMonitorActivitySink{events: events, lg: lg.With(loggateway.Domain("si_activity"))}
}

// EmitSIActivity implements biz.SIActivitySink.
func (s *SIMonitorActivitySink) EmitSIActivity(ctx context.Context, a biz.SIActivityRecord) error {
	if s == nil || s.events == nil {
		return nil // 可观测性降级不阻断闭环（与 pipeline nil-sink 语义一致）。
	}
	status := string(a.Status)
	desc := a.Summary
	if desc == "" {
		desc = fmt.Sprintf("自改进阶段 %s → %s", a.Stage, a.Status)
	}
	return s.events.InsertMonitorEvent(ctx, bizmonitor.EventWrite{
		EventKey:    siEventKeyStage,
		Name:        "自改进阶段",
		Description: desc,
		Status:      status,
		MetadataJSON: siMarshalMetadata(map[string]any{
			"activity_id":        a.ID,
			"parent_activity_id": a.ParentActivityID,
			"run_id":             a.RunID,
			"stage":              a.Stage,
			"attempt":            a.Attempt,
		}),
	})
}

// siFailurePatternKB is the narrow FailurePattern KB surface the negative
// pattern sink needs (data.FailurePatternReadWriter satisfies it).
type siFailurePatternKB interface {
	GetByPatternHash(ctx context.Context, hash string) (*bizmonitor.FailurePattern, error)
	Create(ctx context.Context, pattern bizmonitor.FailurePattern) error
	IncrementFail(ctx context.Context, id string) error
}

// SIKBNegativePatternSink writes regressed-patch anti-patterns to the
// FailurePattern KB (biz.SINegativePatternSink, D8）。同哈希重复出现时转为
// IncrementFail 累加负面计数，不建行。
type SIKBNegativePatternSink struct {
	kb siFailurePatternKB
	lg loggateway.Logger
}

// NewSIKBNegativePatternSink wires the negative pattern sink adapter.
func NewSIKBNegativePatternSink(kb siFailurePatternKB, lg loggateway.Logger) *SIKBNegativePatternSink {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SIKBNegativePatternSink{kb: kb, lg: lg.With(loggateway.Domain("si_negative_kb"))}
}

// RecordNegativePattern implements biz.SINegativePatternSink.
func (s *SIKBNegativePatternSink) RecordNegativePattern(ctx context.Context, rec biz.SINegativePatternRecord) error {
	if s == nil || s.kb == nil {
		return apierror.Internal("SELF_IMPROVEMENT", "negative pattern KB not wired")
	}
	if rec.PatternHash == "" {
		return apierror.BadRequest("SELF_IMPROVEMENT", "pattern hash is required")
	}
	existing, err := s.kb.GetByPatternHash(ctx, rec.PatternHash)
	if err != nil {
		return err
	}
	if existing != nil {
		return s.kb.IncrementFail(ctx, existing.ID)
	}
	now := time.Now().UTC()
	return s.kb.Create(ctx, bizmonitor.FailurePattern{
		ID:           uuid.NewString(),
		Source:       bizmonitor.FailurePatternSource("self_improvement"),
		Type:         rec.TriggerSource,
		PatternHash:  rec.PatternHash,
		PatternRegex: rec.PatternRegex,
		FixAction:    bizmonitor.FixAction{Type: "log_only"},
		Confidence:   0.1,
		FailCount:    1,
		Version:      1,
		IsActive:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
}

// SIOrchestratorFeedbackSink escalates trigger cooldowns on the unified
// evolution orchestrator (biz.SITriggerFeedbackSink, D8 触发器自适应降频）。
type SIOrchestratorFeedbackSink struct {
	orch *biz.SkillEvolutionOrchestrator
}

// NewSIOrchestratorFeedbackSink wires the trigger feedback sink adapter.
func NewSIOrchestratorFeedbackSink(orch *biz.SkillEvolutionOrchestrator) *SIOrchestratorFeedbackSink {
	return &SIOrchestratorFeedbackSink{orch: orch}
}

// EscalateTriggerCooldown implements biz.SITriggerFeedbackSink.
func (s *SIOrchestratorFeedbackSink) EscalateTriggerCooldown(_ context.Context, triggerSource string, factor float64) error {
	if s == nil || s.orch == nil {
		return apierror.Internal("SELF_IMPROVEMENT", "trigger feedback orchestrator not wired")
	}
	s.orch.SetTriggerCooldownMultiplier(triggerSource, factor)
	return nil
}
