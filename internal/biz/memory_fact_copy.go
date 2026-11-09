package biz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// FactCopy 描述一次 L3 事实跨域复制（D3 拍板 acceptance #3：copyMemory
// 等价物——note 从 user 域复制到 workspace）。双侧 scope ACL（源 read +
// 目标 write）由 service 层 authorizeMemoryScope 完成；本层负责取源行
// （信封透明解密后的权威全文）、目标域重打指纹、写信封、落审计。
type FactCopy struct {
	SourceFactID string
	// 目标 scope：agent | team | session | user | global | workspace
	TargetScopeType   string
	TargetScopeID     string
	TargetAgentID     string
	TargetWorkspaceID string
	TargetUserID      string
	TargetTeamID      string
	// 可选覆写；空 = 继承源行（信封解密后的权威 statement/details/kind）
	Statement       string
	DetailsMarkdown string
	FactKind        string
	// 审计 actor；空则从 ctx 的登录态取
	Actor string
}

// MemoryFactCopyAuditRecorder 是 CopyFactRow 落审计的窄接口
//（data.NewMonitorAuditRepo(d) 的返回满足它）。
type MemoryFactCopyAuditRecorder interface {
	InsertAuditLog(ctx context.Context, entry AuditLog) error
}

// SetMemoryFactCopyAuditRecorder 注入跨域复制审计记录器（wire 装配态后置，
// 与 SetPathBExtractor 同模式——构造签名不变，避免 Wire 图重排）。
func (uc *MemoryAdminUsecase) SetMemoryFactCopyAuditRecorder(rec MemoryFactCopyAuditRecorder) {
	if uc != nil {
		uc.copyAudit = rec
	}
}

// PeekFactScope 返回源行的 scope 三元组（scope_type/scope_id/pii_flag）供
// service 层做源侧只读 ACL 判定；不返回行内容（最小披露）。这是非 admin
// 路径（CopyMemoryFact 源侧校验）专用——区别于 GetFactByID 的 requireAdmin。
func (uc *MemoryAdminUsecase) PeekFactScope(ctx context.Context, factID string) (scopeType, scopeID string, pii bool, err error) {
	if uc == nil || uc.admin == nil {
		return "", "", false, apierror.Internal(apierror.DomainMemory, "memory admin store not wired")
	}
	rows, err := uc.admin.GetFactRowsByIDs(ctx, []string{strings.TrimSpace(factID)})
	if err != nil {
		return "", "", false, err
	}
	if len(rows) == 0 {
		return "", "", false, apierror.NotFound(apierror.DomainMemory, "source fact not found")
	}
	var row map[string]any
	if err := json.Unmarshal(rows[0], &row); err != nil {
		return "", "", false, apierror.Internal(apierror.DomainMemory, "source fact row is corrupt")
	}
	return strings.ToLower(strings.TrimSpace(factCopyAnyStr(row["scope_type"]))),
		strings.TrimSpace(factCopyAnyStr(row["scope_id"])),
		factCopyAnyBool(row["pii_flag"]), nil
}

// CopyFactRow 把一条既有事实复制到目标 scope：
//  1. 取源行（GetFactRowsByIDs → scanFactRowJSON 已透明解密，row 内是权威全文）；
//  2. PII 门兜底——user 域 pii_flag=1 的行处于管理员复核隔离态，复制 = 把
//     隔离 PII 洗进目标域，必须拒绝（admin ListPIIFlaggedFacts 是唯一读者路径）；
//  3. 目标域重打指纹（指纹绑 scope_type+scope_id，天然成为新行）→ UpsertFactRow
//     自动按新指纹重写信封（AAD=新指纹，密文不可跨行挪移）；
//  4. 审计 memory.fact.copy（内容只记 sha256 前 16 hex，不留明文）。
func (uc *MemoryAdminUsecase) CopyFactRow(ctx context.Context, in FactCopy) ([]byte, error) {
	if uc == nil || uc.factWriter == nil {
		return nil, apierror.Internal(apierror.DomainMemory, "fact writer not wired")
	}
	srcID := strings.TrimSpace(in.SourceFactID)
	if srcID == "" {
		return nil, apierror.BadRequest(apierror.DomainMemory, "source_fact_id is required")
	}
	targetScopeType := strings.ToLower(strings.TrimSpace(in.TargetScopeType))
	if targetScopeType == "" {
		return nil, apierror.BadRequest(apierror.DomainMemory, "target_scope_type is required")
	}

	srcRows, err := uc.admin.GetFactRowsByIDs(ctx, []string{srcID})
	if err != nil {
		return nil, err
	}
	if len(srcRows) == 0 {
		return nil, apierror.NotFound(apierror.DomainMemory, "source fact not found")
	}
	var src map[string]any
	if err := json.Unmarshal(srcRows[0], &src); err != nil {
		return nil, apierror.Internal(apierror.DomainMemory, "source fact row is corrupt")
	}
	if factCopyAnyStr(src["scope_type"]) == "user" && factCopyAnyBool(src["pii_flag"]) {
		return nil, apierror.Forbidden(apierror.DomainMemory,
			"PII-flagged user facts are quarantined and cannot be copied across scopes")
	}

	stmt := strings.TrimSpace(in.Statement)
	if stmt == "" {
		stmt = strings.TrimSpace(factCopyAnyStr(src["statement"]))
	}
	if stmt == "" {
		return nil, apierror.BadRequest(apierror.DomainMemory, "statement is empty")
	}
	details := in.DetailsMarkdown
	if strings.TrimSpace(details) == "" {
		details = factCopyAnyStr(src["details_markdown"])
	}
	kind := strings.TrimSpace(in.FactKind)
	if kind == "" {
		kind = factCopyAnyStr(src["fact_kind"])
	}

	// 复制血缘进 metadata_json（下一行指纹不含 metadata，重复复制幂等收敛）。
	meta := map[string]any{}
	if raw := strings.TrimSpace(factCopyAnyStr(src["metadata_json"])); raw != "" && raw != "null" {
		_ = json.Unmarshal([]byte(raw), &meta)
	}
	meta["copied_from_fact_id"] = srcID

	raw, err := uc.UpsertFactRow(ctx, FactUpsert{
		ScopeType:       targetScopeType,
		ScopeID:         strings.TrimSpace(in.TargetScopeID),
		WorkspaceID:     strings.TrimSpace(in.TargetWorkspaceID),
		UserID:          strings.TrimSpace(in.TargetUserID),
		TeamID:          strings.TrimSpace(in.TargetTeamID),
		AgentID:         strings.TrimSpace(in.TargetAgentID),
		Statement:       stmt,
		DetailsMarkdown: details,
		FactKind:        kind,
		Confidence:      factCopyAnyFloat(src["confidence"]),
		Importance:      factCopyAnyFloat(src["importance"]),
		SourceKind:      "copy",
		MetadataJSON:    factCopyMustJSON(meta),
	})
	if err != nil {
		return nil, err
	}
	uc.recordFactCopyAudit(ctx, in, src, raw)
	return raw, nil
}

// recordFactCopyAudit 落 memory.fact.copy 审计：detail 只含 scope 轨迹 +
// statement 的 sha256 前 16 hex（内容指纹，可比对但不泄明文——明文三不进）。
// 失败仅告警降级，不阻断复制主流程。
func (uc *MemoryAdminUsecase) recordFactCopyAudit(ctx context.Context, in FactCopy, src map[string]any, newRaw []byte) {
	if uc == nil || uc.copyAudit == nil {
		return
	}
	newID := ""
	var newRow map[string]any
	if err := json.Unmarshal(newRaw, &newRow); err == nil {
		newID = factCopyAnyStr(newRow["id"])
	}
	stmt := strings.TrimSpace(in.Statement)
	if stmt == "" {
		stmt = strings.TrimSpace(factCopyAnyStr(src["statement"]))
	}
	sum := sha256.Sum256([]byte(stmt))
	detail, _ := json.Marshal(map[string]any{
		"source_fact_id":    strings.TrimSpace(in.SourceFactID),
		"source_scope_type": factCopyAnyStr(src["scope_type"]),
		"target_scope_type": strings.ToLower(strings.TrimSpace(in.TargetScopeType)),
		"target_scope_id":   strings.TrimSpace(in.TargetScopeID),
		"fact_kind":         factCopyAnyStr(src["fact_kind"]),
		"statement_sha256":  hex.EncodeToString(sum[:])[:16],
	})
	actor := strings.TrimSpace(in.Actor)
	if actor == "" {
		if p, ok := auth.FromContext(ctx); ok && p != nil {
			actor = fmt.Sprintf("%d", p.UserID)
		}
	}
	if err := uc.copyAudit.InsertAuditLog(ctx, AuditLog{
		Action:     "memory.fact.copy",
		Resource:   "memory_fact",
		ResourceID: newID,
		Detail:     string(detail),
		Actor:      actor,
		Severity:   "info",
	}); err != nil {
		uc.lg.Warn("memory fact copy audit persist failed",
			loggateway.StepID("memory.fact_copy_audit_fail"),
			loggateway.Str("source_fact_id", strings.TrimSpace(in.SourceFactID)),
			loggateway.Err(err))
	}
}

// ---- 局部小工具（biz 根包不引 data 的 anyStr，避免跨层复用）----

func factCopyAnyStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func factCopyAnyFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	default:
		return 0
	}
}

func factCopyAnyBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return strings.TrimSpace(t) == "1" || strings.EqualFold(strings.TrimSpace(t), "true")
	default:
		return false
	}
}

func factCopyMustJSON(m map[string]any) string {
	if len(m) == 0 {
		return ""
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(raw)
}
