package approval

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ============================================================================
// 分级审批矩阵（拍板 A-4）：三级风险 × approval-policy 四档的默认档映射，
// 配置存库可审计。消费点=approval-policy 求值时的默认档建议（W3 接线）；
// 本期落地存取 + 审计闭环。
// ============================================================================

// 合法档位（与 approval-policy 四档一致）。
const (
	ModeNever   = "never"
	ModeAuto    = "auto"
	ModeAsk     = "ask"
	ModeTrusted = "trusted"
)

// RiskMatrixRow 矩阵行（proto RiskMatrixRow 的领域形态）。
type RiskMatrixRow struct {
	MatcherKey       string // 匹配键（tool_pattern/动作类名）
	Risk             string // low|medium|high
	DefaultMode      string // never|auto|ask|trusted
	AutoApproveTimes int    // 中风险同类自动批准次数上限（0=不启用）
	UpdatedBy        string
	UpdatedAt        time.Time
}

// RiskMatrixRepo 矩阵存取口（data 层 raw SQL 实现）。
// Stability:evolving
type RiskMatrixRepo interface {
	ListRiskMatrix(ctx context.Context) ([]RiskMatrixRow, error)
	// ReplaceRiskMatrix 全量替换（事务内 DELETE+INSERT）；updatedBy 由仓储
	// 统一落列，行内 UpdatedBy 字段忽略。
	ReplaceRiskMatrix(ctx context.Context, rows []RiskMatrixRow, updatedBy string) error
}

// MatrixAuditSink 矩阵变更审计口（service 层适配 MonitorUsecase.RecordAuditLog，
// 避免 biz 反向依赖 monitor 大包）。
// Stability:evolving
type MatrixAuditSink interface {
	RecordMatrixChange(ctx context.Context, actor string, rows []RiskMatrixRow) error
}

// Matrix 矩阵用例：校验 + 存取 + 审计。
type Matrix struct {
	repo  RiskMatrixRepo
	audit MatrixAuditSink
}

func NewMatrix(repo RiskMatrixRepo, audit MatrixAuditSink) *Matrix {
	return &Matrix{repo: repo, audit: audit}
}

func validRisk(r string) bool {
	switch r {
	case RiskLow, RiskMedium, RiskHigh:
		return true
	}
	return false
}

func validMode(m string) bool {
	switch m {
	case ModeNever, ModeAuto, ModeAsk, ModeTrusted:
		return true
	}
	return false
}

// ValidateRiskMatrixRows 校验整表（PutRiskMatrix 入参）：matcher_key 非空
// 去重、risk/mode 枚举、auto_approve_times 非负。
func ValidateRiskMatrixRows(rows []RiskMatrixRow) error {
	seen := make(map[string]struct{}, len(rows))
	for i, r := range rows {
		key := strings.TrimSpace(r.MatcherKey)
		if key == "" {
			return fmt.Errorf("approval: matrix row %d matcher_key required", i)
		}
		if _, dup := seen[key]; dup {
			return fmt.Errorf("approval: matrix matcher_key %q duplicated", key)
		}
		seen[key] = struct{}{}
		if !validRisk(r.Risk) {
			return fmt.Errorf("approval: matrix row %q risk must be low|medium|high", key)
		}
		if !validMode(r.DefaultMode) {
			return fmt.Errorf("approval: matrix row %q default_mode must be never|auto|ask|trusted", key)
		}
		if r.AutoApproveTimes < 0 {
			return fmt.Errorf("approval: matrix row %q auto_approve_times must be >= 0", key)
		}
	}
	return nil
}

// List 读取整表（按 matcher_key 字典序，稳定输出）。
func (m *Matrix) List(ctx context.Context) ([]RiskMatrixRow, error) {
	return m.repo.ListRiskMatrix(ctx)
}

// Put 全量替换矩阵：先校验，再落库，最后审计（审计失败不阻塞主流程，
// 由 sink 实现自记 Warn——与 RecordAuditLog best-effort 语义一致）。
func (m *Matrix) Put(ctx context.Context, v Viewer, rows []RiskMatrixRow) error {
	if err := ValidateRiskMatrixRows(rows); err != nil {
		return err
	}
	actor := fmt.Sprintf("%d", v.UserID)
	if err := m.repo.ReplaceRiskMatrix(ctx, rows, actor); err != nil {
		return err
	}
	if m.audit != nil {
		_ = m.audit.RecordMatrixChange(ctx, actor, rows)
	}
	return nil
}
