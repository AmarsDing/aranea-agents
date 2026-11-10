package biz

// D6 脱敏五闸（提案侧，E 阶段 W2b V4，需求 §7.3 验收 #4 主体）。
// 语义锚 = workbench/server/maskgate.go（逐字移植：闸门1 提案入库(proposed)
// → 闸门2 样本试跑（正样本召回必须 100%、负样本命中必须 0、无正样本
// fail-closed）→ 闸门3 静态检查（编译 + ReDoS 启发式）→ 闸门4 人工审批
// （不可自批；批准只生成 shadow 规则，不直接生效）→ 闸门5 影子模式
// （shadow 只计数）→ activate 后按 rollout_pct 灰度生效。AI 提案与手工
// 提案同等待遇，永不自动生效。
//
// 登记偏离（W4 项 (w) 续）：
//  1. 提案→规则关联用确定性 id（MaskRuleIDForProposal = "mr_"+提案 id），
//     取代 wb 的按 name 匹配（同名提案会串规则）；proto 无 name 字段，
//     提案/规则 name 统一取 entity。
//  2. Create/RunGates 审计 best-effort（wb auditSensitive 是「落库后再 500」
//     的半失败语义）； approve/reject/activate 三条终态路径审计严格且与
//     状态变更同事务（wb 仅 approve 同事务，aranea 把 activate 也纳入）。
//  3. activate 的 rollout_pct 越界回落 100（wb 逐字：非法值静默取 100）。
//  4. 激活后引擎热副本 ≤30s TTL 生效（wb 显式 reload；aranea 常驻进程走
//     TTL 热副本，见 secops_mask_gate.go）。
//  5. 审批闸本期不登记统一审批中心 lane（peer 正在改 internal/biz/approval
//     sources；以 secops.policy 权限码 + 不可自批用例内硬校验覆盖同等语义，
//     lane 注册为 peer 收口后的 follow-up）。

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// 提案状态机（与 DDL 20261327 mask_proposals.status 注释一致）。
const (
	MaskProposalStatusProposed        = "proposed"
	MaskProposalStatusSampleFailed    = "sample_failed"
	MaskProposalStatusStaticFailed    = "static_failed"
	MaskProposalStatusPendingApproval = "pending_approval"
	MaskProposalStatusShadow          = "shadow"
	MaskProposalStatusRejected        = "rejected"
	MaskProposalStatusActivated       = "activated"
)

// 提案来源（AI 提案与手工提案同等待遇——来源只做记录，不改变闸门路径）。
const (
	MaskProposalSourceManual = "manual"
	MaskProposalSourceAI     = "ai"
)

// 提案面审计 action（wb 原名 + secops.mask 前缀）。
const (
	SecopsMaskProposalActionCreate       = "secops.mask.proposal_create"
	SecopsMaskProposalActionSampleFailed = "secops.mask.gate_sample_failed"
	SecopsMaskProposalActionStaticFailed = "secops.mask.gate_static_failed"
	SecopsMaskProposalActionGatePassed   = "secops.mask.gate_passed"
	SecopsMaskProposalActionApprove      = "secops.mask.proposal_approve"
	SecopsMaskProposalActionReject       = "secops.mask.proposal_reject"
	SecopsMaskProposalActionActivate     = "secops.mask.proposal_activate"
)

// maskProposalListPageSizeDefault / Max 是分页默认与上限（列表 RPC）。
const (
	maskProposalListPageSizeDefault = 50
	maskProposalListPageSizeMax     = 200
)

// MaskProposal 脱敏提案（mask_proposals 行；wordlist/静态结果保留 JSON 文本，
// 双方言可移植）。
type MaskProposal struct {
	ID                 string
	Name               string
	Entity             string
	Pattern            string
	WordlistJSON       string
	Operator           string
	Source             string
	Status             string
	SampleRecall       *float64
	SampleDetailJSON   string
	StaticFindingsJSON string
	CreatedBy          string
	ReviewedBy         string
	ReviewedAt         time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// MaskSample 脱敏样本（mask_samples 行；positive=1 应被命中 / 0 不应被命中）。
type MaskSample struct {
	ID       string
	Entity   string
	Text     string
	Positive bool
}

// MaskProposalInput 是提案入库入参（proto CreateMaskProposalRequest 映射）。
type MaskProposalInput struct {
	Entity       string
	Pattern      string
	WordlistJSON string
	Operator     string
}

// SecopsMaskRuleAdminRepo 是提案面的规则读口（全量含 disabled/shadow，
// ListMaskRules RPC 用；与运行面窄口 SecopsMaskRuleRepo 分立）。
type SecopsMaskRuleAdminRepo interface {
	// ListMaskRules 返回全部规则（id ASC）。
	ListMaskRules(ctx context.Context) ([]MaskRule, error)
}

// SecopsMaskProposalRepo 是脱敏提案台账窄口。
type SecopsMaskProposalRepo interface {
	// ListProposals 分页列提案（status 空 = 全部；created_at DESC）。
	ListProposals(ctx context.Context, status string, limit, offset int) ([]MaskProposal, int64, error)
	// GetProposal 读单条；不存在 → NotFound。
	GetProposal(ctx context.Context, id string) (*MaskProposal, error)
	// CreateProposal 插入提案（status=proposed）。
	CreateProposal(ctx context.Context, p MaskProposal) error
	// ListSamplesByEntity 列同 entity 的样本集（闸门2 试跑输入）。
	ListSamplesByEntity(ctx context.Context, entity string) ([]MaskSample, error)
	// UpdateGates 是闸门2/3 的 CAS 状态迁移（WHERE id AND status=fromStatus），
	// 返回是否命中；未命中 = 状态已被并发改变（调用方报 Conflict）。
	UpdateGates(ctx context.Context, id, fromStatus, toStatus string, recall float64, sampleDetailJSON, findingsJSON string) (bool, error)
	// ReviewProposal 是审批事务：读行校验 pending_approval →（approve 时
	// 插 shadow 规则）→ CAS 状态迁移 → 审计闭包（同事务；失败回滚）。
	ReviewProposal(ctx context.Context, id, reviewerID string, approve bool, ruleID string, audit func(context.Context) error) (*MaskProposal, error)
	// ActivateProposal 是激活事务：shadow 规则 shadow=0 + 灰度 + version+1
	// → 提案 shadow→activated → 审计闭包（同事务；失败回滚）。
	ActivateProposal(ctx context.Context, id string, rolloutPct int, ruleID string, audit func(context.Context) error) (*MaskProposal, error)
}

// MaskRuleIDForProposal 是提案→规则确定性关联 id（approve 生成、activate
// 定位同一条规则；取代 wb 的按 name 匹配）。
func MaskRuleIDForProposal(proposalID string) string {
	return "mr_" + strings.TrimSpace(proposalID)
}

// ── 闸门3：静态检查 ───────────────────────────────────────────────────

// 静态检查启发式（wb 逐字；Go RE2 无回溯，此检查面向「规则可移植性/
// 误配置」的防线，不代表本引擎存在 ReDoS 面）。
var (
	maskNestedQuantifierRe  = regexp.MustCompile(`\([^)]*[+*][^)]*\)[+*?{]`)
	maskStackedQuantifierRe = regexp.MustCompile(`[+*?][+*]`)
	maskOverlapWildcardRe   = regexp.MustCompile(`\(\.\*\)\|`)
)

// StaticCheckMaskPattern 闸门3：正则编译 + ReDoS 启发式。空 pattern（纯
// 词表规则）无 findings。
func StaticCheckMaskPattern(pattern string) []string {
	var findings []string
	if pattern == "" {
		return findings
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return append(findings, "正则编译失败: "+err.Error())
	}
	if maskNestedQuantifierRe.MatchString(pattern) {
		findings = append(findings, "ReDoS 风险：嵌套量词（形如 (a+)+）")
	}
	if maskStackedQuantifierRe.MatchString(pattern) {
		findings = append(findings, "可疑写法：量词叠加（形如 a+*）")
	}
	if maskOverlapWildcardRe.MatchString(pattern) || strings.Contains(pattern, `(.*)(.*)`) {
		findings = append(findings, "ReDoS 风险：重叠通配分支")
	}
	return findings
}

// ── 闸门2：样本试跑 ───────────────────────────────────────────────────

// MaskGateSampleResult 是单样本试跑结果（写回 sample_detail_json）。
type MaskGateSampleResult struct {
	ID       string `json:"id"`
	Positive bool   `json:"positive"`
	Hits     int    `json:"hits"`
	Pass     bool   `json:"pass"`
}

// RunMaskSampleGate 闸门2：用同 entity 样本集试跑候选规则。
//
// 试跑只计命中不替换（Shadow=true）、全量应用（RolloutPct=100），避免占位符
// 状态与会话灰度污染判定（wb 逐字）。无正样本 → fail-closed（pass=false，
// detail 记 error 说明）；规则编译失败 → err（调用方按 400 处理，不迁移）。
func RunMaskSampleGate(samples []MaskSample, rule MaskRule) (recall float64, detailJSON string, pass bool, err error) {
	probe := rule
	probe.Enabled = true
	probe.Shadow = true
	probe.RolloutPct = 100
	eng, err := NewMaskEngine([]MaskRule{probe})
	if err != nil {
		return 0, "", false, fmt.Errorf("规则编译失败: %w", err)
	}
	results := make([]MaskGateSampleResult, 0, len(samples))
	posTotal, posHit := 0, 0
	negFail := false
	for _, s := range samples {
		_, hits := eng.Mask("gate-"+s.ID, s.Text)
		ok := (s.Positive && hits > 0) || (!s.Positive && hits == 0)
		if s.Positive {
			posTotal++
			if hits > 0 {
				posHit++
			}
		} else if hits > 0 {
			negFail = true
		}
		results = append(results, MaskGateSampleResult{ID: s.ID, Positive: s.Positive, Hits: hits, Pass: ok})
	}
	if posTotal == 0 {
		return 0, `{"error":"该 entity 无正样本，fail-closed 拒入库"}`, false, nil
	}
	b, marshalErr := json.Marshal(results)
	if marshalErr != nil {
		return 0, "", false, marshalErr
	}
	recall = float64(posHit) / float64(posTotal)
	pass = recall >= 1.0 && !negFail
	return recall, string(b), pass, nil
}

// ── 用例 ─────────────────────────────────────────────────────────────

// SecopsMaskProposalUsecase 是脱敏提案五闸用例（提案面；运行面引擎在
// SecopsMaskUsecase）。
type SecopsMaskProposalUsecase struct {
	repo  SecopsMaskProposalRepo
	rules SecopsMaskRuleAdminRepo
	audit SecopsOffboardAuditRecorder
	lg    loggateway.Logger
	// now / newID 可测替换（nil 回落 time.Now / "mp_"+uuid）。
	now   func() time.Time
	newID func() string
}

// NewSecopsMaskProposalUsecase 装配提案用例。repo 为 nil 返回 nil
// （service 侧据此报 Internal，wire 侧不注册）。
func NewSecopsMaskProposalUsecase(repo SecopsMaskProposalRepo, rules SecopsMaskRuleAdminRepo, lg loggateway.Logger) *SecopsMaskProposalUsecase {
	if repo == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SecopsMaskProposalUsecase{
		repo:  repo,
		rules: rules,
		lg:    lg,
		now:   time.Now,
		newID: func() string { return "mp_" + uuid.NewString() },
	}
}

// SetAuditRecorder 后置注入审计写口（构造签名稳定，wire 友好）。
func (uc *SecopsMaskProposalUsecase) SetAuditRecorder(rec SecopsOffboardAuditRecorder) {
	if uc != nil {
		uc.audit = rec
	}
}

func (uc *SecopsMaskProposalUsecase) clock() time.Time {
	if uc != nil && uc.now != nil {
		return uc.now()
	}
	return time.Now()
}

// ListRules 列全部脱敏规则（含 shadow/disabled：安全管理员维护视图）。
func (uc *SecopsMaskProposalUsecase) ListRules(ctx context.Context) ([]MaskRule, error) {
	if uc == nil || uc.rules == nil {
		return nil, apierror.Internal(apierror.DomainAdmin, "secops mask rule repo not wired")
	}
	return uc.rules.ListMaskRules(ctx)
}

// ListProposals 分页列提案（status 空 = 全部）。
func (uc *SecopsMaskProposalUsecase) ListProposals(ctx context.Context, status string, page, pageSize int) ([]MaskProposal, int64, error) {
	if uc == nil {
		return nil, 0, apierror.Internal(apierror.DomainAdmin, "secops mask proposal usecase not wired")
	}
	status = strings.TrimSpace(status)
	if status != "" && !validMaskProposalStatus(status) {
		return nil, 0, apierror.BadRequest(apierror.DomainAdmin, "invalid proposal status %q", status)
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = maskProposalListPageSizeDefault
	}
	if pageSize > maskProposalListPageSizeMax {
		pageSize = maskProposalListPageSizeMax
	}
	return uc.repo.ListProposals(ctx, status, pageSize, (page-1)*pageSize)
}

// CreateProposal 闸门1：提案入库（status 恒 proposed；AI/手工同等）。
// 校验：entity 必填、pattern 与 wordlist 至少一项、operator 合法、
// wordlist_json 必须是字符串数组（TEXT[] → TEXT 迁移后的格式闸）。
func (uc *SecopsMaskProposalUsecase) CreateProposal(ctx context.Context, in MaskProposalInput, actorID string) (*MaskProposal, error) {
	entity := strings.TrimSpace(in.Entity)
	if entity == "" {
		return nil, apierror.BadRequest(apierror.DomainAdmin, "entity is required")
	}
	operator := strings.TrimSpace(in.Operator)
	if operator == "" {
		operator = MaskOperatorPlaceholder
	}
	if operator != MaskOperatorPlaceholder && operator != MaskOperatorMask {
		return nil, apierror.BadRequest(apierror.DomainAdmin, "operator must be %q or %q", MaskOperatorPlaceholder, MaskOperatorMask)
	}
	wordlistJSON := strings.TrimSpace(in.WordlistJSON)
	if wordlistJSON == "" {
		wordlistJSON = "[]"
	}
	wordlist, err := parseMaskWordlistJSON(wordlistJSON)
	if err != nil {
		return nil, apierror.BadRequest(apierror.DomainAdmin, "wordlist_json must be a JSON string array: %v", err)
	}
	pattern := strings.TrimSpace(in.Pattern)
	if pattern == "" && len(wordlist) == 0 {
		return nil, apierror.BadRequest(apierror.DomainAdmin, "pattern or wordlist is required")
	}
	now := uc.clock().UTC()
	p := MaskProposal{
		ID:           uc.newID(),
		Name:         entity,
		Entity:       entity,
		Pattern:      in.Pattern,
		WordlistJSON: wordlistJSON,
		Operator:     operator,
		Source:       MaskProposalSourceManual,
		Status:       MaskProposalStatusProposed,
		CreatedBy:    actorID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := uc.repo.CreateProposal(ctx, p); err != nil {
		return nil, err
	}
	uc.recordAudit(ctx, SecopsMaskProposalActionCreate, p.ID, map[string]any{
		"entity":   entity,
		"source":   p.Source,
		"operator": operator,
	}, "info")
	return &p, nil
}

// RunMaskGates 闸门2+3：样本试跑 → 静态检查。任一不过即落终态，绝不进入
// 审批；全过 → pending_approval。仅 proposed/sample_failed/static_failed
// 可运行（失败后可修提案再跑）。
func (uc *SecopsMaskProposalUsecase) RunMaskGates(ctx context.Context, id string) (*MaskProposal, error) {
	p, err := uc.repo.GetProposal(ctx, id)
	if err != nil {
		return nil, err
	}
	switch p.Status {
	case MaskProposalStatusProposed, MaskProposalStatusSampleFailed, MaskProposalStatusStaticFailed:
	default:
		return nil, apierror.FailedPrecondition(apierror.DomainAdmin,
			"only proposed/sample_failed/static_failed proposals can run gates (current: %s)", p.Status)
	}
	wordlist, err := parseMaskWordlistJSON(p.WordlistJSON)
	if err != nil {
		return nil, apierror.BadRequest(apierror.DomainAdmin, "stored wordlist_json is invalid: %v", err)
	}
	samples, err := uc.repo.ListSamplesByEntity(ctx, p.Entity)
	if err != nil {
		return nil, err
	}
	recall, detailJSON, samplePass, err := RunMaskSampleGate(samples, MaskRule{
		Name: p.Name, Entity: p.Entity, Pattern: p.Pattern,
		Wordlist: wordlist, Operator: p.Operator,
	})
	if err != nil {
		// wb：试跑期规则编译失败 → 400，不迁移状态。
		return nil, apierror.BadRequest(apierror.DomainAdmin, "%v", err)
	}

	// 闸门2 未过：落 sample_failed 终态。
	if !samplePass {
		if err := uc.transitionGates(ctx, p, MaskProposalStatusSampleFailed, recall, detailJSON, p.StaticFindingsJSON); err != nil {
			return nil, err
		}
		p.Status = MaskProposalStatusSampleFailed
		p.SampleRecall = &recall
		p.SampleDetailJSON = detailJSON
		uc.recordAudit(ctx, SecopsMaskProposalActionSampleFailed, p.ID, map[string]any{"recall": recall}, "warn")
		return p, nil
	}

	// 闸门3：静态检查。
	findings := StaticCheckMaskPattern(p.Pattern)
	if len(findings) > 0 {
		fj, marshalErr := json.Marshal(findings)
		if marshalErr != nil {
			return nil, apierror.Internal(apierror.DomainAdmin, "marshal static findings: %v", marshalErr)
		}
		if err := uc.transitionGates(ctx, p, MaskProposalStatusStaticFailed, recall, detailJSON, string(fj)); err != nil {
			return nil, err
		}
		p.Status = MaskProposalStatusStaticFailed
		p.SampleRecall = &recall
		p.SampleDetailJSON = detailJSON
		p.StaticFindingsJSON = string(fj)
		uc.recordAudit(ctx, SecopsMaskProposalActionStaticFailed, p.ID, map[string]any{"findings": findings, "recall": recall}, "warn")
		return p, nil
	}
	if err := uc.transitionGates(ctx, p, MaskProposalStatusPendingApproval, recall, detailJSON, "[]"); err != nil {
		return nil, err
	}
	p.Status = MaskProposalStatusPendingApproval
	p.SampleRecall = &recall
	p.SampleDetailJSON = detailJSON
	p.StaticFindingsJSON = "[]"
	uc.recordAudit(ctx, SecopsMaskProposalActionGatePassed, p.ID, map[string]any{"recall": recall}, "info")
	return p, nil
}

// transitionGates 闸门2/3 的 CAS 迁移（状态并发改变 → FailedPrecondition，
// wb「提案状态已变化，请刷新后重试」）。
func (uc *SecopsMaskProposalUsecase) transitionGates(ctx context.Context, p *MaskProposal, toStatus string, recall float64, detailJSON, findingsJSON string) error {
	ok, err := uc.repo.UpdateGates(ctx, p.ID, p.Status, toStatus, recall, detailJSON, findingsJSON)
	if err != nil {
		return err
	}
	if !ok {
		return apierror.FailedPrecondition(apierror.DomainAdmin, "proposal status changed concurrently, refresh and retry")
	}
	return nil
}

// Approve 闸门4：人工审批。仅 pending_approval 可批；不可自批；批准只生成
// 一条 shadow 规则（shadow=1, rollout_pct=100），不直接生效。审计与状态
// 变更同事务（审计失败整体回滚）。
func (uc *SecopsMaskProposalUsecase) Approve(ctx context.Context, id, reviewerID string) (*MaskProposal, error) {
	return uc.review(ctx, id, reviewerID, true)
}

// Reject 闸门4（拒绝分支）：仅 pending_approval 可拒；不可自拒（与 wb 同口径，
// 自审校验对 approve/reject 一致生效）。
func (uc *SecopsMaskProposalUsecase) Reject(ctx context.Context, id, reviewerID string) (*MaskProposal, error) {
	return uc.review(ctx, id, reviewerID, false)
}

func (uc *SecopsMaskProposalUsecase) review(ctx context.Context, id, reviewerID string, approve bool) (*MaskProposal, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apierror.BadRequest(apierror.DomainAdmin, "proposal id is required")
	}
	p, err := uc.repo.GetProposal(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.Status != MaskProposalStatusPendingApproval {
		return nil, apierror.FailedPrecondition(apierror.DomainAdmin,
			"only pending_approval proposals can be reviewed (current: %s); proposals never self-activate", p.Status)
	}
	if reviewerID != "" && p.CreatedBy == reviewerID {
		return nil, apierror.Forbidden(apierror.DomainAdmin, "cannot review own mask proposal")
	}
	ruleID := ""
	action := SecopsMaskProposalActionReject
	detail := map[string]any{"rule_id": "", "status": MaskProposalStatusRejected}
	if approve {
		ruleID = MaskRuleIDForProposal(id)
		action = SecopsMaskProposalActionApprove
		detail = map[string]any{"rule_id": ruleID, "status": MaskProposalStatusShadow}
	}
	audit := uc.strictAudit(ctx, action, id, detail, "info")
	return uc.repo.ReviewProposal(ctx, id, reviewerID, approve, ruleID, audit)
}

// Activate 闸门5→生效：仅 shadow 可激活；shadow 规则转正 + 按 rollout_pct
// 灰度（非法值回落 100，wb 逐字）+ version+1。审计与状态变更同事务。
func (uc *SecopsMaskProposalUsecase) Activate(ctx context.Context, id string, rolloutPct int) (*MaskProposal, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apierror.BadRequest(apierror.DomainAdmin, "proposal id is required")
	}
	if rolloutPct <= 0 || rolloutPct > 100 {
		rolloutPct = 100
	}
	p, err := uc.repo.GetProposal(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.Status != MaskProposalStatusShadow {
		return nil, apierror.FailedPrecondition(apierror.DomainAdmin,
			"only shadow proposals can be activated (current: %s)", p.Status)
	}
	ruleID := MaskRuleIDForProposal(id)
	audit := uc.strictAudit(ctx, SecopsMaskProposalActionActivate, id, map[string]any{
		"rule_id": ruleID, "rollout_pct": rolloutPct,
	}, "info")
	return uc.repo.ActivateProposal(ctx, id, rolloutPct, ruleID, audit)
}

// strictAudit 构造事务内审计闭包（entry 在事务外按原 ctx 构造：ExecInTx 的
// txCtx 派生自 Background，auth 主体值只能从入参 ctx 取）。
func (uc *SecopsMaskProposalUsecase) strictAudit(ctx context.Context, action, proposalID string, detail map[string]any, severity string) func(context.Context) error {
	entry, err := buildSecopsAuditEntry(ctx, action, proposalID, detail, severity)
	if err != nil {
		return func(context.Context) error { return err }
	}
	rec := uc.audit
	return func(txCtx context.Context) error {
		if rec == nil {
			return nil
		}
		return rec.InsertAuditLog(txCtx, entry)
	}
}

// recordAudit 落提案面审计（best-effort：失败仅告警；终态路径走 strictAudit）。
func (uc *SecopsMaskProposalUsecase) recordAudit(ctx context.Context, action, proposalID string, detail map[string]any, severity string) {
	if err := insertSecopsAudit(ctx, uc.audit, action, proposalID, detail, severity); err != nil {
		uc.lg.Warn("mask proposal audit persist failed",
			loggateway.StepID("secops.mask_proposal_audit_fail"),
			loggateway.Str("action", action),
			loggateway.Str("proposal_id", proposalID),
			loggateway.Err(err))
	}
}

func validMaskProposalStatus(status string) bool {
	switch status {
	case MaskProposalStatusProposed, MaskProposalStatusSampleFailed, MaskProposalStatusStaticFailed,
		MaskProposalStatusPendingApproval, MaskProposalStatusShadow,
		MaskProposalStatusRejected, MaskProposalStatusActivated:
		return true
	}
	return false
}

// parseMaskWordlistJSON 解析 wordlist_json（必须为字符串数组；空/空白 → 空表）。
func parseMaskWordlistJSON(raw string) ([]string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	return out, nil
}
