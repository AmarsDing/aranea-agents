package tool

import (
	"context"
	"sort"
	"strings"
	"sync"

	"aranea-agents/internal/biz/shared"
)

// P0-5 审批工程化（docs/reports/2026-10-21-review-harness-p0-gap-solutions.md §6，
// 批 A11）：会话/Agent 作用域的确定性审批姿态表。CI/夜间批量场景既不能每卡
// 都点（ask 太吵）也不敢全局关确认（bypass 仅受 ARANEA_DEV_MODE 闸，生产恒
// false）——本表给出 per-scope 的一等配置姿态：
//   - ask    ：命中即强制确认（session/persisted grant 仍可满足，与 catalog
//     路径 grant 语义对齐）。
//   - never  ：命中即确定性拒绝（BeforeTool p3 闸短路，先于循环守卫/确认门禁；
//     拒绝文案只给规则 ID 不给 pattern——镜像 param_rule_deny 信息论保护）。
//   - trusted：预授权新档位（decide() 中恒排在 computer-use danger floor 与
//     param rule verdict 之后消费——danger floor 永不被 trusted 越过）。
//   - auto   ：自动审批档（D5/phase 2 引入）：消费语义等同 ask（同 grants 可满足、
//     同 reason=policy_ask，零新 trigger/reason），仅竞选 rank 独立成档
//     （never=0 < auto=1 < ask=2 < trusted=3）并在 verdict 投影中携带
//     source_mode=auto 供 auto_review_verdict 审计区分。状态机见
//     docs/development/96-approval-policy-phase2 三件套。
//
// 事件定级（AS-EVT-01）：never 命中产 approval_policy_denied=Informational
// （SystemNoticeEvent+审计双写，避开 BlockUpTo 行列防止等待副作用放大）；
// 确认闸 approval.asked/decided 对偶=Important（decision_records 经 outbox
// BlockUpTo，不进 Critical lane——Critical 保留给 terminal/reset-required）。
// 详见 §9.3 映射表。

// ApprovalMode 是审批姿态的四值枚举（D5/phase 2 引入第四档 "auto"；
// >3 值触发 AS-FSM-01 显式状态机立档义务——已结清，见
// docs/development/96-approval-policy-phase2 三件套）。
type ApprovalMode string

const (
	ApprovalModeAsk     ApprovalMode = "ask"
	ApprovalModeNever   ApprovalMode = "never"
	ApprovalModeTrusted ApprovalMode = "trusted"
	ApprovalModeAuto    ApprovalMode = "auto"
)

// ApprovalScope 是策略行归属的作用域。
type ApprovalScope string

const (
	ApprovalScopeSession ApprovalScope = "session"
	ApprovalScopeAgent   ApprovalScope = "agent"
)

// ApprovalPolicy 是 session_approval_policies 表的一行（13 字段，AS-COG-01 合规）。
type ApprovalPolicy struct {
	ID            string
	Scope         string // session | agent
	ScopeKey      string // scope=session → sessionID；scope=agent → agentID
	ToolPattern   string // 必填（"*" 显式通配）；glob / 're:' 正则（policyrule.MatchText 语义）
	CommandPrefix string // 可空；"*" 系 glob 而非纯前缀（沿引擎语义，命名随表列）
	ArgsPattern   string // 可空；glob / 're:' 正则，对参数压平文本匹配
	Mode          string // ask | never | trusted | auto
	Priority      int
	Enabled       bool
	ShadowOnly    bool  // 预演：命中必落审计+notice，零强制
	ExpiresAt     int64 // unix 秒；0 = 不过期（trusted 强制 scope+TTL 的TTL 载体）
	CreatedAt     int64 // unix 秒
	CreatedBy     string
}

// ApprovalPolicyReader 是运行时 gate 的读口。
// Stability:evolving
type ApprovalPolicyReader interface {
	// ListEnabledApprovalPolicies 返回 (scope, scopeKey) 下 enabled 且未过期的
	// 规则（Priority 升序、同档 ID 字典序，确定性）。nowUnix 由调用方给定
	// （测试可控时钟）。
	ListEnabledApprovalPolicies(ctx context.Context, scope, scopeKey string, nowUnix int64) ([]ApprovalPolicy, error)
}

// ApprovalPolicyAdmin 是管理面写口。写路径必须经 ValidateApprovalPolicy 拒收
// 坏 pattern（业务侧 PG 实现落地时复用同一校验，防两套语义）。
// Stability:evolving
type ApprovalPolicyAdmin interface {
	// ListApprovalPolicies 返回 (scope, scopeKey) 下全部规则（含 disabled 与
	// 已过期，管理面可见可清理），Priority 升序、同档 ID 字典序。
	ListApprovalPolicies(ctx context.Context, scope, scopeKey string) ([]ApprovalPolicy, error)
	// GetApprovalPolicyByID 按主键查单条；不存在返回 shared.ErrNotFound。
	GetApprovalPolicyByID(ctx context.Context, id string) (ApprovalPolicy, error)
	// UpsertApprovalPolicy：ID 已存在为更新，否则新建。坏 pattern/非法枚举拒收。
	UpsertApprovalPolicy(ctx context.Context, p ApprovalPolicy) error
	// DeleteApprovalPolicy 幂等。
	DeleteApprovalPolicy(ctx context.Context, id string) error
}

// ApprovalPolicyStore combines reader and admin; used only for Wire wiring
// （批 B：ToolUsecase 增字段 + option + wire provider，本批仅交付接口与实现）。
// Stability:evolving
type ApprovalPolicyStore interface {
	ApprovalPolicyReader
	ApprovalPolicyAdmin
}

// InMemApprovalPolicyStore 是 ApprovalPolicyStore 的内存实现：会话级姿态的
// 进程内默认实现（单测/未装配 PG repo 的部署形态兜底）。进程级协调态不新增
// sync.Map 全局量——实例由装配方持有注入。
type InMemApprovalPolicyStore struct {
	mu    sync.RWMutex
	items map[string]ApprovalPolicy
}

// NewInMemApprovalPolicyStore 构造空内存仓。
func NewInMemApprovalPolicyStore() *InMemApprovalPolicyStore {
	return &InMemApprovalPolicyStore{items: map[string]ApprovalPolicy{}}
}

func (s *InMemApprovalPolicyStore) ListApprovalPolicies(_ context.Context, scope, scopeKey string) ([]ApprovalPolicy, error) {
	if s == nil {
		return nil, nil
	}
	scope = strings.TrimSpace(scope)
	scopeKey = strings.TrimSpace(scopeKey)
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ApprovalPolicy, 0, len(s.items))
	for _, p := range s.items {
		if p.Scope == scope && p.ScopeKey == scopeKey {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *InMemApprovalPolicyStore) ListEnabledApprovalPolicies(_ context.Context, scope, scopeKey string, nowUnix int64) ([]ApprovalPolicy, error) {
	rows, err := s.ListApprovalPolicies(context.Background(), scope, scopeKey)
	if err != nil {
		return nil, err
	}
	out := rows[:0]
	for i := range rows {
		if approvalPolicyEffective(&rows[i], nowUnix) {
			out = append(out, rows[i])
		}
	}
	return out, nil
}

func (s *InMemApprovalPolicyStore) GetApprovalPolicyByID(_ context.Context, id string) (ApprovalPolicy, error) {
	if s == nil {
		return ApprovalPolicy{}, shared.ErrNotFound
	}
	id = strings.TrimSpace(id)
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.items[id]
	if !ok {
		return ApprovalPolicy{}, shared.ErrNotFound
	}
	return p, nil
}

func (s *InMemApprovalPolicyStore) UpsertApprovalPolicy(_ context.Context, p ApprovalPolicy) error {
	if s == nil {
		return nil
	}
	p.Scope = strings.TrimSpace(p.Scope)
	p.ScopeKey = strings.TrimSpace(p.ScopeKey)
	p.Mode = strings.TrimSpace(p.Mode)
	p.ID = strings.TrimSpace(p.ID)
	p.ToolPattern = strings.TrimSpace(p.ToolPattern)
	if err := ValidateApprovalPolicy(p); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[p.ID] = p
	return nil
}

func (s *InMemApprovalPolicyStore) DeleteApprovalPolicy(_ context.Context, id string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, strings.TrimSpace(id))
	return nil
}

var _ ApprovalPolicyStore = (*InMemApprovalPolicyStore)(nil)
